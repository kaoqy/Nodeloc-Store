package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ── 活动营销 ──────────────────────────────────────────────────────────
//
// 活动是商店对外的一次促销承诺：规则、适用范围、时间窗口、名额和展示位。
// 规则本身存成结构化 JSON（Activity.Rules / ActivityRule.Config），后台按
// 类型渲染表单，结算时由服务端重新计算，前端传回来的价格一律不采信。

// Activity 是一次促销活动。规则字段保持结构化，避免把折扣逻辑写死在代码里。
type Activity struct {
	Base
	Name        string `gorm:"size:160;not null" json:"name"`
	Subtitle    string `gorm:"size:200" json:"subtitle,omitempty"`
	Description string `gorm:"type:text" json:"description,omitempty"`
	CoverImage  string `gorm:"size:500" json:"cover_image,omitempty"`
	BannerImage string `gorm:"size:500" json:"banner_image,omitempty"`

	// Type 是活动类型，取值见 ActivityTypes；Rules 是该类型的结构化参数。
	Type  string `gorm:"size:48;not null;index" json:"type"`
	Rules string `gorm:"type:text" json:"rules,omitempty"`

	// 适用范围。空数组表示全场；同时给出商品和分类时按并集匹配。
	ProductIDs  string `gorm:"type:text" json:"product_ids,omitempty"`
	CategoryIDs string `gorm:"type:text" json:"category_ids,omitempty"`

	// 人群限制：all / new_user / first_purchase / member / role。
	UserScope     string `gorm:"size:32;default:'all';not null" json:"user_scope"`
	UserRoleScope string `gorm:"size:64" json:"user_role_scope,omitempty"`

	StartAt *time.Time `gorm:"index" json:"start_at,omitempty"`
	EndAt   *time.Time `gorm:"index" json:"end_at,omitempty"`

	SortOrder int `gorm:"default:0;not null;index" json:"sort_order"`

	// 活动库存与名额：活动商品的可售总量、参与人数上限。
	StockLimit int `gorm:"default:0;not null" json:"stock_limit"`
	StockUsed  int `gorm:"default:0;not null" json:"stock_used"`
	QuotaLimit int `gorm:"default:0;not null" json:"quota_limit"`
	QuotaUsed  int `gorm:"default:0;not null" json:"quota_used"`

	PerUserLimit int `gorm:"default:0;not null" json:"per_user_limit"`

	RequireLogin  bool `gorm:"default:true;not null" json:"require_login"`
	AllowStacking bool `gorm:"default:false;not null" json:"allow_stacking"`
	AutoApply     bool `gorm:"default:true;not null" json:"auto_apply"`

	// Status 见 ActivityStatuses。时间到点由后台任务和读取时共同推进。
	Status string `gorm:"size:24;default:'draft';not null;index" json:"status"`

	CreatedBy       *uint  `gorm:"index" json:"created_by,omitempty"`
	Remark          string `gorm:"size:500" json:"remark,omitempty"`
	TerminateReason string `gorm:"size:500" json:"terminate_reason,omitempty"`

	// 展示与通知设置。
	ShowOnHome  bool   `gorm:"default:true;not null" json:"show_on_home"`
	ShowInList  bool   `gorm:"default:true;not null" json:"show_in_list"`
	NotifyUsers bool   `gorm:"default:false;not null" json:"notify_users"`
	NotifyText  string `gorm:"type:text" json:"notify_text,omitempty"`
	Advanced    string `gorm:"type:text" json:"advanced,omitempty"`

	RulesList []ActivityRule `gorm:"foreignKey:ActivityID;constraint:OnDelete:CASCADE;" json:"rule_list,omitempty"`
}

// 活动类型常量。中文名见 ActivityTypeLabels。
const (
	ActivityTypeLimitedDiscount  = "limited_discount"
	ActivityTypeStoreDiscount    = "store_discount"
	ActivityTypeProductDiscount  = "product_discount"
	ActivityTypeCategoryDiscount = "category_discount"
	ActivityTypeFullReduction    = "full_reduction"
	ActivityTypeFullQuantity     = "full_quantity"
	ActivityTypeBulkDiscount     = "bulk_discount"
	ActivityTypeCouponActivity   = "coupon_activity"
	ActivityTypeNewUser          = "new_user"
	ActivityTypeFirstPurchase    = "first_purchase"
	ActivityTypeMemberOnly       = "member_only"
	ActivityTypeLimited          = "limited_activity"
	ActivityTypeSeckill          = "seckill"
	ActivityTypeCouponClaim      = "coupon_claim"
)

