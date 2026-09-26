package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/domain"
)

var (
	ErrInvalidInput       = errors.New("invalid payment input")
	ErrForbidden          = errors.New("order does not belong to user")
	ErrInvalidCallback    = errors.New("invalid callback signature")
	ErrAmountMismatch     = errors.New("payment amount does not match order")
	ErrPaymentNotComplete = errors.New("payment is not complete")
)

// maxOrderQuantity bounds a single storefront order so one buyer cannot drain
// the card inventory in one request.
const maxOrderQuantity = 20

type Service struct {
	orders      contract.OrderRepo
	gateway     contract.PaymentGateway
	fulfillment contract.FulfillmentService
	users       contract.UserLookup
	paymentID   string
}

type CreatePaymentInput struct {
	UserID      uint
	OrderNo     string
	Description string
}

type CreateOrderInput struct {
	UserID   uint
	Slug     string
	Quantity int
	Contact  string
	Note     string
}

type CreatePaymentOutput struct {
	PaymentOrder *domain.PaymentOrder `json:"payment_order"`
	Order        *models.Order        `json:"order"`
}

type CallbackResult struct {
	OrderNo       string `json:"order_no"`
	TransactionID string `json:"transaction_id"`
	Status        string `json:"status"`
	Fulfillment   string `json:"fulfillment_status"`
}

