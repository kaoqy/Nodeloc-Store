package contract

import (
	"context"
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
	GetPurchasableProduct(ctx context.Context, slug string) (*models.Product, error)
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
	// ListUndeliveredPaidOrders returns paid orders whose delivery never
	// completed, including those waiting for card stock to be refilled.
	ListUndeliveredPaidOrders(ctx context.Context, limit int) ([]models.Order, error)
	// ListReconcilableOrders returns orders still marked 待支付 locally that do
	// carry a NodeLoc transaction id, so 批量查单 can ask the provider about them.
	// minAge leaves freshly created orders to their own checkout page, which is
	// already asking.
	ListReconcilableOrders(ctx context.Context, limit int, minAge time.Duration) ([]models.Order, error)
	UpdateOrderStatus(ctx context.Context, orderNo string, status string) (*models.Order, error)
	SetOrderDeliveryContent(ctx context.Context, orderNo string, content string) (*models.Order, error)
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
	QueryPayment(ctx context.Context, transactionID string) (*QueryPaymentResult, error)
	Transfer(ctx context.Context, request TransferRequest) (*TransferResult, error)
	VerifyCallback(params map[string]string) bool
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

type QueryPaymentResult struct {
	TransactionID  string
	OrderID        string
	Amount         int
	Status         string
	PlatformFee    *int
	MerchantPoints *int
	Raw            []byte
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

// FulfillmentService delivers a paid order. Implementations must support
// automatic card delivery, manual-delivery queues, and waiting-for-stock.
type FulfillmentService interface {
	Fulfill(ctx context.Context, order *models.Order) error
}