// 活动状态常量。
const (
	ActivityStatusDraft     = "draft"
	ActivityStatusScheduled = "scheduled"
	ActivityStatusRunning   = "running"
	ActivityStatusPaused    = "paused"
	ActivityStatusEnded     = "ended"
	ActivityStatusOffline   = "offline"
	ActivityStatusDeleted   = "deleted"
)

var ActivityTypes = []string{
	ActivityTypeLimitedDiscount, ActivityTypeStoreDiscount, ActivityTypeProductDiscount,
	ActivityTypeCategoryDiscount, ActivityTypeFullReduction, ActivityTypeFullQuantity,
	ActivityTypeBulkDiscount, ActivityTypeCouponActivity, ActivityTypeNewUser,
	ActivityTypeFirstPurchase, ActivityTypeMemberOnly, ActivityTypeLimited,
	ActivityTypeSeckill, ActivityTypeCouponClaim,
}

var ActivityTypeLabels = map[string]string{
	ActivityTypeLimitedDiscount:  "限时折扣",
	ActivityTypeStoreDiscount:    "全场折扣",
	ActivityTypeProductDiscount:  "指定商品折扣",
	ActivityTypeCategoryDiscount: "指定分类折扣",
	ActivityTypeFullReduction:    "满减",
	ActivityTypeFullQuantity:     "满件优惠",
	ActivityTypeBulkDiscount:     "批量购买优惠",
	ActivityTypeCouponActivity:   "优惠码活动",
	ActivityTypeNewUser:          "新人优惠",
	ActivityTypeFirstPurchase:    "首次购买优惠",
	ActivityTypeMemberOnly:       "会员专属活动",
	ActivityTypeLimited:          "限量活动",
	ActivityTypeSeckill:          "商品秒杀",
	ActivityTypeCouponClaim:      "优惠券领取活动",
}

var ActivityStatuses = []string{
	ActivityStatusDraft, ActivityStatusScheduled, ActivityStatusRunning,
	ActivityStatusPaused, ActivityStatusEnded, ActivityStatusOffline, ActivityStatusDeleted,
}

var ActivityStatusLabels = map[string]string{
	ActivityStatusDraft:     "草稿",
	ActivityStatusScheduled: "未开始",
	ActivityStatusRunning:   "进行中",
	ActivityStatusPaused:    "已暂停",
	ActivityStatusEnded:     "已结束",
	ActivityStatusOffline:   "已下架",
	ActivityStatusDeleted:   "已删除",
}

func ValidActivityType(value string) bool {
	for _, candidate := range ActivityTypes {
		if candidate == value {
			return true
		}
	}
	return false
}

func ValidActivityStatus(value string) bool {
	for _, candidate := range ActivityStatuses {
		if candidate == value {
			return true
		}
	}
	return false
}

// ActivityRule 是结构化规则的一行。同一次活动可以挂多条规则，例如
// 「满 100 减 20」+「指定分类再打 9 折」，按 SortOrder 依次生效。
type ActivityRule struct {
	Base
	ActivityID uint   `gorm:"not null;index;uniqueIndex:uniq_activity_rule" json:"activity_id"`
	RuleType   string `gorm:"size:48;not null;uniqueIndex:uniq_activity_rule" json:"rule_type"`
	// Config 是该规则的结构化参数（阈值、折扣、件数、档位等）。
	Config    string `gorm:"type:text" json:"config,omitempty"`
	IsEnabled bool   `gorm:"default:true;not null" json:"is_enabled"`
	SortOrder int    `gorm:"default:0;not null" json:"sort_order"`
}

