package contract

import (
	"context"

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
	GetOrderByNo(ctx context.Context, orderNo string) (*models.Order, error)
	ListOrdersByUser(ctx context.Context, userID uint, limit, offset int) ([]models.Order, int64, error)
	MarkOrderPaid(ctx context.Context, orderNo, transactionID string, platformFee, merchantPoints *int) (*models.Order, error)
	MarkOrderRefunded(ctx context.Context, orderNo string) error
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
