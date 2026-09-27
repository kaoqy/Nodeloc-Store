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
	ErrPaymentUnsettled   = errors.New("provider has no completed payment for this order")
	// ErrNoProviderTransaction means the store never recorded a NodeLoc
	// transaction id for this order, so 查单 has nothing to ask about. It is a
	// different problem from "the provider says unpaid" and needs a different
	// next step from the buyer (start the payment again).
	ErrNoProviderTransaction = errors.New("store has no NodeLoc transaction id for this order")
	// ErrForeignTransaction refuses to settle a transaction the provider names
	// for some other order.
	ErrForeignTransaction = errors.New("provider transaction belongs to another order")
	// ErrRefundRecipientUnknown reaches the back office verbatim, so it names
	// what the admin can actually do next.
	ErrRefundRecipientUnknown = errors.New("该买家没有绑定 NodeLoc 账号，积分无从退回；请先在订单详情里人工处理")
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

// ReconcileResult is what a 查单 established. "Provider says unpaid" is a normal
// answer rather than a failure, so it comes back here instead of as an error:
// the storefront can then say what the buyer should do next — keep waiting, pay
// again, or contact the shop — instead of one generic retry message.
type ReconcileResult struct {
	Order          *models.Order `json:"order"`
	Settled        bool          `json:"settled"`
	ProviderStatus string        `json:"provider_status,omitempty"`
	Retryable      bool          `json:"retryable"`
	CheckedAt      time.Time     `json:"checked_at"`
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
	// provider_transaction_id is unique, and a checkout whose response carried no
	// id would store an empty one — so two such orders would collide on the index
	// and the second payment attempt would fail with a database error.
	if result.TransactionID != "" {
		transaction.ProviderTransactionID = stringPointer(result.TransactionID)
	}
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
//
// A redirect that does not verify is the other case: none of its fields may be
// trusted, so the order is settled only if the provider's own query API names
// it as paid. That fallback is what stops a buyer who really paid from being
// told to pay again over one differently-encoded parameter.
func (s *Service) HandleCallback(ctx context.Context, params map[string]string) (*CallbackResult, error) {
	orderNo := first(params, "external_reference", "order_id", "order_no", "out_trade_no")
	transactionID := first(params, "transaction_id", "trade_no", "id")

	if !s.gateway.VerifyCallback(params) {
		return nil, ErrInvalidCallback
	}
	if orderNo == "" {
		return nil, ErrInvalidInput
	}

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

	order, err := s.settle(ctx, orderNo, paymentOrder, transactionID, platformFee, merchantPoints)
	if err != nil {
		return nil, err
	}
	return newCallbackResult(order), nil
}

// HandleCallbackSets settles one callback from every encoding of its payload.
// A value as plain as '+' inside paid_at survives NodeLoc's signing but not
// query-string decoding, so the same callback arrives as two candidate sets:
// 查单 is the last resort, tried only once none of them verifies.
func (s *Service) HandleCallbackSets(ctx context.Context, sets []map[string]string) (*CallbackResult, error) {
	for _, params := range sets {
		result, err := s.HandleCallback(ctx, params)
		if err == nil || !errors.Is(err, ErrInvalidCallback) {
			return result, err
		}
	}
	if len(sets) == 0 {
		return nil, ErrInvalidInput
	}

	orderNo := first(sets[0], "external_reference", "order_id", "order_no", "out_trade_no")
	transactionID := first(sets[0], "transaction_id", "trade_no", "id")
	result, err := s.reconcile(ctx, orderNo, transactionID, 0)
	if err != nil {
		log.Printf("payment callback %s: no signature candidate verified and the provider query did not settle it: %v", orderNo, err)
		return nil, err
	}
	if !result.Settled {
		log.Printf("payment callback %s: no signature candidate verified and NodeLoc reports the payment as %s", orderNo, result.ProviderStatus)
		return nil, fmt.Errorf("%w: provider query reports %s", ErrPaymentUnsettled, result.ProviderStatus)
	}
	log.Printf("payment callback %s: settled from provider query after every signature candidate failed", orderNo)
	return newCallbackResult(result.Order), nil
}

// settle writes the provider's confirmation onto the payment rows, marks the
// order paid and runs delivery, then hands back the order as it now reads —
// including any card content delivery just wrote. Re-running it for the same
// order is safe: MarkOrderPaid leaves settled orders alone.
func (s *Service) settle(ctx context.Context, orderNo string, paymentOrder *domain.PaymentOrder, transactionID string, platformFee, merchantPoints *int) (*models.Order, error) {
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
		// NodeLoc has confirmed the money by now. Returning this error told the
		// buyer the payment could not be checked — the one message that makes
		// people pay twice — so the order stays paid, the note explains the wait,
		// and the delivery worker retries it.
		log.Printf("payment settle %s: paid, delivery failed and was queued for retry: %v", orderNo, err)
		if noteErr := s.orders.MarkOrderDeliveryPending(ctx, orderNo, "支付已确认，商品交付遇到一点问题，商店会自动重试，通常几分钟内送达。"); noteErr != nil {
			log.Printf("payment settle %s: could not note the delivery retry: %v", orderNo, noteErr)
		}
	}
	fresh, err := s.orders.GetOrderByNo(ctx, orderNo)
	if err != nil {
		log.Printf("payment settle %s: reload after fulfillment failed: %v", orderNo, err)
		return order, nil
	}
	return fresh, nil
}

