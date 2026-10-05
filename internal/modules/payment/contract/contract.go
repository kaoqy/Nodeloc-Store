package contract

import (
	"context"
	"errors"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/domain"
)

// OrderRepo persists store orders and payment-specific records. Implementations
// must make callback updates and fulfillment operations idempotent.
type OrderRepo interface {
	CreatePaymentOrder(ctx context.Context, paymentOrder *domain.PaymentOrder) error
	GetPaymentOrderByOrderNo(ctx context.Context, orderNo string) (*domain.PaymentOrder, error)
	GetPaymentOrderByTransactionID(ctx context.Context, transactionID string) (*domain.PaymentOrder, error)
	SavePaymentOrder(ctx context.Context, paymentOrder *domain.PaymentOrder) error
	CreateTransaction(ctx context.Context, transaction *domain.Transaction) error
	SaveTransaction(ctx context.Context, transaction *domain.Transaction) error
	GetLatestTransaction(ctx context.Context, orderNo, transactionType string) (*domain.Transaction, error)
	// Both lookups are explicit because checkout must never reinterpret one
	// product as another. New storefront clients submit the numeric product ID;
	// slug remains for compatibility with older links and callers.
	GetPurchasableProductByID(ctx context.Context, id uint) (*models.Product, error)
	GetPurchasableProductBySlug(ctx context.Context, slug string) (*models.Product, error)
	CountAvailableCards(ctx context.Context, productID uint) (int64, error)
	CreateOrder(ctx context.Context, order *models.Order) error
	GetOrderByNo(ctx context.Context, orderNo string) (*models.Order, error)
	ListOrdersByUser(ctx context.Context, userID uint, limit, offset int, status, search string) ([]models.Order, int64, error)
	// ListAllOrders filters the admin order list. attention="undelivered" narrows
	// to paid orders still owed a delivery, which is the queue a shop owner works
	// through after the background retry has had its turns.
	ListAllOrders(ctx context.Context, limit, offset int, status, search string, buyerID uint, attention string) ([]models.Order, int64, error)
	MarkOrderPaid(ctx context.Context, orderNo, transactionID string, platformFee, merchantPoints *int) (*models.Order, error)
	MarkOrderRefunded(ctx context.Context, orderNo string) error
	// MarkOrderDeliveryPending notes on a paid order that delivery did not run
	// and the background retry will pick it up, so the wait is explained rather
	// than looking like a lost payment.
	MarkOrderDeliveryPending(ctx context.Context, orderNo, note string) error
	// MarkOrderPluginDelivering marks a paid order as being delivered by a
	// plugin rather than by the shop's own card/manual queue. The order stays in
	// this state until the plugin's delivery is written, so a crash in between is
	// retried through the plugin instead of falling back to cards.
	MarkOrderPluginDelivering(ctx context.Context, orderNo string) error
	// MarkOrderPluginDelivered writes a plugin's finished delivery onto the order.
	MarkOrderPluginDelivered(ctx context.Context, orderNo, content, note string) error
	// MarkOrderPluginReview parks a paid order whose external delivery may have
	// happened but could not be confirmed. It is deliberately outside the
	// automatic retry set: retrying could create a second external resource.
	MarkOrderPluginReview(ctx context.Context, orderNo, note string) error
	// ListUndeliveredPaidOrders returns paid orders whose delivery never
	// completed, including those waiting for card stock to be refilled.
	ListUndeliveredPaidOrders(ctx context.Context, limit int) ([]models.Order, error)
	// ListUndeliveredPaidOrdersForProduct is the same queue for one product: the
	// buyers who paid for keys this shelf did not have. A card restock releases
	// exactly these, oldest payment first.
	ListUndeliveredPaidOrdersForProduct(ctx context.Context, productID uint, limit int) ([]models.Order, error)
	// CountUndeliveredPaidOrdersByProduct is "how many paid orders are still
	// waiting" per product, for the restocking queue.
	CountUndeliveredPaidOrdersByProduct(ctx context.Context) (map[uint]int64, error)
	// ListReconcilableOrders returns orders still marked 待支付 locally that do
	// carry a NodeLoc transaction id, so 批量查单 can ask the provider about them.
	// minAge leaves freshly created orders to their own checkout page, which is
	// already asking.
	ListReconcilableOrders(ctx context.Context, limit int, minAge time.Duration) ([]models.Order, error)
	UpdateOrderStatus(ctx context.Context, orderNo string, status string) (*models.Order, error)
	SetOrderDeliveryContent(ctx context.Context, orderNo string, content string) (*models.Order, error)
	// Transfers are the ledger of 店家主动转账: money the shop sent to a buyer's
	// NodeLoc account with no order behind it. Both outcomes are recorded, so a
	// rejected attempt is still something the owner can point at.
	CreateTransfer(ctx context.Context, transfer *domain.Transfer) error
	SaveTransfer(ctx context.Context, transfer *domain.Transfer) error
	ListTransfers(ctx context.Context, userID uint, limit, offset int) ([]domain.Transfer, int64, error)
}

