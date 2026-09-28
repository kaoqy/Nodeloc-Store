package domain

import "errors"

// The catalogue rules a shop owner can trip, named so a broken one can be
// reported without quoting the database. English here is log material; the
// sentence a person reads is attached at the HTTP boundary, which is the only
// layer that knows the back office is written in Chinese.
var (
	// ErrInvalidInput wraps the specific field a submission got wrong, so the
	// answer can name it: "invalid input: 商品名称要填。"
	ErrInvalidInput = errors.New("invalid input")

	// These three are the unique indexes the catalogue holds. A collision used to
	// reach the shop owner as "constraint failed: UNIQUE constraint failed:
	// products.slug (2067)" — the driver's own words, for a mistake as ordinary as
	// reusing a slug.
	ErrProductSlugTaken  = errors.New("product slug already taken")
	ErrCategorySlugTaken = errors.New("category slug already taken")
	ErrCouponCodeTaken   = errors.New("coupon code already taken")
)