// ReconcileOrder is the 查单 path: the buyer or the back office asks what the
// provider recorded for a local order, and a completed payment is settled and
// delivered on the spot. userID 0 means an operator, who may reconcile any
// order.
func (s *Service) ReconcileOrder(ctx context.Context, orderNo string, userID uint) (*ReconcileResult, error) {
	return s.reconcile(ctx, orderNo, "", userID)
}

func (s *Service) reconcile(ctx context.Context, orderNo, hintedTransactionID string, userID uint) (*ReconcileResult, error) {
	orderNo = strings.TrimSpace(orderNo)
	if orderNo == "" {
		return nil, ErrInvalidInput
	}
	order, err := s.orders.GetOrderByNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	if userID != 0 && order.UserID != userID {
		return nil, ErrForbidden
	}
	switch order.Status {
	case "pending":
	case "paid", "completed":
		// Already settled here: re-running 查单 is how the storefront refreshes,
		// so report the order as settled instead of asking the provider again.
		return &ReconcileResult{Order: order, Settled: true, CheckedAt: time.Now().UTC()}, nil
	default:
		return nil, fmt.Errorf("%w: order status is %s", ErrPaymentUnsettled, order.Status)
	}

	paymentOrder, err := s.orders.GetPaymentOrderByOrderNo(ctx, orderNo)
	if err != nil {
		if errors.Is(err, domain.ErrPaymentOrderNotFound) {
			return nil, ErrNoProviderTransaction
		}
		return nil, err
	}

	// Our own payment row is the primary candidate; an id that only arrives with
	// the request is accepted solely when the provider echoes this order back, so
	// a guessed transaction id can never settle someone else's order.
	candidates := []queryCandidate{{transaction: derefString(paymentOrder.ProviderTransactionID), owned: true}}
	if hinted := strings.TrimSpace(hintedTransactionID); hinted != "" && hinted != candidates[0].transaction {
		candidates = append(candidates, queryCandidate{transaction: hinted})
	}

	for _, candidate := range candidates {
		if candidate.transaction == "" {
			continue
		}
		query, queryErr := s.gateway.QueryPayment(ctx, candidate.transaction)
		if queryErr != nil {
			if candidate.owned {
				// Whether NodeLoc is unreachable or refusing the shop's
				// credentials decides what the buyer is told, so do not mask it
				// behind a hint that was never ours to begin with.
				return nil, queryErr
			}
			continue
		}
		if query == nil {
			return nil, fmt.Errorf("%w: NodeLoc returned an unreadable query result", domain.ErrProviderUnreachable)
		}
		if query.OrderID != "" && query.OrderID != orderNo {
			if candidate.owned {
				return nil, ErrForeignTransaction
			}
			continue
		}
		checkedAt := time.Now().UTC()
		if !providerCompleted(query.Status) {
			return &ReconcileResult{
				Order: order,
				// A payment NodeLoc already failed will not settle later; one still
				// open is worth another look, which is what drives the retry copy.
				ProviderStatus: query.Status,
				Retryable:      !providerFailed(query.Status),
				CheckedAt:      checkedAt,
			}, nil
		}
		if query.Amount != 0 && query.Amount != paymentOrder.Amount {
			return nil, ErrAmountMismatch
		}
		log.Printf("payment reconcile %s: provider confirmed %s as paid", orderNo, candidate.transaction)
		settled, err := s.settle(ctx, orderNo, paymentOrder, candidate.transaction, query.PlatformFee, query.MerchantPoints)
		if err != nil {
			return nil, err
		}
		return &ReconcileResult{
			Order:          settled,
			Settled:        settled.Status != "pending",
			ProviderStatus: query.Status,
			CheckedAt:      checkedAt,
		}, nil
	}

	// Nothing to ask about: the store never got a transaction id for this order,
	// so the buyer has to start the payment again rather than keep polling.
	return nil, ErrNoProviderTransaction
}