// UserLookup is a lightweight interface for checking user existence/status.
type UserLookup interface {
	FindByID(ctx context.Context, id uint) (*UserInfo, error)
}

// UserInfo is a minimal user representation for cross-module lookups.
type UserInfo struct {
	ID       uint
	Username string
	IsActive bool
	// NodeLoc identity, needed to send a refund back to the right account.
	OAuthUID      string
	OAuthUsername string
}

// PaymentGateway defines the NodeLoc payment provider operations. The concrete
// HTTP client and signing details belong to infrastructure.
type PaymentGateway interface {
	CreatePayment(ctx context.Context, request CreatePaymentRequest) (*CreatePaymentResult, error)
	// QueryPayment asks what NodeLoc recorded for a payment. The request carries
	// the shop's own order number, amount and description as well as the
	// transaction id, because on the real provider 查单 is a browser-session
	// route and the answer then has to come from re-submitting 下单 instead.
	QueryPayment(ctx context.Context, request QueryPaymentRequest) (*QueryPaymentResult, error)
	Transfer(ctx context.Context, request TransferRequest) (*TransferResult, error)
	VerifyCallback(params map[string]string) bool
	// SigningStyle names the credential convention NodeLoc accepted for this
	// process, empty while none has been tried or nothing has worked. The 设置
	// page reports it so a shop owner can see which key the store is actually
	// signing with instead of guessing from the docs.
	SigningStyle() string
}

type CreatePaymentRequest struct {
	Amount      int
	Description string
	OrderID     string
}

type CreatePaymentResult struct {
	TransactionID string
	PaymentURL    string
	Status        string
	Raw           []byte
}

type QueryPaymentRequest struct {
	TransactionID string
	// OrderID, Amount and Description are only used by the fallback 核实 route
	// (re-submitting 下单 for this order and reading the status NodeLoc reports
	// back). Left empty, the gateway asks 查单 and reports whatever it answers.
	OrderID     string
	Amount      int
	Description string
}

type QueryPaymentResult struct {
	TransactionID  string
	OrderID        string
	Amount         int
	Status         string
	PlatformFee    *int
	MerchantPoints *int
	// Via names the route the answer came from: "query" for NodeLoc's 查单 and
	// "reprocess" for the 下单 oracle. A shop needs to know which one it is
	// relying on, because the second one is only reachable when 查单 is not.
	Via string
	// Fallback is why 查单 was not used, in NodeLoc's own words where it had any.
	// It is back-office material: the money answer is already in Status, and a
	// buyer has no use for the provider's English about this store's settings.
	Fallback string
	Raw      []byte
}

type TransferRequest struct {
	ToUserID   string
	ToUsername string
	Amount     int
	OrderID    string
}

type TransferResult struct {
	TransactionID string
	Status        string
	Raw           []byte
}

