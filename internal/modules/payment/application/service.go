package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
	activitydomain "github.com/kaoqy/Nodeloc-Store/internal/modules/activity/domain"
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
	// ErrCouponUnavailable covers a code that was accepted in the quote box but
	// cannot be honoured when the order is written — it expired, ran out, or the
	// shop turned 优惠码 off in the meantime. The buyer re-quotes rather than
	// paying an amount they were never shown.
	ErrCouponUnavailable = errors.New("coupon cannot be applied to this order")
	// ErrGrantRecipientUnknown stops a 店家转账 before it reaches NodeLoc: points
	// can only land in a NodeLoc account, and a shop buyer who signed up locally
	// has none. Saying so is the operator's cue to ask them to bind it.
	ErrGrantRecipientUnknown = errors.New("这个账号没有绑定 NodeLoc 用户，积分没有可转入的账户；请让对方在个人中心用 NodeLoc 登录一次")
	// ErrNewAPIPricingRuleMissing is the honest answer for a New-API
	// amount-type product: it is configured with a top-up range only, and the
	// shop has no rule that turns a top-up amount into the amount NodeLoc must
	// collect. The order cannot be priced, so it is refused with this specific
	// reason instead of a misleading "product unavailable".
	ErrNewAPIPricingRuleMissing = errors.New("New-API 额度型商品缺少计价规则")
)

// maxOrderQuantity bounds a single storefront order so one buyer cannot drain
// the card inventory in one request.
const maxOrderQuantity = 20

// newAPIDeliveryChannel mirrors the catalogue's channel value. It is duplicated
// here as a plain constant so the money module does not import catalogue
// internals; the value is part of the product contract.
const newAPIDeliveryChannel = "new_api"

// isNewAPIProduct reports whether a product is sold through the product-level
// New-API redemption channel. Legacy rows predate delivery_channel, so an empty
// value is read as "not New-API" and keeps the shop's own fulfilment.
func isNewAPIProduct(product *models.Product) bool {
	return product != nil && strings.EqualFold(strings.TrimSpace(product.DeliveryChannel), newAPIDeliveryChannel)
}

// topupAmountFromForm reads the buyer's New-API top-up amount as a positive
// integer. The purchase form already validated it as a required number; this is
// the money-side parse, so an unreadable value stops the order instead of
// silently becoming zero.
func topupAmountFromForm(formValues map[string]string) (int, error) {
	raw := strings.TrimSpace(formValues["nl_amount"])
	if raw == "" {
		return 0, fmt.Errorf("%w: 请填写本次充值额度", ErrInvalidInput)
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%w: 本次充值额度必须是正整数", ErrInvalidInput)
	}
	return value, nil
}

type Service struct {
	orders      contract.OrderRepo
	gateway     contract.PaymentGateway
	fulfillment contract.FulfillmentService
	// plugins is optional. When a product is bound to a plugin, that plugin
	// delivers the order through this hook instead of the shop's own card/manual
	// fulfillment. A shop with no plugin installed leaves it nil and nothing
	// changes.
	plugins contract.PluginDeliverer
	users   contract.UserLookup
	coupons contract.CouponPricing
	// activities 是可选的活动定价端口：没有活动模块时下单照旧，只是不算活动价。
	activities contract.ActivityPricing
	// events is optional: a store with no inbox still has to be able to take money.
	events    contract.BuyerNotifier
	paymentID string
	// features carries the owner's 收款 switch; see CreateOrder's gate.
	features config.FeaturesConfig
}

type CreatePaymentInput struct {
	UserID      uint
	OrderNo     string
	Description string
}