type queryCandidate struct {
	transaction string
	owned       bool
}

// providerCompleted reads back the gateway's normalised query status.
func providerCompleted(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case domain.StatusSucceeded, domain.StatusPaid:
		return true
	}
	return false
}

// providerFailed is the terminal counterpart: NodeLoc will never mark this
// payment paid, so "check again later" would be a lie.
func providerFailed(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case domain.StatusFailed, domain.StatusCancelled, domain.StatusRefunded, "expired", "closed", "timeout":
		return true
	}
	return false
}

func newCallbackResult(order *models.Order) *CallbackResult {
	return &CallbackResult{
		OrderNo:       order.OrderNo,
		TransactionID: derefString(order.TransactionID),
		Status:        order.Status,
		Fulfillment:   order.FulfillmentStatus,
	}
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
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

// deliveryRetryLimit bounds one background sweep so a long backlog cannot pin
// the loop on a cold database.
const deliveryRetryLimit = 50

// RetryPendingDeliveries hands orders that NodeLoc confirmed as paid but that
// never finished delivery back to the fulfillment store — automatic card
// delivery, the manual queue, and stock that arrived after 等待补货. It returns
// how many orders moved on.
func (s *Service) RetryPendingDeliveries(ctx context.Context) (int, error) {
	pending, err := s.orders.ListUndeliveredPaidOrders(ctx, deliveryRetryLimit)
	if err != nil {
		return 0, err
	}
	delivered := 0
	for i := range pending {
		order := pending[i]
		if err := s.fulfillment.Fulfill(ctx, &order); err != nil {
			log.Printf("delivery retry %s: still undelivered: %v", order.OrderNo, err)
			continue
		}
		if order.FulfillmentStatus == "delivered" || order.FulfillmentStatus == "completed" ||
			order.FulfillmentStatus == "manual_pending" || order.FulfillmentStatus == "waiting_stock" {
			delivered++
		}
	}
	return delivered, nil
}

// reconcileBatchLimit caps one 批量查单 sweep; the admin runs it again for more.
const reconcileBatchLimit = 20

// The background sweep is deliberately smaller and older: it exists to catch
// abandoned checkouts, not to poll NodeLoc over orders whose buyer is watching.
const (
	autoReconcileLimit  = 10
	autoReconcileMinAge = 10 * time.Minute
)

type ReconcileItem struct {
	OrderNo        string `json:"order_no"`
	Settled        bool   `json:"settled"`
	ProviderStatus string `json:"provider_status,omitempty"`
	Code           string `json:"code,omitempty"`
	Message        string `json:"message,omitempty"`
	Detail         string `json:"detail,omitempty"`
}

type ReconcileReport struct {
	CheckedAt time.Time       `json:"checked_at"`
	Checked   int             `json:"checked"`
	Settled   int             `json:"settled"`
	Items     []ReconcileItem `json:"items"`
}

// AdminReconcilePending is the back-office sweep: every order still marked
// 待支付 that carries a NodeLoc transaction id gets asked about right now.
func (s *Service) AdminReconcilePending(ctx context.Context) (*ReconcileReport, error) {
	return s.reconcilePending(ctx, reconcileBatchLimit, 0)
}

// AutoReconcilePending runs the same sweep in the background, which is what
// settles an order whose browser redirect never came back without anyone
// pressing a button. The age floor keeps an open checkout page out of it — that
// page already polls the provider on the buyer's behalf.
func (s *Service) AutoReconcilePending(ctx context.Context) (*ReconcileReport, error) {
	return s.reconcilePending(ctx, autoReconcileLimit, autoReconcileMinAge)
}

func (s *Service) reconcilePending(ctx context.Context, limit int, minAge time.Duration) (*ReconcileReport, error) {
	candidates, err := s.orders.ListReconcilableOrders(ctx, limit, minAge)
	if err != nil {
		return nil, err
	}
	report := &ReconcileReport{CheckedAt: time.Now().UTC(), Checked: len(candidates), Items: make([]ReconcileItem, 0, len(candidates))}
	var firstErr string
	for _, order := range candidates {
		item := ReconcileItem{OrderNo: order.OrderNo}
		result, reconcileErr := s.reconcile(ctx, order.OrderNo, "", 0)
		switch {
		case reconcileErr != nil:
			failure := Classify(reconcileErr)
			item.Code = failure.Code
			item.Message = failure.Message
			item.Detail = failure.Detail
			if firstErr == "" {
				firstErr = fmt.Sprintf("%s: %v", failure.Code, reconcileErr)
			}
		case result.Settled:
			item.Settled = true
			item.ProviderStatus = result.ProviderStatus
			report.Settled++
		default:
			item.ProviderStatus = result.ProviderStatus
		}
		report.Items = append(report.Items, item)
	}
	// One summary line per sweep: a provider outage would otherwise log a line
	// for every open order, every few minutes.
	if report.Checked > 0 {
		if firstErr != "" {
			log.Printf("payment auto-reconcile: checked %d, settled %d, first failure %s", report.Checked, report.Settled, firstErr)
		} else if report.Settled > 0 {
			log.Printf("payment auto-reconcile: checked %d, settled %d", report.Checked, report.Settled)
		}
	}
	return report, nil
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

// AdminListOrders lists orders for the back office. attention="undelivered"
// swaps the status filter for "paid but still owed a delivery", the queue that
// needs a person once the automatic retries have had their turns.
func (s *Service) AdminListOrders(ctx context.Context, limit, offset int, status, search string, buyerID uint, attention string) (*OrderList, error) {
	if attention != "" && attention != "undelivered" {
		return nil, fmt.Errorf("%w: unknown attention filter %q", ErrInvalidInput, attention)
	}
	orders, total, err := s.orders.ListAllOrders(ctx, limit, offset, status, search, buyerID, attention)
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

// AdminRefundOrder moves the points back through NodeLoc before the shop calls
// the order refunded. Marking it locally only would tell the buyer their money
// is on the way while NodeLoc still shows it as spent.
func (s *Service) AdminRefundOrder(ctx context.Context, orderNo string) (*models.Order, error) {
	orderNo = strings.TrimSpace(orderNo)
	order, err := s.orders.GetOrderByNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	user, err := s.users.FindByID(ctx, order.UserID)
	if err != nil {
		return nil, fmt.Errorf("lookup refund recipient: %w", err)
	}
	if user == nil || (user.OAuthUID == "" && user.OAuthUsername == "") {
		return nil, ErrRefundRecipientUnknown
	}
	if _, err := s.Refund(ctx, RefundInput{
		OrderNo:    orderNo,
		ToUserID:   user.OAuthUID,
		ToUsername: user.OAuthUsername,
	}); err != nil {
		return nil, err
	}
	return s.orders.GetOrderByNo(ctx, orderNo)
}

// orderListStatuses are the only ?status= values a buyer may filter by, so a
// stray query string narrows the list instead of silently returning nothing.
var orderListStatuses = map[string]bool{
	"pending": true, "paid": true, "completed": true,
	"cancelled": true, "refunded": true, "failed": true,
}

func (s *Service) ListOrders(ctx context.Context, userID uint, limit, offset int, status, search string) (*OrderList, error) {
	if userID == 0 {
		return nil, ErrForbidden
	}
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "" && !orderListStatuses[status] {
		status = ""
	}
	orders, total, err := s.orders.ListOrdersByUser(ctx, userID, limit, offset, status, strings.TrimSpace(search))
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