// ProbeOutcome is one answer to 「can this store talk to NodeLoc Payments?」, for
// the 设置 page. Code is empty when NodeLoc accepted the probe; otherwise it is
// the same machine-readable code a refused 下单 or 查单 carries, so the shop owner
// and the buyer are told the same thing about the same failure. Style names the
// signing convention the provider actually took.
type ProbeOutcome struct {
	Code      string
	Message   string
	Detail    string
	Retryable bool
	Style     string
}

// FulfillmentService delivers a paid order. Implementations must support
// automatic card delivery, manual-delivery queues, and waiting-for-stock.
type FulfillmentService interface {
	Fulfill(ctx context.Context, order *models.Order) error
}

// PluginDeliverer is the plugin runtime as the money side sees it.
//
// It mirrors the plugin module's own contract without importing it — modules
// only depend on each other's contract packages — and it is two calls because
// the decision and the delivery are separate: Owns decides whether a plugin is
// responsible for this order at all, and Fulfill performs the delivery.
type PluginDeliverer interface {
	// Owns reports whether an enabled plugin is bound to this order's product.
	// It is how payment knows to route the delivery through the plugin rather
	// than the shop's own card/manual queue.
	Owns(ctx context.Context, order *models.Order) (bool, error)
	// Fulfill resolves the order's plugin binding and delivers it. A nil
	// result with a nil error means the binding vanished in between; the caller
	// falls back to the shop's own fulfilment.
	Fulfill(ctx context.Context, order *models.Order) (*PluginDelivery, error)
	// ValidateSelection refuses a purchase whose answers do not resolve to a
	// delivery item, before any money moves.
	ValidateSelection(ctx context.Context, productID uint, formValues map[string]string) error
	// FormFields returns provider-contributed purchase-form fields that must be
	// merged into the product's own form for validation and order storage.
	FormFields(ctx context.Context, productID uint) ([]models.ProductFormField, error)
}

// PluginDelivery is what a plugin handed back for one order.
type PluginDelivery struct {
	Content   string
	Note      string
	Reference string
	// Uncertain means the provider may have created the goods but the answer
	// could not be confirmed. The order stays out of the delivered state so an
	// automatic retry cannot create a duplicate external resource.
	Uncertain bool
}

// ErrPluginUnbound is the sentinel ErrPluginUnbound implementations return from
// PrepareOrder when a product has no enabled plugin binding.
var ErrPluginUnbound = errors.New("no plugin is bound to this product")

// CouponPricing is the catalogue's answer to "what is this code worth on that
// order". Payment asks before any money moves, because the amount handed to
// NodeLoc must already be the discounted one; a code that cannot be priced
// there has to fail the checkout rather than quietly bill the full total.
type CouponPricing interface {
	DiscountFor(ctx context.Context, userID, productID uint, quantity, unitPrice int, code string) (discount int, couponID uint, err error)
}

// ActivityPricing is the activity module as the money side sees it.
//
// It mirrors the activity module's own contract without importing it: modules
// only depend on each other's contract packages. The money side computes the
// price through Price before anything is charged, records the participation with
// Reserve once the order exists, and marks the record settled / refunded so a
// refund gives the activity quota back instead of holding it forever.
type ActivityPricing interface {
	Price(ctx context.Context, input models.ActivityMatchInput) (*models.ActivityPricing, error)
	Reserve(ctx context.Context, orderID uint, record *models.ActivityRecord) error
	MarkOrderSettled(ctx context.Context, orderID uint) error
	MarkOrderRefunded(ctx context.Context, orderID uint) error
	MarkOrderCancelled(ctx context.Context, orderID uint) error
}

// BuyerEvent is one of an order's moments worth telling the buyer about: the
// payment landed, the goods were delivered, the shop refunded. Payment writes the
// copy because it is the module that knows what actually happened; whoever
// carries the message stays generic.
type BuyerEvent struct {
	UserID  uint
	Type    string
	Title   string
	Content string
	Link    string
}

// BuyerNotifier carries BuyerEvents to the buyer's inbox. Publish has no error
// to return by design: the order it describes has already moved on, and a
// message that cannot be written must never make a settled payment look failed.
type BuyerNotifier interface {
	Publish(ctx context.Context, event BuyerEvent)
}