type OrderList struct {
	Orders []models.Order `json:"orders"`
	Total  int64          `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

type RefundInput struct {
	OrderNo    string
	ToUserID   string
	ToUsername string
}

func NewService(orders contract.OrderRepo, gateway contract.PaymentGateway, fulfillment contract.FulfillmentService, users contract.UserLookup, paymentID string) *Service {
	if orders == nil || gateway == nil || fulfillment == nil || users == nil {
		panic("payment: nil dependency")
	}
	return &Service{orders: orders, gateway: gateway, fulfillment: fulfillment, users: users, paymentID: strings.TrimSpace(paymentID)}
}

func (s *Service) CreateOrder(ctx context.Context, input CreateOrderInput) (*models.Order, error) {
	slug := strings.TrimSpace(input.Slug)
	if input.UserID == 0 || slug == "" {
		return nil, ErrInvalidInput
	}
	quantity := input.Quantity
	if quantity == 0 {
		quantity = 1
	}
	if quantity < 1 || quantity > maxOrderQuantity {
		return nil, fmt.Errorf("%w: quantity must be between 1 and %d", ErrInvalidInput, maxOrderQuantity)
	}

	user, err := s.users.FindByID(ctx, input.UserID)
	if err != nil {
		return nil, fmt.Errorf("lookup order user: %w", err)
	}
	if user == nil || !user.IsActive {
		return nil, ErrForbidden
	}

	product, err := s.orders.GetPurchasableProduct(ctx, slug)
	if err != nil {
		return nil, err
	}
	if product.Price <= 0 {
		return nil, domain.ErrProductNotPurchasable
	}

	contact := strings.TrimSpace(input.Contact)
	note := strings.TrimSpace(input.Note)
	if len(contact) > 255 {
		return nil, fmt.Errorf("%w: contact information is too long", ErrInvalidInput)
	}
	if product.RequireContact && contact == "" {
		return nil, fmt.Errorf("%w: this product requires contact information", ErrInvalidInput)
	}
	if product.ProductType == "card" && product.AutoDeliver {
		available, err := s.orders.CountAvailableCards(ctx, product.ID)
		if err != nil {
			return nil, fmt.Errorf("count available cards: %w", err)
		}
		if available < int64(quantity) {
			return nil, domain.ErrInsufficientStock
		}
	}

	order := &models.Order{
		OrderNo:           newOrderNo(),
		UserID:            input.UserID,
		ProductID:         product.ID,
		Quantity:          quantity,
		UnitPrice:         product.Price,
		TotalAmount:       product.Price * quantity,
		Status:            "pending",
		FulfillmentStatus: "pending",
	}
	if contact != "" {
		order.CustomerContact = &contact
	}
	if note != "" {
		order.CustomerNote = &note
	}
	if err := s.orders.CreateOrder(ctx, order); err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}
	return order, nil
}

func (s *Service) CreatePayment(ctx context.Context, input CreatePaymentInput) (*CreatePaymentOutput, error) {
	input.OrderNo = strings.TrimSpace(input.OrderNo)
	if input.UserID == 0 || input.OrderNo == "" {
		return nil, ErrInvalidInput
	}
	user, err := s.users.FindByID(ctx, input.UserID)
	if err != nil {
		return nil, fmt.Errorf("lookup payment user: %w", err)
	}
	if user == nil || !user.IsActive {
		return nil, ErrForbidden
	}
	order, err := s.orders.GetOrderByNo(ctx, input.OrderNo)
	if err != nil {
		return nil, err
	}
	if order.UserID != input.UserID {
		return nil, ErrForbidden
	}
	if order.Status != "pending" {
		return nil, fmt.Errorf("%w: order status is %s", ErrInvalidInput, order.Status)
	}
	if order.TotalAmount <= 0 {
		return nil, ErrInvalidInput
	}

	description := strings.TrimSpace(input.Description)
	if description == "" {
		description = "Order " + order.OrderNo
	}
	if len(description) > 255 {
		description = description[:255]
	}
	if s.paymentID == "" {
		return nil, domain.ErrPaymentNotConfigured
	}

	result, err := s.gateway.CreatePayment(ctx, contract.CreatePaymentRequest{
		Amount: order.TotalAmount, Description: description, OrderID: order.OrderNo,
	})
	if err != nil {
		if errors.Is(err, domain.ErrPaymentNotConfigured) {
			return nil, err
		}
		return nil, fmt.Errorf("create provider payment: %w", err)
	}

	status := result.Status
	if status == "" {
		status = domain.StatusPending
	}

	// payment_orders keeps one row per order, so a buyer who returns from the
	// checkout page and pays again must update that row instead of failing the
	// retry with a unique-constraint error.
	existing, err := s.orders.GetPaymentOrderByOrderNo(ctx, order.OrderNo)
	if err != nil && !errors.Is(err, domain.ErrPaymentOrderNotFound) {
		return nil, fmt.Errorf("load payment order: %w", err)
	}
	var paymentOrder *domain.PaymentOrder
	if existing != nil {
		paymentOrder = existing
	} else {
		paymentOrder = &domain.PaymentOrder{
			OrderID: order.ID, OrderNo: order.OrderNo, UserID: order.UserID,
			PaymentID: s.paymentID, Provider: "nodeloc", Amount: order.TotalAmount,
		}
	}
	paymentOrder.Description = description
	paymentOrder.Status = status
	if result.PaymentURL != "" {
		paymentOrder.PaymentURL = stringPointer(result.PaymentURL)
	}
	if result.TransactionID != "" {
		paymentOrder.ProviderTransactionID = stringPointer(result.TransactionID)
	}
	if err := s.savePaymentOrder(ctx, paymentOrder, existing == nil); err != nil {
		return nil, fmt.Errorf("save payment order: %w", err)
	}

	raw := string(result.Raw)
	transaction, err := s.orders.GetLatestTransaction(ctx, order.OrderNo, domain.TransactionTypePayment)
	if err != nil {
		return nil, fmt.Errorf("load payment transaction: %w", err)
	}
	if transaction == nil {
		transaction = &domain.Transaction{
			OrderID: order.ID, OrderNo: order.OrderNo, Provider: "nodeloc",
			Type: domain.TransactionTypePayment, Amount: order.TotalAmount, Currency: "points",
		}
	}
	transaction.Status = status
	transaction.ProviderTransactionID = stringPointer(result.TransactionID)
	transaction.ResponsePayload = &raw
	if transaction.ID != 0 {
		if err := s.orders.SaveTransaction(ctx, transaction); err != nil {
			return nil, fmt.Errorf("save payment transaction: %w", err)
		}
	} else if err := s.orders.CreateTransaction(ctx, transaction); err != nil {
		return nil, fmt.Errorf("save payment transaction: %w", err)
	}

	return &CreatePaymentOutput{PaymentOrder: paymentOrder, Order: order}, nil
}

func (s *Service) savePaymentOrder(ctx context.Context, paymentOrder *domain.PaymentOrder, isNew bool) error {
	if isNew {
		return s.orders.CreatePaymentOrder(ctx, paymentOrder)
	}
	return s.orders.SavePaymentOrder(ctx, paymentOrder)
}

// HandleCallback settles the order behind NodeLoc's signed browser redirect.
// The redirect is signed with the merchant secret, so a payload that verifies
// is authoritative: status, amount and fees are read from it and the query API
// is only a cross-check. Losing the query call (no egress, timeout, provider
// outage) must not strand a paid order.
func (s *Service) HandleCallback(ctx context.Context, params map[string]string) (*CallbackResult, error) {
	if !s.gateway.VerifyCallback(params) {
		return nil, ErrInvalidCallback
	}
	orderNo := first(params, "external_reference", "order_id", "order_no", "out_trade_no")
	if orderNo == "" {
		return nil, ErrInvalidInput
	}
	transactionID := first(params, "transaction_id", "trade_no", "id")

	// Restarting checkout leaves several payment rows on one order, so the row
	// the callback names is the one to settle; the old "transaction id must
	// match the first row" check rejected exactly the newest, real payment.
	paymentOrder, err := s.paymentForCallback(ctx, orderNo, transactionID)
	if err != nil {
		return nil, err
	}
	if transactionID == "" {
		if paymentOrder.ProviderTransactionID == nil || *paymentOrder.ProviderTransactionID == "" {
			return nil, ErrInvalidInput
		}
		transactionID = *paymentOrder.ProviderTransactionID
	}

	reported := first(params, "status", "state")
	if !callbackCompleted(reported) {
		return nil, fmt.Errorf("%w: provider reported %q", ErrPaymentNotComplete, reported)
	}
	if amount := intParam(params, "amount"); amount != 0 && amount != paymentOrder.Amount {
		return nil, ErrAmountMismatch
	}
	platformFee, merchantPoints := intPtrParam(params, "platform_fee", "fee"), intPtrParam(params, "merchant_points", "merchant_amount")

	if query, queryErr := s.gateway.QueryPayment(ctx, transactionID); queryErr != nil {
		log.Printf("payment callback %s: provider query unavailable, settling from signed redirect: %v", orderNo, queryErr)
	} else if query != nil {
		if query.OrderID != "" && query.OrderID != orderNo {
			return nil, ErrInvalidCallback
		}
		if query.Amount != 0 && query.Amount != paymentOrder.Amount {
			return nil, ErrAmountMismatch
		}
		switch query.Status {
		case domain.StatusSucceeded, domain.StatusPaid:
			if query.PlatformFee != nil {
				platformFee = query.PlatformFee
			}
			if query.MerchantPoints != nil {
				merchantPoints = query.MerchantPoints
			}
		case domain.StatusFailed, domain.StatusCancelled, domain.StatusRefunded:
			return nil, fmt.Errorf("%w: provider query reports %s", ErrPaymentNotComplete, query.Status)
		}
	}

	now := time.Now().UTC()
	paymentOrder.Status = domain.StatusPaid
	paymentOrder.ProviderTransactionID = stringPointer(transactionID)
	paymentOrder.PaidAt = &now
	if err := s.orders.SavePaymentOrder(ctx, paymentOrder); err != nil {
		return nil, err
	}
	// The provider call is only trusted once verified, so the recorded payment
	// transaction is settled here rather than at checkout time.
	paymentTransaction, err := s.orders.GetLatestTransaction(ctx, orderNo, domain.TransactionTypePayment)
	if err != nil {
		return nil, fmt.Errorf("load payment transaction: %w", err)
	}
	if paymentTransaction != nil {
		paymentTransaction.Status = domain.StatusPaid
		paymentTransaction.ProviderTransactionID = stringPointer(transactionID)
		paymentTransaction.CompletedAt = &now
		if err := s.orders.SaveTransaction(ctx, paymentTransaction); err != nil {
			return nil, fmt.Errorf("settle payment transaction: %w", err)
		}
	}
	order, err := s.orders.MarkOrderPaid(ctx, orderNo, transactionID, platformFee, merchantPoints)
	if err != nil {
		return nil, err
	}
	if err := s.fulfillment.Fulfill(ctx, order); err != nil {
		return nil, fmt.Errorf("fulfill paid order: %w", err)
	}
	return &CallbackResult{
		OrderNo:       orderNo,
		TransactionID: transactionID,
		Status:        domain.StatusPaid,
		Fulfillment:   order.FulfillmentStatus,
	}, nil
}

func (s *Service) paymentForCallback(ctx context.Context, orderNo, transactionID string) (*domain.PaymentOrder, error) {
	if transactionID != "" {
		byTransaction, err := s.orders.GetPaymentOrderByTransactionID(ctx, transactionID)
		if err == nil && byTransaction != nil {
			return byTransaction, nil
		}
	}
	return s.orders.GetPaymentOrderByOrderNo(ctx, orderNo)
}

// callbackCompleted recognises the paid markers NodeLoc uses across its
// redirect and query payloads; anything else leaves the order pending.
func callbackCompleted(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "complete", "success", "succeeded", "paid", "trade_success", "1":
		return true
	}
	return false
}

func intParam(params map[string]string, keys ...string) int {
	if value := first(params, keys...); value != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil {
			return parsed
		}
	}
	return 0
}

func intPtrParam(params map[string]string, keys ...string) *int {
	for _, key := range keys {
		if value, ok := params[key]; ok {
			parsed, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return nil
			}
			return &parsed
		}
	}
	return nil
}

func (s *Service) FulfillOrder(ctx context.Context, orderNo string) (*models.Order, error) {
	order, err := s.orders.GetOrderByNo(ctx, strings.TrimSpace(orderNo))
	if err != nil {
		return nil, err
	}
	if err := s.fulfillment.Fulfill(ctx, order); err != nil {
		return nil, err
	}
	return s.orders.GetOrderByNo(ctx, order.OrderNo)
}

func (s *Service) Refund(ctx context.Context, input RefundInput) (*contract.TransferResult, error) {
	input.OrderNo = strings.TrimSpace(input.OrderNo)
	if input.OrderNo == "" || (strings.TrimSpace(input.ToUserID) == "" && strings.TrimSpace(input.ToUsername) == "") {
		return nil, ErrInvalidInput
	}
	order, err := s.orders.GetOrderByNo(ctx, input.OrderNo)
	if err != nil {
		return nil, err
	}
	if order.Status != "paid" && order.Status != "completed" {
		return nil, ErrInvalidInput
	}
	result, err := s.gateway.Transfer(ctx, contract.TransferRequest{
		ToUserID:   strings.TrimSpace(input.ToUserID),
		ToUsername: strings.TrimSpace(input.ToUsername),
		Amount:     order.TotalAmount,
		OrderID:    order.OrderNo,
	})
	if err != nil {
		return nil, fmt.Errorf("refund transfer: %w", err)
	}
	if result.Status != domain.StatusSucceeded {
		return nil, ErrPaymentNotComplete
	}
	if err := s.orders.MarkOrderRefunded(ctx, order.OrderNo); err != nil {
		return nil, err
	}
	paymentOrder, err := s.orders.GetPaymentOrderByOrderNo(ctx, order.OrderNo)
	if err == nil {
		now := time.Now().UTC()
		paymentOrder.Status = domain.StatusRefunded
		paymentOrder.RefundedAt = &now
		if saveErr := s.orders.SavePaymentOrder(ctx, paymentOrder); saveErr != nil {
			return nil, saveErr
		}
	}
	return result, nil
}

func (s *Service) GetOrder(ctx context.Context, userID uint, orderNo string) (*models.Order, error) {
	order, err := s.orders.GetOrderByNo(ctx, strings.TrimSpace(orderNo))
	if err != nil {
		return nil, err
	}
	if userID == 0 || order.UserID != userID {
		return nil, ErrForbidden
	}
	return order, nil
}

func (s *Service) AdminGetOrder(ctx context.Context, orderNo string) (*models.Order, error) {
	order, err := s.orders.GetOrderByNo(ctx, strings.TrimSpace(orderNo))
	if err != nil {
		return nil, err
	}
	return order, nil
}

func (s *Service) AdminListOrders(ctx context.Context, limit, offset int, status, search string) (*OrderList, error) {
	orders, total, err := s.orders.ListAllOrders(ctx, limit, offset, status, search)
	if err != nil {
		return nil, err
	}
	return &OrderList{Orders: orders, Total: total, Limit: limit, Offset: offset}, nil
}

func (s *Service) AdminCancelOrder(ctx context.Context, orderNo string) (*models.Order, error) {
	return s.orders.UpdateOrderStatus(ctx, strings.TrimSpace(orderNo), "cancelled")
}

func (s *Service) AdminDeliverOrder(ctx context.Context, orderNo string, content string) (*models.Order, error) {
	return s.orders.SetOrderDeliveryContent(ctx, strings.TrimSpace(orderNo), content)
}

func (s *Service) AdminRefundOrder(ctx context.Context, orderNo string) (*models.Order, error) {
	return s.orders.UpdateOrderStatus(ctx, strings.TrimSpace(orderNo), "refunded")
}

// orderListStatuses are the only ?status= values a buyer may filter by, so a
// stray query string narrows the list instead of silently returning nothing.
var orderListStatuses = map[string]bool{
	"pending": true, "paid": true, "completed": true,
	"cancelled": true, "refunded": true, "failed": true,
}

func (s *Service) ListOrders(ctx context.Context, userID uint, limit, offset int, status string) (*OrderList, error) {
	if userID == 0 {
		return nil, ErrForbidden
	}
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "" && !orderListStatuses[status] {
		status = ""
	}
	orders, total, err := s.orders.ListOrdersByUser(ctx, userID, limit, offset, status)
	if err != nil {
		return nil, err
	}
	return &OrderList{Orders: orders, Total: total, Limit: limit, Offset: offset}, nil
}

func first(params map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(params[key]); value != "" {
			return value
		}
	}
	return ""
}

func stringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

// newOrderNo builds a collision-resistant order number accepted by NodeLoc
// (alphanumeric, stable across retries, short enough for the callback payload).
func newOrderNo() string {
	suffix := make([]byte, 5)
	if _, err := rand.Read(suffix); err != nil {
		return fmt.Sprintf("NL%d", time.Now().UnixNano())
	}
	return "NL" + time.Now().UTC().Format("20060102150405") + strings.ToUpper(hex.EncodeToString(suffix))
}