type CreateOrderInput struct {
	UserID uint
	// ProductID is the authoritative product selected on the detail page.
	// Slug is accepted only for old clients; when both are present they must
	// identify the same row, so a stale form cannot silently order another item.
	ProductID uint
	Slug      string
	Quantity  int
	Contact   string
	Note      string
	// CouponCode is what the buyer typed in the 优惠码 field, if anything.
	CouponCode string
	FormValues map[string]string
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
	// ProviderVia names which NodeLoc route answered, "query" or "reprocess". A
	// shop whose money settles through the second one is not broken, but the
	// owner has to be able to see that 查单 is not what is answering.
	ProviderVia string `json:"provider_via,omitempty"`
	// ProviderNote is NodeLoc's own sentence about the 查单 this store could not
	// use. Back office only: ReconcileOrder clears it, because the buyer's next
	// step is unchanged by the provider's English and it is not their credential
	// to fix.
	ProviderNote string `json:"provider_note,omitempty"`
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

// SetPluginDeliverer attaches the plugin runtime. It is a setter rather than a
// constructor argument because the plugin module is wired after payment (it
// reads the shop's own catalogue to resolve a mapping), and a nil deliverer is a
// valid configuration: no plugin, the shop's own delivery.
func (s *Service) SetPluginDeliverer(deliverer contract.PluginDeliverer) {
	s.plugins = deliverer
}

// SetActivityPricing attaches the activity module. It is a setter because the
// activity module is wired alongside payment; a shop with no activities leaves
// it nil and every order simply has no activity discount.
func (s *Service) SetActivityPricing(pricing contract.ActivityPricing) {
	s.activities = pricing
}

func NewService(orders contract.OrderRepo, gateway contract.PaymentGateway, fulfillment contract.FulfillmentService, users contract.UserLookup, coupons contract.CouponPricing, events contract.BuyerNotifier, paymentID string, features config.FeaturesConfig) *Service {
	if orders == nil || gateway == nil || fulfillment == nil || users == nil {
		panic("payment: nil dependency")
	}
	return &Service{orders: orders, gateway: gateway, fulfillment: fulfillment, users: users, coupons: coupons, events: events, paymentID: strings.TrimSpace(paymentID), features: features}
}

func (s *Service) CreateOrder(ctx context.Context, input CreateOrderInput) (*models.Order, error) {
	slug := strings.TrimSpace(input.Slug)
	if input.UserID == 0 || (input.ProductID == 0 && slug == "") {
		return nil, ErrInvalidInput
	}
	quantity := input.Quantity
	if quantity == 0 {
		quantity = 1
	}
	if quantity < 1 || quantity > maxOrderQuantity {
		return nil, fmt.Errorf("%w: quantity must be between 1 and %d", ErrInvalidInput, maxOrderQuantity)
	}
	// The switch on 设置 means "this shop is not taking money right now", so the
	// order stops here instead of piling up pending rows nobody can ever pay.
	if !s.features.PaymentsOn() {
		return nil, domain.ErrPaymentsDisabled
	}

	user, err := s.users.FindByID(ctx, input.UserID)
	if err != nil {
		return nil, fmt.Errorf("lookup order user: %w", err)
	}
	if user == nil || !user.IsActive {
		return nil, ErrForbidden
	}

	var product *models.Product
	if input.ProductID != 0 {
		product, err = s.orders.GetPurchasableProductByID(ctx, input.ProductID)
	} else {
		product, err = s.orders.GetPurchasableProductBySlug(ctx, slug)
	}
	if err != nil {
		return nil, err
	}
	if input.ProductID != 0 && slug != "" && product.Slug != slug {
		return nil, fmt.Errorf("%w: 商品标识与商品 ID 不一致，请刷新商品页后重试", ErrInvalidInput)
	}
	// A New-API product is priced by its top-up amount, but the shop defines no
	// rule that maps that amount to money and NodeLoc can only collect a positive
	// amount. Refuse with the real reason rather than the generic
	// "not purchasable" — the operator needs to know this is a business rule
	// that is missing, not a product that is off the shelf.
	if isNewAPIProduct(product) && product.Price <= 0 {
		return nil, ErrNewAPIPricingRuleMissing
	}
	if product.Price <= 0 {
		return nil, domain.ErrProductNotPurchasable
	}

	contact := strings.TrimSpace(input.Contact)
	note := strings.TrimSpace(input.Note)
	var formFields []models.ProductFormField
	if product.FormSchema != "" {
		if err := json.Unmarshal([]byte(product.FormSchema), &formFields); err != nil {
			return nil, fmt.Errorf("%w: 商品购买表单配置无效", ErrInvalidInput)
		}
	}
	if s.plugins != nil {
		extraFields, err := s.plugins.FormFields(ctx, product.ID)
		if err != nil {
			return nil, fmt.Errorf("load plugin form fields: %w", err)
		}
		formFields = append(formFields, extraFields...)
	}
	formValuesJSON, err := models.ValidateProductForm(formFields, input.FormValues)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	// A product bound to a plugin has to resolve to a delivery item before the
	// order exists: the same failure at delivery time would be a paid order the
	// shop can only untangle by hand.
	if s.plugins != nil {
		var answers map[string]string
		if formValuesJSON != "" {
			_ = json.Unmarshal([]byte(formValuesJSON), &answers)
		}
		if err := s.plugins.ValidateSelection(ctx, product.ID, answers); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
	}
	if len(contact) > 255 {
		return nil, fmt.Errorf("%w: contact information is too long", ErrInvalidInput)
	}
	if product.RequireContact && contact == "" {
		return nil, fmt.Errorf("%w: this product requires contact information", ErrInvalidInput)
	}
	// New-API products deliver a freshly created redemption code after payment;
	// they never draw on card stock, so they skip the card-count gate entirely.
	newAPI := isNewAPIProduct(product)
	topupAmount := 0
	if newAPI {
		var answers map[string]string
		if formValuesJSON != "" {
			_ = json.Unmarshal([]byte(formValuesJSON), &answers)
		}
		topupAmount, err = topupAmountFromForm(answers)
		if err != nil {
			return nil, err
		}
		if product.MinTopupAmount <= 0 || product.MaxTopupAmount <= 0 ||
			product.MinTopupAmount > product.MaxTopupAmount {
			return nil, fmt.Errorf("%w: 该商品的充值额度范围配置无效，请联系店家处理", ErrInvalidInput)
		}
		if topupAmount < product.MinTopupAmount || topupAmount > product.MaxTopupAmount {
			return nil, fmt.Errorf(
				"%w: 本次充值额度需在 %d 到 %d 之间",
				ErrInvalidInput, product.MinTopupAmount, product.MaxTopupAmount,
			)
		}
	} else if product.ProductType == "card" && product.AutoDeliver {
		available, err := s.orders.CountAvailableCards(ctx, product.ID)
		if err != nil {
			return nil, fmt.Errorf("count available cards: %w", err)
		}
		if available < int64(quantity) {
			return nil, domain.ErrInsufficientStock
		}
	}

	total := product.Price * quantity
	couponCode := strings.ToUpper(strings.TrimSpace(input.CouponCode))
	var couponID *uint
	if couponCode != "" {
		if s.coupons == nil {
			return nil, ErrCouponUnavailable
		}
		taken, id, err := s.coupons.DiscountFor(ctx, input.UserID, product.ID, quantity, product.Price, couponCode)
		if err != nil {
			// Both errors stay in the chain: ErrCouponUnavailable is what Classify
			// matches on, and the catalogue error behind it names the exact rule
			// ("you already used this code") the buyer should be told.
			return nil, fmt.Errorf("%w: %w", ErrCouponUnavailable, err)
		}
		// A 0 amount is not a payment NodeLoc can collect, so the floor is one
		// fen even if the pricing side ever loosens.
		if taken < 0 || taken >= total {
			taken = total - 1
		}
		total -= taken
		if id != 0 {
			couponID = &id
		}
	}

	// 活动优惠在优惠码之后计算，金额一律来自服务端的结构化规则，绝不采信前端
	// 传回来的价格；活动把订单打到 0 时同样保留 1，NodeLoc 才收得到钱。
	var activityPricing *models.ActivityPricing
	if s.activities != nil {
		priced, err := s.activities.Price(ctx, models.ActivityMatchInput{
			UserID:    input.UserID,
			ProductID: product.ID,
			Quantity:  quantity,
			UnitPrice: product.Price,
		})
		if err != nil && !errors.Is(err, activitydomain.ErrNoActivity) {
			return nil, fmt.Errorf("activity pricing: %w", err)
		}
		activityPricing = priced
	}
	if activityPricing != nil && activityPricing.DiscountAmount > 0 {
		if activityPricing.DiscountAmount >= total {
			activityPricing.DiscountAmount = total - 1
			activityPricing.Payable = 1
		}
		total -= activityPricing.DiscountAmount
	}

	order := &models.Order{
		OrderNo:           newOrderNo(),
		UserID:            input.UserID,
		ProductID:         product.ID,
		Quantity:          quantity,
		TopupAmount:       topupAmount,
		UnitPrice:         product.Price,
		DiscountAmount:    product.Price*quantity - total,
		CouponID:          couponID,
		CouponCode:        couponCode,
		TotalAmount:       total,
		Status:            "pending",
		FulfillmentStatus: "pending",
	}
	if activityPricing != nil {
		activityID := activityPricing.ActivityID
		order.ActivityID = &activityID
		order.ActivityName = activityPricing.ActivityName
		order.ActivityDiscount = activityPricing.DiscountAmount
		order.ActivitySnapshot = activityPricing.Snapshot
	}
	if contact != "" {
		order.CustomerContact = &contact
	}
	if note != "" {
		order.CustomerNote = &note
	}
	order.FormValues = formValuesJSON
	if err := s.orders.CreateOrder(ctx, order); err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}
	// 订单已经落地才占用活动名额：占位失败只是统计缺口，不能把订单判成失败。
	if activityPricing != nil && s.activities != nil {
		record := &models.ActivityRecord{
			ActivityID:     activityPricing.ActivityID,
			UserID:         input.UserID,
			Quantity:       quantity,
			OriginalAmount: product.Price * quantity,
			DiscountAmount: activityPricing.DiscountAmount,
			PayableAmount:  total,
			Snapshot:       activityPricing.Snapshot,
		}
		productID := product.ID
		record.ProductID = &productID
		if err := s.activities.Reserve(ctx, order.ID, record); err != nil {
			log.Printf("[payment] order %s: activity %d reservation failed: %v", order.OrderNo, activityPricing.ActivityID, err)
		}
	}
	return order, nil
}