// ActivityRecord 是一次参与记录：谁、在哪一单、拿了多少优惠。
// Snapshot 保存下单当时完整的活动规则，历史订单因此不受后续改价影响。
type ActivityRecord struct {
	Base
	ActivityID     uint  `gorm:"not null;index:idx_activity_record;index" json:"activity_id"`
	UserID         uint  `gorm:"not null;index:idx_activity_record;index" json:"user_id"`
	OrderID        *uint `gorm:"index" json:"order_id,omitempty"`
	ProductID      *uint `gorm:"index" json:"product_id,omitempty"`
	Quantity       int   `gorm:"default:1;not null" json:"quantity"`
	OriginalAmount int   `gorm:"default:0;not null" json:"original_amount"`
	DiscountAmount int   `gorm:"default:0;not null" json:"discount_amount"`
	PayableAmount  int   `gorm:"default:0;not null" json:"payable_amount"`
	// Status: reserved 已占用、used 已支付、refunded 已退款、cancelled 已取消。
	Status     string `gorm:"size:24;default:'reserved';not null;index" json:"status"`
	CouponCode string `gorm:"size:64" json:"coupon_code,omitempty"`
	Snapshot   string `gorm:"type:text" json:"snapshot,omitempty"`
	IP         string `gorm:"size:64" json:"ip,omitempty"`
}

// ActivityLog 是活动的操作与风控日志：上下架、暂停、规则修改和异常拒绝。
type ActivityLog struct {
	Base
	ActivityID uint   `gorm:"not null;index" json:"activity_id"`
	ActorID    *uint  `gorm:"index" json:"actor_id,omitempty"`
	Action     string `gorm:"size:64;not null;index" json:"action"`
	Detail     string `gorm:"type:text" json:"detail,omitempty"`
	Before     string `gorm:"type:text" json:"before,omitempty"`
	After      string `gorm:"type:text" json:"after,omitempty"`
	IP         string `gorm:"size:64" json:"ip,omitempty"`
	UserAgent  string `gorm:"size:255" json:"user_agent,omitempty"`
	Result     string `gorm:"size:24;default:'ok';not null" json:"result"`
	Error      string `gorm:"type:text" json:"error,omitempty"`
}

// CouponRecord 是优惠码的领取与使用记录。优惠码既可以由买家在活动页领取，
// 也可以由后台直接发放；每一条都独立记账，便于统计与防重复领取。
type CouponRecord struct {
	Base
	CouponID  uint       `gorm:"not null;index:idx_coupon_record" json:"coupon_id"`
	UserID    uint       `gorm:"not null;index:idx_coupon_record;index" json:"user_id"`
	OrderID   *uint      `gorm:"index" json:"order_id,omitempty"`
	Status    string     `gorm:"size:24;default:'claimed';not null;index" json:"status"`
	Source    string     `gorm:"size:32;default:'claim';not null" json:"source"`
	ClaimedAt time.Time  `json:"claimed_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	ExpireAt  *time.Time `gorm:"index" json:"expire_at,omitempty"`
}

// ActivityMatchInput 是计算一次活动优惠需要的输入。价格只从服务端读到的
// 商品数据来，绝不使用请求里的金额。
type ActivityMatchInput struct {
	UserID       uint
	ProductID    uint
	CategoryID   *uint
	Quantity     int
	UnitPrice    int
	IsFirstOrder bool
	IsNewUser    bool
	Role         string
	Now          time.Time
}

// ActivityPricing 是活动折扣的计算结果。
type ActivityPricing struct {
	ActivityID     uint
	ActivityName   string
	DiscountAmount int
	Payable        int
	Snapshot       string
	// AppliedRules 说明是哪些规则生效，买家侧只展示名称，后台可看明细。
	AppliedRules []string
}

// ParseIDList 解析商品/分类的 JSON 数组，空值返回空集合。
func ParseIDList(value string) []uint {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	var out []uint
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return nil
	}
	return out
}

// EncodeIDList 把 id 列表存成 JSON，nil 存成空数组而不是 null。
func EncodeIDList(ids []uint) (string, error) {
	if ids == nil {
		ids = []uint{}
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return "", fmt.Errorf("encode id list: %w", err)
	}
	return string(encoded), nil
}
