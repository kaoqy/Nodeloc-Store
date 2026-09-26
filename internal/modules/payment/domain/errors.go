package domain

import "errors"

// Sentinels shared by the payment module's layers. Infrastructure returns them
// and application/transport classify them without importing concrete stores.
var (
	ErrOrderNotFound         = errors.New("order not found")
	ErrPaymentOrderNotFound  = errors.New("payment order not found")
	ErrInsufficientStock     = errors.New("insufficient card stock")
	ErrProductNotPurchasable = errors.New("product is not purchasable")
	ErrNotPayable            = errors.New("order is not paid")
	// ErrPaymentNotConfigured is returned when the Payment ID / Secret Key pair
	// is missing, so the buyer gets a clear message instead of a generic 500.
	ErrPaymentNotConfigured = errors.New("payment is not configured on this store")
)
