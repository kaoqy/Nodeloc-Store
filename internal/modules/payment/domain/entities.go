package domain

import "time"

// Transaction records a payment-provider transaction independently from the
// store order so callbacks, queries, transfers, and refunds remain auditable.
type Transaction struct {
	ID                    uint       `gorm:"primaryKey" json:"id"`
	OrderID               uint       `gorm:"not null;index" json:"order_id"`
	OrderNo               string     `gorm:"size:64;not null;index" json:"order_no"`
	Provider              string     `gorm:"size:32;not null;default:nodeloc" json:"provider"`
	ProviderTransactionID string     `gorm:"size:128;uniqueIndex" json:"provider_transaction_id"`
	Type                  string     `gorm:"size:24;not null;index" json:"type"`
	Status                string     `gorm:"size:24;not null;index" json:"status"`
	Amount                int        `gorm:"not null" json:"amount"`
	Currency              string     `gorm:"size:12;not null;default:points" json:"currency"`
	RequestPayload        *string    `gorm:"type:text" json:"-"`
	ResponsePayload       *string    `gorm:"type:text" json:"-"`
	FailureReason         *string    `gorm:"type:text" json:"failure_reason,omitempty"`
	CompletedAt           *time.Time `json:"completed_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// PaymentOrder is the payment module's persistence projection for a store
// order. The canonical merchandise and fulfillment fields remain in
// models.Order; this entity stores provider-specific payment state.
type PaymentOrder struct {
	ID                    uint       `gorm:"primaryKey" json:"id"`
	OrderID               uint       `gorm:"not null;uniqueIndex" json:"order_id"`
	OrderNo               string     `gorm:"size:64;not null;uniqueIndex" json:"order_no"`
	UserID                uint       `gorm:"not null;index" json:"user_id"`
	PaymentID             string     `gorm:"size:64;not null;index" json:"payment_id"`
	Provider              string     `gorm:"size:32;not null;default:nodeloc" json:"provider"`
	Amount                int        `gorm:"not null" json:"amount"`
	Description           string     `gorm:"size:255;not null" json:"description"`
	Status                string     `gorm:"size:24;not null;index" json:"status"`
	ProviderTransactionID *string    `gorm:"size:128;index" json:"provider_transaction_id,omitempty"`
	PaymentURL            *string    `gorm:"type:text" json:"payment_url,omitempty"`
	CallbackPayload       *string    `gorm:"type:text" json:"-"`
	PaidAt                *time.Time `json:"paid_at,omitempty"`
	RefundedAt            *time.Time `json:"refunded_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

const (
	TransactionTypePayment  = "payment"
	TransactionTypeQuery    = "query"
	TransactionTypeTransfer = "transfer"
	TransactionTypeRefund   = "refund"

	StatusPending   = "pending"
	StatusProcessing = "processing"
	StatusPaid      = "paid"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusRefunded  = "refunded"
	StatusCancelled = "cancelled"
)