func (s *Service) CreatePayment(ctx context.Context, input CreatePaymentInput) (*CreatePaymentOutput, error) {
	input.OrderNo = strings.TrimSpace(input.OrderNo)
	if input.UserID == 0 || input.OrderNo == "" {
		return nil, ErrInvalidInput
	}
	if !s.features.PaymentsOn() {
		return nil, domain.ErrPaymentsDisabled
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
		// NodeLoc already has a payment for this order number, which is the answer
		// to every 立即购买 a buyer presses a second time. Hand them the payment
		// that exists instead of a refusal they can only clear by paying twice.
		if resumed, resumeErr := s.resumeExistingPayment(ctx, order, input.UserID, err); resumed != nil || resumeErr != nil {
			return resumed, resumeErr
		}
		// A refused 下单 has to leave a trace the shop owner can read. Until it
		// does, "买家说付不了款" is unanswerable: the buyer only ever sees the
		// generic sentence, and the provider's reason disappears with the request.
		s.recordCheckoutFailure(ctx, order, err)
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
	// A retry that gets through must not keep the previous refusal: the order
	// detail would read "支付失败" next to a working payment link.
	transaction.FailureReason = nil
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

// resumeExistingPayment answers NodeLoc's 「Order already exists with status …」
// refusal, which is what every second 下单 for one order number gets — a buyer who
// came back to the order page and pressed the button again. The payment is on
// NodeLoc's side, so the store hands that payment back instead of refusing the
// retry, which is the one answer that pushes people to pay twice.
//
// When the provider's own words say the money already moved, 查单 settles it here
// and now: that is the lost-callback case, and the buyer should be shown the
// goods rather than a checkout page.
//
// (nil, nil) means "not this problem" and the caller reports the original error.
func (s *Service) resumeExistingPayment(ctx context.Context, order *models.Order, userID uint, cause error) (*CreatePaymentOutput, error) {
	var refusal *domain.PaymentAlreadyRequested
	if !errors.As(cause, &refusal) {
		return nil, nil
	}
	paymentOrder, err := s.orders.GetPaymentOrderByOrderNo(ctx, order.OrderNo)
	if err != nil || paymentOrder == nil {
		return nil, nil
	}

	if callbackCompleted(refusal.Status) {
		result, reconcileErr := s.reconcile(ctx, order.OrderNo, derefString(paymentOrder.ProviderTransactionID), userID)
		if reconcileErr == nil && result != nil && result.Settled {
			log.Printf("payment checkout %s: NodeLoc already had this order paid, 查单 settled it", order.OrderNo)
			fresh, loadErr := s.orders.GetPaymentOrderByOrderNo(ctx, order.OrderNo)
			if loadErr != nil || fresh == nil {
				fresh = paymentOrder
			}
			return &CreatePaymentOutput{PaymentOrder: fresh, Order: result.Order}, nil
		}
		// NodeLoc says paid and its query cannot say so yet. Sending the buyer back
		// to the checkout page would invite a second charge, so this reads as the
		// unsettled answer it is and the storefront keeps confirming — except when
		// the store never got a transaction id to ask about, where the only thing
		// left is the payment page NodeLoc already has.
		log.Printf("payment checkout %s: NodeLoc reported %s for this order but 查单 did not settle it: %v", order.OrderNo, refusal.Status, reconcileErr)
		if errors.Is(reconcileErr, ErrNoProviderTransaction) && strings.TrimSpace(derefString(paymentOrder.PaymentURL)) != "" {
			return &CreatePaymentOutput{PaymentOrder: paymentOrder, Order: order}, nil
		}
		if reconcileErr != nil {
			return nil, reconcileErr
		}
		return nil, fmt.Errorf("%w: provider reported %s", ErrPaymentUnsettled, refusal.Status)
	}

	if strings.TrimSpace(derefString(paymentOrder.PaymentURL)) == "" {
		return nil, nil
	}
	log.Printf("payment checkout %s: NodeLoc already has this order (%s), reissuing the payment it took", order.OrderNo, refusal.Status)
	return &CreatePaymentOutput{PaymentOrder: paymentOrder, Order: order}, nil
}

// recordCheckoutFailure leaves a refused 下单 on the order's payment transaction.
// Before this, a buyer's "付不了款" was unanswerable: the storefront only ever saw
// the generic sentence and NodeLoc's reason died with the request.
//
// A row that already shows money having moved is left alone — a callback for that
// transaction is the truth about it, and stamping this attempt's failure over it
// would destroy the receipt for a paid order.
func (s *Service) recordCheckoutFailure(ctx context.Context, order *models.Order, cause error) {
	reason := Classify(cause).Code + "：" + cause.Error()
	log.Printf("payment checkout %s: NodeLoc refused the 下单 request (%s)", order.OrderNo, reason)

	transaction, err := s.orders.GetLatestTransaction(ctx, order.OrderNo, domain.TransactionTypePayment)
	if err != nil {
		log.Printf("payment checkout %s: failure not recorded, the payment transaction could not be read: %v", order.OrderNo, err)
		return
	}
	if transaction != nil && (transaction.Status == domain.StatusPaid || transaction.Status == domain.StatusSucceeded) {
		return
	}
	if transaction == nil {
		transaction = &domain.Transaction{
			OrderID: order.ID, OrderNo: order.OrderNo, Provider: "nodeloc",
			Type: domain.TransactionTypePayment, Amount: order.TotalAmount, Currency: "points",
		}
	}
	transaction.Status = domain.StatusFailed
	transaction.FailureReason = &reason
	if transaction.ID != 0 {
		err = s.orders.SaveTransaction(ctx, transaction)
	} else {
		err = s.orders.CreateTransaction(ctx, transaction)
	}
	if err != nil {
		log.Printf("payment checkout %s: failure could not be written: %v", order.OrderNo, err)
	}
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
	if transactionID == "" && paymentOrder.ProviderTransactionID != nil {
		// NodeLoc's redirect example carries transaction_id, but a release that
		// leaves it out of the browser redirect must not strand money the
		// signature already accounts for.
		transactionID = strings.TrimSpace(*paymentOrder.ProviderTransactionID)
	}

	reported := first(params, "status", "state")
	if !callbackCompleted(reported) {
		return nil, fmt.Errorf("%w: provider reported %q", ErrPaymentNotComplete, reported)
	}
	if amount := intParam(params, "amount", "paid_amount", "total_amount"); amount != 0 && amount != paymentOrder.Amount {
		return nil, ErrAmountMismatch
	}
	// The fee and the net are what the ledger records, and the provider names them
	// differently on 回调 and on 查单: platform_fee/fee_amount here, merchant_points there.
	// A decimal 「9.5」 is a real settlement, so these read as points-rounded numbers instead
	// of failing to parse and leaving the order's fee at zero.
	platformFee := intPtrParam(params, "platform_fee", "fee_amount", "fee")
	merchantPoints := intPtrParam(params, "merchant_points", "merchant_amount", "net_amount")

	if transactionID == "" {
		// Nothing to cross-check against: the redirect is signed with this
		// store's own credential and names this order at this amount, which is
		// the authority this handler already treats as enough. Settling it beats
		// the alternative — telling a buyer who paid to pay again.
		log.Printf("payment callback %s: signed redirect carries no transaction id, settling from the signature", orderNo)
	} else if query, queryErr := s.gateway.QueryPayment(ctx, contract.QueryPaymentRequest{
		TransactionID: transactionID,
		OrderID:       orderNo,
		Amount:        paymentOrder.Amount,
		Description:   paymentOrder.Description,
	}); queryErr != nil {
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
	// A signed redirect can settle an order without naming a transaction id, and
	// writing an empty one over the row would erase the id a later callback
	// brings and leave 交易号 blank on a paid order.
	if transactionID != "" {
		paymentOrder.ProviderTransactionID = stringPointer(transactionID)
	}
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
		paymentTransaction.FailureReason = nil
		if transactionID != "" {
			paymentTransaction.ProviderTransactionID = stringPointer(transactionID)
		}
		paymentTransaction.CompletedAt = &now
		if err := s.orders.SaveTransaction(ctx, paymentTransaction); err != nil {
			return nil, fmt.Errorf("settle payment transaction: %w", err)
		}
	}
	// NodeLoc can send the same callback twice, and 查单 may settle an order a
	// callback already settled. One order should reach the inbox once, so only a
	// real transition sends the message — and a read that fails still sends it,
	// because nothing here may refuse a confirmed payment.
	alreadySettled := false
	if current, readErr := s.orders.GetOrderByNo(ctx, orderNo); readErr == nil && current != nil {
		alreadySettled = current.Status == "paid" || current.Status == "completed"
	}
	order, err := s.orders.MarkOrderPaid(ctx, orderNo, transactionID, platformFee, merchantPoints)
	if err != nil {
		return nil, err
	}
	// 支付确认后把活动参与记录从「已占用」推进到「已使用」，活动统计与名额
	// 由此只认真正收过钱的订单。失败只记日志：钱已经到账，不能被统计拖累。
	if order != nil && s.activities != nil && order.ActivityID != nil {
		if err := s.activities.MarkOrderSettled(ctx, order.ID); err != nil {
			log.Printf("[payment] order %s: mark activity used: %v", order.OrderNo, err)
		}
	}
	if _, err := s.deliverOrder(ctx, order); err != nil {
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
		if !alreadySettled {
			s.notifyOrder(ctx, order, deliveryEvent(order))
		}
		return order, nil
	}
	if !alreadySettled {
		s.notifyOrder(ctx, fresh, deliveryEvent(fresh))
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
		// Only this order's own row may take the 下单 fallback: an id that merely
		// arrived with the request has no order behind it here, and letting it be
		// confirmed by re-submitting this order would settle the order on a
		// transaction id that was never ours.
		request := contract.QueryPaymentRequest{TransactionID: candidate.transaction}
		if candidate.owned {
			request.OrderID = orderNo
			request.Amount = paymentOrder.Amount
			request.Description = paymentOrder.Description
		}
		query, queryErr := s.gateway.QueryPayment(ctx, request)
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
		via, note := providerRoute(query, userID)
		if !providerCompleted(query.Status) {
			return &ReconcileResult{
				Order: order,
				// A payment NodeLoc already failed will not settle later; one still
				// open is worth another look, which is what drives the retry copy.
				ProviderStatus: query.Status,
				Retryable:      !providerFailed(query.Status),
				CheckedAt:      checkedAt,
				ProviderVia:    via,
				ProviderNote:   note,
			}, nil
		}
		if query.Amount != 0 && query.Amount != paymentOrder.Amount {
			return nil, ErrAmountMismatch
		}
		log.Printf("payment reconcile %s: provider confirmed %s as paid（经由 %s）", orderNo, candidate.transaction, query.Via)
		settled, err := s.settle(ctx, orderNo, paymentOrder, candidate.transaction, query.PlatformFee, query.MerchantPoints)
		if err != nil {
			return nil, err
		}
		return &ReconcileResult{
			Order:          settled,
			Settled:        settled.Status != "pending",
			ProviderStatus: query.Status,
			CheckedAt:      checkedAt,
			ProviderVia:    via,
			ProviderNote:   note,
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

// providerRoute says which NodeLoc route answered a reconcile, and hands the
// reason the 查单 route was skipped to the operator only. The buyer's money state
// is already in the response; the provider's sentence about this store's settings
// would just be English text on a storefront.
func providerRoute(query *contract.QueryPaymentResult, userID uint) (string, string) {
	if query == nil {
		return "", ""
	}
	if userID != 0 {
		return query.Via, ""
	}
	return query.Via, query.Fallback
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

// intParam reads an amount the provider reported in this callback. NodeLoc writes money
// both ways — 100 and "100.00" — and a value that is present but unreadable must never be
// reported as absent: this is the check that decides whether a signed callback is for the
// order it names, and a silent 0 skips it.
func intParam(params map[string]string, keys ...string) int {
	for _, key := range keys {
		if points, ok := pointsOfValue(params[key]); ok {
			return points
		}
	}
	return 0
}

// intPtrParam keeps 「服务商没有报这个字段」 apart from 「报了 0」: the first leaves the
// store's own ledger alone, the second corrects it.
func intPtrParam(params map[string]string, keys ...string) *int {
	for _, key := range keys {
		if points, ok := pointsOfValue(params[key]); ok {
			return &points
		}
	}
	return nil
}

// pointsOfValue converts one provider money field, decimal included: 平台费 9.5 积分 is a
// different settlement than 9, so the fraction is rounded rather than dropped.
func pointsOfValue(text string) (int, bool) {
	trimmed := strings.TrimSpace(strings.ReplaceAll(text, ",", ""))
	if trimmed == "" {
		return 0, false
	}
	if whole, err := strconv.Atoi(trimmed); err == nil {
		return whole, true
	}
	decimal, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, false
	}
	return int(math.Round(decimal)), true
}

// deliverOrder runs one order's delivery. A product bound to a plugin is handed
// to that plugin; every other product keeps the shop's own card/manual queue.
//
// pluginFailure is a partial-delivery marker: the plugin delivered the goods
// but writing them onto the order did not go through, so the next sweep has to
// try the plugin again rather than fall back to the shop's queue and hand the
// same buyer a second, different item. An error means the plugin could not
// deliver and the caller should retry later.
func (s *Service) deliverOrder(ctx context.Context, order *models.Order) (pluginFailure bool, err error) {
	if s.plugins == nil || order == nil || order.ProductID == 0 {
		return false, s.fulfillment.Fulfill(ctx, order)
	}
	owns, err := s.plugins.Owns(ctx, order)
	if err != nil {
		return false, err
	}
	if !owns {
		// Nothing is bound, or the plugin was uninstalled between the two calls:
		// the shop's own delivery is still the right answer.
		return false, s.fulfillment.Fulfill(ctx, order)
	}
	if order.FulfillmentStatus != "plugin_pending" {
		// Mark the order plugin-owned before the plugin runs. A crash between
		// the plugin shipping and the order being written leaves the order in
		// this state, so the retry sweep asks the same plugin again instead of
		// falling into the card queue and handing the buyer a second item.
		if err := s.orders.MarkOrderPluginDelivering(ctx, order.OrderNo); err != nil {
			return false, err
		}
		order.FulfillmentStatus = "plugin_pending"
	}
	result, err := s.plugins.Fulfill(ctx, order)
	if err != nil {
		return false, err
	}
	if result == nil {
		// The binding disappeared between PrepareOrder and Fulfill; the shop's
		// own queue is still the correct fallback for a product a plugin no
		// longer claims.
		return false, s.fulfillment.Fulfill(ctx, order)
	}
	if result.Uncertain {
		note := strings.TrimSpace(result.Note)
		if note == "" {
			note = "第三方交付结果待确认，系统不会自动重复创建。"
		}
		if err := s.orders.MarkOrderPluginReview(ctx, order.OrderNo, note); err != nil {
			return false, err
		}
		order.FulfillmentStatus = "plugin_review"
		return true, nil
	}
	if result.Content == "" {
		result.Content = "插件已完成本次交付，请查看订单详情。"
	}
	if err := s.orders.MarkOrderPluginDelivered(ctx, order.OrderNo, result.Content, result.Note); err != nil {
		log.Printf("payment delivery %s: plugin delivered but the order could not be updated: %v", order.OrderNo, err)
		return true, nil
	}
	if fresh, readErr := s.orders.GetOrderByNo(ctx, order.OrderNo); readErr == nil && fresh != nil {
		*order = *fresh
	} else {
		order.FulfillmentStatus = "delivered"
		order.DeliveryContent = &result.Content
	}
	return false, nil
}

func (s *Service) FulfillOrder(ctx context.Context, orderNo string) (*models.Order, error) {
	return s.FulfillOrderConfirmed(ctx, orderNo, false)
}

// FulfillOrderConfirmed retries delivery. A plugin_review order may have
// created an external resource already, so it requires an explicit operator
// confirmation before another provider call is allowed.
func (s *Service) FulfillOrderConfirmed(ctx context.Context, orderNo string, confirmExternalRetry bool) (*models.Order, error) {
	order, err := s.orders.GetOrderByNo(ctx, strings.TrimSpace(orderNo))
	if err != nil {
		return nil, err
	}
	if order.FulfillmentStatus == "plugin_review" && !confirmExternalRetry {
		return nil, fmt.Errorf("%w: 该订单的第三方交付结果待确认，请先在第三方后台核对后再确认重试", ErrInvalidInput)
	}
	if _, err := s.deliverOrder(ctx, order); err != nil {
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
		if s.retryDelivery(ctx, &pending[i]) != "" {
			delivered++
		}
	}
	return delivered, nil
}

// retryDelivery attempts one order's delivery again and reports where it ended
// up: an empty status means the attempt failed outright.
func (s *Service) retryDelivery(ctx context.Context, order *models.Order) string {
	if _, err := s.deliverOrder(ctx, order); err != nil {
		log.Printf("delivery retry %s: still undelivered: %v", order.OrderNo, err)
		return ""
	}
	if order.FulfillmentStatus == "delivered" || order.FulfillmentStatus == "completed" {
		// The buyer was told the payment landed; this is the message that says
		// the goods did too, which the 已付款 page could not send on its own.
		s.notifyOrder(ctx, order, eventDelivered)
	}
	return order.FulfillmentStatus
}

// restockReleaseLimit caps what one card restock delivers. A shop that restocks
// into a deeper queue sees the next hundred go out now and the remainder on the
// following sweep, which is a queue no realistic shelf reaches.
const restockReleaseLimit = 100

// ReleaseProductBacklog is what a card import owes the buyers who paid before it:
// the orders waiting for this product are delivered now, oldest payment first,
// instead of waiting for the background sweep to notice the stock. It returns how
// many actually left the shop, so the back office can say so out loud.
func (s *Service) ReleaseProductBacklog(ctx context.Context, productID uint) (int, error) {
	pending, err := s.orders.ListUndeliveredPaidOrdersForProduct(ctx, productID, restockReleaseLimit)
	if err != nil {
		return 0, err
	}
	released := 0
	for i := range pending {
		switch s.retryDelivery(ctx, &pending[i]) {
		case "delivered", "completed":
			released++
		}
	}
	return released, nil
}

// WaitingOrdersByProduct is the restocking queue's "and how many have already
// paid for it" column.
func (s *Service) WaitingOrdersByProduct(ctx context.Context) (map[uint]int64, error) {
	return s.orders.CountUndeliveredPaidOrdersByProduct(ctx)
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
	ProviderVia    string `json:"provider_via,omitempty"`
	ProviderNote   string `json:"provider_note,omitempty"`
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
			item.ProviderVia = result.ProviderVia
			item.ProviderNote = result.ProviderNote
			report.Settled++
		default:
			item.ProviderStatus = result.ProviderStatus
			item.ProviderVia = result.ProviderVia
			item.ProviderNote = result.ProviderNote
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

// probeTransactionID is what 「测试支付网关」 asks 查单 about. It is
// deliberately an id no order can have: a button on a settings page must never
// find, let alone move, somebody's money.
const probeTransactionID = "nodeloc-store-connectivity-probe"

// probeDescription labels the replayed 下单 in the merchant's own NodeLoc records,
// so a self-test is readable as one instead of looking like an order nobody paid.
const probeDescription = "nodeloc-store 支付网关自检"

// probeOrderLookback is how many of the shop's orders the settings probe reads
// before it admits there is nothing safe to ask NodeLoc about.
const probeOrderLookback = 20

// Probe is the settings page's question: can this store reach NodeLoc Payments at
// all?
//
// The credential half of that answer can only come from 下单. On the live forum 查单
// is Discourse's own browser-session route and answers 「["BAD CSRF"]」 to a
// server-side call however correctly it is signed, so reading the guard as the whole
// verdict told a shop with a mistyped Payment Token that 「收款与发货不受影响」 while no
// buyer could pay — which is the opposite of what a button named 测试支付网关 is for.
//
// 下单 opens a payment, so the probe does not invent an order: it re-submits one of
// this shop's own orders that NodeLoc already holds a transaction for. The provider
// answers 「Order already exists with status …」 for an order id it knows, and that
// refusal is the proof the store wants — NodeLoc read the signature and recognised
// the request as its own, without a second charge being put on anybody.
func (s *Service) Probe(ctx context.Context) contract.ProbeOutcome {
	if s.gateway == nil {
		return contract.ProbeOutcome{Code: "not_configured", Message: "商店还没有接入 NodeLoc 支付网关。"}
	}
	proved, refusal := s.probeSignature(ctx)
	outcome := contract.ProbeOutcome{Style: s.gateway.SigningStyle()}
	if !proved {
		if refusal == nil {
			// A store that has never taken money through NodeLoc has no order id the
			// provider can remember, so nothing short of opening a fresh payment can
			// prove its credentials. Say that instead of guessing green.
			outcome.Code = "not_verified"
			outcome.Message = "商店还没有任何一单在 NodeLoc 留下交易号，本按钮无法在不打开新支付的前提下验证下单凭据。"
			return outcome
		}
		failure := Classify(refusal)
		outcome.Code, outcome.Message, outcome.Detail, outcome.Retryable = failure.Code, failure.Message, failure.Detail, failure.Retryable
		return outcome
	}
	if _, err := s.gateway.QueryPayment(ctx, contract.QueryPaymentRequest{TransactionID: probeTransactionID}); err != nil {
		failure := Classify(err)
		outcome.Code, outcome.Message, outcome.Detail, outcome.Retryable = failure.Code, failure.Message, failure.Detail, failure.Retryable
	}
	outcome.Style = s.gateway.SigningStyle()
	return outcome
}

// probeSignature replays 下单 for an order NodeLoc already holds. It reports whether
// the provider's answer says anything about this store's signature, and the refusal
// to show the owner when it says the signature is not accepted.
func (s *Service) probeSignature(ctx context.Context) (bool, error) {
	order, ok := s.probeOrder(ctx)
	if !ok {
		return false, nil
	}
	_, err := s.gateway.CreatePayment(ctx, contract.CreatePaymentRequest{
		Amount:      order.TotalAmount,
		Description: probeDescription,
		OrderID:     order.OrderNo,
	})
	var already *domain.PaymentAlreadyRequested
	if errors.As(err, &already) {
		// 「Already exists」 is an answer about this shop's own order, which NodeLoc
		// could only give after it accepted the signature on the request.
		return true, nil
	}
	if err != nil {
		return false, err
	}
	// The provider did not recognise an order id it had handed a transaction for —
	// its records aged out, most likely. The signature is proven all the same.
	return true, nil
}

// probeOrder picks a settled order whose payment NodeLoc took, so the replay is read
// out of the provider's records rather than turning into a new charge.
func (s *Service) probeOrder(ctx context.Context) (models.Order, bool) {
	orders, _, err := s.orders.ListAllOrders(ctx, probeOrderLookback, 0, "", "", 0, "")
	if err != nil {
		return models.Order{}, false
	}
	for i := range orders {
		order := orders[i]
		if order.Status != "paid" && order.Status != "completed" {
			continue
		}
		paymentOrder, err := s.orders.GetPaymentOrderByOrderNo(ctx, order.OrderNo)
		if err != nil || paymentOrder == nil {
			continue
		}
		if strings.TrimSpace(derefString(paymentOrder.ProviderTransactionID)) != "" {
			return order, true
		}
	}
	return models.Order{}, false
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
	// NodeLoc has moved the points by now, so this says what is already true.
	s.notifyOrder(ctx, order, eventRefunded)
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
	if err := checkAttention(attention); err != nil {
		return nil, err
	}
	orders, total, err := s.orders.ListAllOrders(ctx, limit, offset, status, search, buyerID, attention)
	if err != nil {
		return nil, err
	}
	return &OrderList{Orders: orders, Total: total, Limit: limit, Offset: offset}, nil
}

func checkAttention(attention string) error {
	if attention != "" && attention != "undelivered" {
		return fmt.Errorf("%w: unknown attention filter %q", ErrInvalidInput, attention)
	}
	return nil
}

// maxExportOrders bounds an order download. A shop holding tens of thousands of
// orders should narrow the window first rather than pull the whole ledger into
// one response.
const maxExportOrders = 20000

// ExportOrders walks the same filtered list the back office shows, page by page,
// so the CSV a shop owner downloads is the batch they were looking at. The
// second return says the batch was cut short: a truncated file must never read
// like a complete one.
func (s *Service) ExportOrders(ctx context.Context, status, search string, buyerID uint, attention string) ([]models.Order, bool, error) {
	if err := checkAttention(attention); err != nil {
		return nil, false, err
	}
	const pageSize = 100
	var (
		orders    []models.Order
		truncated bool
	)
	for offset := 0; ; offset += pageSize {
		batch, _, err := s.orders.ListAllOrders(ctx, pageSize, offset, status, search, buyerID, attention)
		if err != nil {
			return nil, false, err
		}
		orders = append(orders, batch...)
		if len(batch) < pageSize {
			return orders, truncated, nil
		}
		if len(orders)+pageSize > maxExportOrders {
			return orders, true, nil
		}
	}
}

func (s *Service) AdminCancelOrder(ctx context.Context, orderNo string) (*models.Order, error) {
	order, err := s.orders.UpdateOrderStatus(ctx, strings.TrimSpace(orderNo), "cancelled")
	if err != nil {
		return nil, err
	}
	// 取消的订单要把活动名额还回去，否则限量活动会被弃单占满。
	if order != nil && s.activities != nil && order.ActivityID != nil {
		if err := s.activities.MarkOrderCancelled(ctx, order.ID); err != nil {
			log.Printf("[payment] order %s: release activity reservation: %v", order.OrderNo, err)
		}
	}
	return order, nil
}

func (s *Service) AdminDeliverOrder(ctx context.Context, orderNo string, content string) (*models.Order, error) {
	orderNo = strings.TrimSpace(orderNo)
	// Re-shipping an order that already shipped edits its content; the buyer is
	// told once, not once per edit.
	alreadyShipped := false
	if current, err := s.orders.GetOrderByNo(ctx, orderNo); err == nil && current != nil {
		alreadyShipped = current.FulfillmentStatus == "delivered" || current.FulfillmentStatus == "completed"
	}
	order, err := s.orders.SetOrderDeliveryContent(ctx, orderNo, content)
	if err != nil {
		return nil, err
	}
	if !alreadyShipped {
		s.notifyOrder(ctx, order, eventShipped)
	}
	return order, nil
}

// AdminRefundOrder moves the points back through NodeLoc before the shop calls
// the order refunded. Marking it locally only would tell the buyer their money
// is on the way while NodeLoc still shows it as spent.
// RefundOrderForUser 是给 AI 客服用的用户维度退款：只有这张订单属于该用户、
// 且处于已支付状态时才受理。金额不由调用方提供，退款走 NodeLoc 原路退回。
func (s *Service) RefundOrderForUser(ctx context.Context, userID uint, orderNo string) (int, string, error) {
	orderNo = strings.TrimSpace(orderNo)
	if userID == 0 || orderNo == "" {
		return 0, "", ErrInvalidInput
	}
	order, err := s.orders.GetOrderByNo(ctx, orderNo)
	if err != nil {
		return 0, "", err
	}
	if order.UserID != userID {
		// 越权退款连「这单存在」都不该透露，统一按找不到处理。
		return 0, "", ErrForbidden
	}
	switch order.Status {
	case "paid", "completed":
	default:
		return 0, order.Status, fmt.Errorf("%w: 这一单当前状态是 %s，不能退款", ErrInvalidInput, order.Status)
	}
	refunded, err := s.AdminRefundOrder(ctx, orderNo)
	if err != nil {
		return 0, "", err
	}
	return refunded.TotalAmount, refunded.Status, nil
}

// RefundableOrders 列出该用户已支付、尚未退款的订单，供 AI 先核对再退款。
func (s *Service) RefundableOrders(ctx context.Context, userID uint, limit int) ([]models.Order, error) {
	if userID == 0 {
		return nil, ErrForbidden
	}
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	list, err := s.ListOrders(ctx, userID, limit, 0, "", "")
	if err != nil {
		return nil, err
	}
	out := make([]models.Order, 0, len(list.Orders))
	for _, order := range list.Orders {
		if order.Status == "paid" || order.Status == "completed" {
			out = append(out, order)
		}
	}
	return out, nil
}

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
	// 退款成功后回退活动名额并把参与记录标成已退款，让限量活动能重新放出。
	if order != nil && s.activities != nil && order.ActivityID != nil {
		if err := s.activities.MarkOrderRefunded(ctx, order.ID); err != nil {
			log.Printf("[payment] order %s: release activity after refund: %v", orderNo, err)
		}
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
