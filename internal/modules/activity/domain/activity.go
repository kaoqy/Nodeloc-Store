package domain

import (
	"errors"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
)

// Activity 模块复用 canonical persistence model，调用方不直接碰 models。
type (
	Activity       = models.Activity
	ActivityRule   = models.ActivityRule
	ActivityRecord = models.ActivityRecord
	ActivityLog    = models.ActivityLog
	CouponRecord   = models.CouponRecord
	// MatchInput / Pricing 是结算入参与结果，同样复用 models 里的定义。
	MatchInput = models.ActivityMatchInput
	Pricing    = models.ActivityPricing
	Coupon     = models.Coupon
	Product    = models.Product
)

var (
	ErrActivityNotFound = errors.New("activity not found")
	ErrInvalidInput     = errors.New("invalid activity input")
	ErrNotRunning       = errors.New("activity is not running")
	ErrStockExhausted   = errors.New("activity stock exhausted")
	ErrQuotaExhausted   = errors.New("activity quota exhausted")
	ErrPerUserLimit     = errors.New("activity per-user limit reached")
	ErrNotApplicable    = errors.New("activity does not apply to this order")
	// ErrNoActivity 表示没有命中任何活动，是正常情况而不是故障。
	ErrNoActivity = errors.New("no activity matched")
)

// 规则类型常量，与后台规则表单一一对应。
const (
	RuleFactorOff    = "factor_off"
	RulePercentOff   = "percent_off"
	RuleAmountOff    = "amount_off"
	RuleFixedPrice   = "fixed_price"
	RuleFullReduce   = "full_reduce"
	RuleFullQuantity = "full_quantity"
	RuleBulkPrice    = "bulk_price"
	RuleCouponLock   = "coupon_lock"
	RuleGiftCoupon   = "gift_coupon"
)

// RuleCategories 把规则分组，后台按活动类型只展示可用规则。
var RuleTypes = []string{
	RuleFactorOff, RulePercentOff, RuleAmountOff, RuleFixedPrice, RuleFullReduce,
	RuleFullQuantity, RuleBulkPrice, RuleCouponLock, RuleGiftCoupon,
}

func ValidRuleType(value string) bool {
	for _, candidate := range RuleTypes {
		if candidate == value {
			return true
		}
	}
	return false
}

// RuleConfig 是规则的结构化参数。字段按规则类型取用，未用到的保持零值。
// 全部为整数，金额单位与商品定价一致，避免浮点误差。
type RuleConfig struct {
	// Factor 是乘区折扣系数，0.8000 表示售价 × 0.8。
	//
	// 这是首选的折扣表达方式：它直接乘在售价上，口径与「实际成交价 =
	// 售价 × 折扣率」完全一致，不会出现「减 X 元」在不同数量下被重复放大
	// 的问题。按四位小数存储，避免 0.85 这类系数在浮点里失精。
	Factor float64 `json:"factor,omitempty"`
	// PercentOff: 折扣百分比，90 表示 9 折。保留用于兼容既有活动。
	Percent int `json:"percent,omitempty"`
	// AmountOff / FullReduce: 直接减多少钱。
	Amount int `json:"amount,omitempty"`
	// FixedPrice / BulkPrice: 折后单价。
	Price int `json:"price,omitempty"`
	// FullReduce / FullQuantity: 门槛（金额或件数）。
	Threshold int `json:"threshold,omitempty"`
	// FullQuantity: 满件后每件减多少；BulkPrice: 达到件数后的单价。
	Quantity int `json:"quantity,omitempty"`
	PerUnit  int `json:"per_unit,omitempty"`
	// 阶梯档位，用于满减/满件/批量价的多档配置。
	Tiers []RuleTier `json:"tiers,omitempty"`
	// CouponID 用于优惠券领取类活动与赠送券规则。
	CouponID uint `json:"coupon_id,omitempty"`
	// CouponPrefix 是系统自动生成优惠码时的前缀。
	CouponPrefix string `json:"coupon_prefix,omitempty"`
}

// RuleTier 是一档阶梯：达到 Threshold 后按 Amount 减（或按 Price 计价）。
type RuleTier struct {
	Threshold int `json:"threshold"`
	Amount    int `json:"amount,omitempty"`
	Percent   int `json:"percent,omitempty"`
	Price     int `json:"price,omitempty"`
	Quantity  int `json:"quantity,omitempty"`
}

// ListFilter 是后台活动列表的筛选条件。
type ListFilter struct {
	Status  string
	Type    string
	Search  string
	Keyword string
	Limit   int
	Offset  int
	Sort    string
}

// ActivityView 是后台列表行，带参与统计。
type ActivityView struct {
	Activity
	OrderCount     int64   `json:"order_count"`
	UserCount      int64   `json:"user_count"`
	DiscountTotal  int64   `json:"discount_total"`
	RevenueTotal   int64   `json:"revenue_total"`
	ConversionRate float64 `json:"conversion_rate"`
}

// ActivityStats 是单个活动的数据面板。
type ActivityStats struct {
	ActivityID     uint    `json:"activity_id"`
	Participants   int64   `json:"participants"`
	Orders         int64   `json:"orders"`
	PaidOrders     int64   `json:"paid_orders"`
	DiscountTotal  int64   `json:"discount_total"`
	RevenueTotal   int64   `json:"revenue_total"`
	StockUsed      int     `json:"stock_used"`
	StockLimit     int     `json:"stock_limit"`
	QuotaUsed      int     `json:"quota_used"`
	QuotaLimit     int     `json:"quota_limit"`
	ConversionRate float64 `json:"conversion_rate"`
	RefundCount    int64   `json:"refund_count"`
}
