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
	// ErrProviderUnreachable means NodeLoc was never able to answer: DNS, TLS,
	// timeout, an HTTP 5xx or a body that is not JSON. Nothing about the order is
	// known, so the caller is told to try again.
	ErrProviderUnreachable = errors.New("nodeloc payment service is unreachable")
	// ErrProviderRejected means NodeLoc answered and refused the call itself,
	// typically because the credentials or signature do not match the payment
	// application. Retrying cannot help; the shop owner has to fix the settings.
	ErrProviderRejected = errors.New("nodeloc payment service rejected the request")
	// ErrPaymentAlreadyRequested is the reading of NodeLoc's 「Order already
	// exists with status …」 answer to a second 下单 for an order_id this store
	// already sent. The payment exists on their side, so the storefront must hand
	// the buyer that payment rather than refuse them forever.
	ErrPaymentAlreadyRequested = errors.New("nodeloc already has a payment for this order")
)

// PaymentAlreadyRequested is the typed refusal behind ErrPaymentAlreadyRequested.
// It carries the status word NodeLoc named in the refusal, because 「this order is
// mid-payment」 and 「this order is already paid」 are two different next steps.
type PaymentAlreadyRequested struct {
	Status string
}

func (e *PaymentAlreadyRequested) Error() string {
	if e.Status == "" {
		return ErrPaymentAlreadyRequested.Error()
	}
	return ErrPaymentAlreadyRequested.Error() + ": " + e.Status
}

// Unwrap keeps the refusal inside the provider-rejection family: a store that
// cannot resume the existing payment still reports it as NodeLoc refusing the
// call, which is what the buyer-facing copy and the settings probe already read.
func (e *PaymentAlreadyRequested) Unwrap() error { return ErrProviderRejected }

func (e *PaymentAlreadyRequested) Is(target error) bool {
	return target == ErrPaymentAlreadyRequested
}
