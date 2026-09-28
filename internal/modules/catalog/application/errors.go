package application

import "errors"

// Coupon rejections are named here rather than folded into one generic error:
// the storefront has to tell 这码过期了 apart from 这码你用过, and only the shop
// owner needs to see which rule the code failed.
//
// Each one also carries the machine-readable code and the sentence to show,
// because the quote box and checkout price the same code through the same rules
// from two different modules. Duplicating that table where the HTTP layer sits
// would let the two paths refuse in different words; instead the payment module
// reads CouponCode()/CouponMessage() through its own one-method interface and
// never imports this package.
type CouponError struct {
	// Code is what the storefront switches on, e.g. coupon_expired.
	Code string
	// Message is written for the person typing the code, not for the log.
	Message string
	reason  string
}

func (e *CouponError) Error() string { return e.reason }

func (e *CouponError) CouponCode() string { return e.Code }

func (e *CouponError) CouponMessage() string { return e.Message }

var (
	ErrInvalidQuery = errors.New("invalid query")

	ErrCouponNotFound      = &CouponError{"coupon_not_found", "没有这个优惠码，检查有没有打错。", "coupon not found"}
	ErrCouponInactive      = &CouponError{"coupon_inactive", "这个优惠码已被店家停用。", "coupon is not active"}
	ErrCouponNotStarted    = &CouponError{"coupon_not_started", "这个优惠码还没到生效时间。", "coupon is not valid yet"}
	ErrCouponExpired       = &CouponError{"coupon_expired", "这个优惠码已经过期了。", "coupon has expired"}
	ErrCouponExhausted     = &CouponError{"coupon_exhausted", "这个优惠码的额度已经用完了。", "coupon has reached its usage limit"}
	ErrCouponPerUser       = &CouponError{"coupon_used", "你的账号已经用过这个优惠码。", "coupon already used by this account"}
	ErrCouponMinAmount     = &CouponError{"coupon_min_amount", "订单金额还没达到这个优惠码的使用条件。", "order total is below the coupon minimum"}
	ErrCouponNotApplicable = &CouponError{"coupon_not_applicable", "这个优惠码不能用在当前商品上。", "coupon does not apply to this product"}
	ErrCouponDisabled      = &CouponError{"coupon_disabled", "商店暂未开启优惠码。", "coupons are disabled for this store"}
)
