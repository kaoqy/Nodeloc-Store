package models

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Base model with common columns
type Base struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// ── User & Identity ─────────────────────────────────────────────────

type User struct {
	Base
	Username        string     `gorm:"size:64;uniqueIndex;not null" json:"username"`
	Email           *string    `gorm:"size:190;uniqueIndex" json:"email,omitempty"`
	PasswordHash    *string    `gorm:"size:255" json:"-"`
	IsAdmin         bool       `gorm:"default:false;not null" json:"is_admin"`
	IsActive        bool       `gorm:"column:is_active;default:true;not null" json:"is_active"`
	Role            string     `gorm:"size:32;default:'user';not null;index" json:"role"`
	Points          int        `gorm:"default:0;not null" json:"points"`
	ConsecutiveDays int        `gorm:"default:0;not null" json:"consecutive_days"`
	TotalCheckins   int        `gorm:"default:0;not null" json:"total_checkins"`
	LastCheckinDate *time.Time `json:"last_checkin_date,omitempty"`
	LastLoginAt     *time.Time `json:"last_login_at,omitempty"`
	Nickname        string     `gorm:"size:64" json:"nickname"`
	AvatarURL       string     `gorm:"size:255" json:"avatar_url"`
	Bio             string     `gorm:"type:text" json:"bio"`
	LastLoginIP     string     `gorm:"size:45" json:"-"`

	// OAuth binding (denormalized for quick lookups)
	OAuthProvider   *string `gorm:"size:32;index" json:"oauth_provider,omitempty"`
	OAuthUID        *string `gorm:"size:190;index" json:"oauth_uid,omitempty"`
	OAuthUsername   *string `gorm:"size:64" json:"oauth_username,omitempty"`
	OAuthName       *string `gorm:"size:64" json:"oauth_name,omitempty"`
	OAuthAvatar     *string `gorm:"size:255" json:"oauth_avatar,omitempty"`
	OAuthTrustLevel *int    `json:"oauth_trust_level,omitempty"`
	OAuthScope      *string `gorm:"size:255" json:"oauth_scope,omitempty"`
	OAuthHasEmail   bool    `gorm:"default:false" json:"oauth_has_email"`

	// Relations
	OAuthIdentities []OAuthIdentity `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE;" json:"-"`
	Orders          []Order         `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE;" json:"-"`
	Checkins        []CheckIn       `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE;" json:"-"`
	PointEntries    []PointLedger   `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE;" json:"-"`
	Notifications   []Notification  `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE;" json:"-"`
}

type OAuthIdentity struct {
	Base
	UserID       uint    `gorm:"uniqueIndex:uniq_oauth_identities_user;not null" json:"user_id"`
	Provider     string  `gorm:"size:32;uniqueIndex:uniq_oauth_identities_provider_uid;not null" json:"provider"`
	ProviderUID  string  `gorm:"size:190;uniqueIndex:uniq_oauth_identities_provider_uid;not null" json:"provider_uid"`
	Username     *string `gorm:"size:64" json:"username,omitempty"`
	DisplayName  *string `gorm:"size:64" json:"display_name,omitempty"`
	AvatarURL    *string `gorm:"size:255" json:"avatar_url,omitempty"`
	Scope        *string `gorm:"size:255" json:"scope,omitempty"`
	AccessToken  *string `gorm:"type:text" json:"-"`
	RefreshToken *string `gorm:"type:text" json:"-"`

	User User `gorm:"foreignKey:UserID;" json:"-"`
}

// TableName pins the table name; GORM would otherwise derive
// "o_auth_identities" from the OAuth acronym and miss the table the identity
// module queries.
func (OAuthIdentity) TableName() string { return "oauth_identities" }

// legacyOAuthTable is what GORM named this table before TableName pinned it.
const legacyOAuthTable = "o_auth_identities"

// OAuthAttempt is one NodeLoc 登录 round trip as the shop experienced it: the
// step it reached, whether it got there, and the provider's own words when it
// did not. A failed login leaves nothing in the buyer's browser but a generic
// 「链接过期」, and the real reason — a redirect_uri that does not match the
// application, a rejected scope, a state cookie the proxy dropped — only ever
// appeared in the container's log. This table puts it on the 设置 page the shop
// owner is already looking at.
//
// It never holds a token, a secret or an authorization code: Detail is the
// provider's error text, trimmed and scrubbed of anything that looks like a
// credential before it is written.
type OAuthTransaction struct {
	Base
	StateHash  string     `gorm:"size:64;uniqueIndex;not null" json:"-"`
	Intent     string     `gorm:"size:16;not null" json:"intent"`
	UserID     *uint      `gorm:"index" json:"user_id,omitempty"`
	ReturnURL  string     `gorm:"size:2048;not null;default:'/'" json:"return_url"`
	Status     string     `gorm:"size:16;not null;default:'pending';index" json:"status"`
	ExpiresAt  time.Time  `gorm:"index;not null" json:"expires_at"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
}

func (OAuthTransaction) TableName() string { return "oauth_transactions" }

type OAuthAttempt struct {
	Base
	// Step is initiate (the browser was sent to NodeLoc) or callback (NodeLoc
	// sent something back).
	Step string `gorm:"size:16;index" json:"step"`
	// Outcome is started, success or failed.
	Outcome string `gorm:"size:16" json:"outcome"`
	// Reason is the machine code the storefront reads: disabled, not_configured,
	// rejected, unreachable, denied, expired, state…
	Reason string `gorm:"size:32" json:"reason,omitempty"`
	Detail string `gorm:"type:text" json:"detail,omitempty"`
	// RedirectURI records what the shop actually told NodeLoc to send the buyer
	// back to, because a mismatch there is the most common broken login and the
	// one thing the forum never explains.
	RedirectURI string `gorm:"type:text" json:"redirect_uri,omitempty"`
	// Username is the NodeLoc account a successful login resolved to.
	Username string `gorm:"size:64" json:"username,omitempty"`
	Binding  bool   `gorm:"not null;default:false" json:"binding"`
}

func (OAuthAttempt) TableName() string { return "oauth_attempts" }

// ── Points & Checkin ─────────────────────────────────────────────────

type PointLedger struct {
	Base
	UserID        int    `gorm:"not null;index" json:"user_id"`
	Delta         int    `gorm:"not null" json:"delta"`
	BalanceAfter  int    `gorm:"not null" json:"balance_after"`
	Reason        string `gorm:"size:120;not null" json:"reason"`
	ReferenceType string `gorm:"size:32;not null;index:idx_reference,unique" json:"reference_type"`
	ReferenceID   string `gorm:"size:128;not null;index:idx_reference,unique" json:"reference_id"`
	ActorID       *int   `json:"actor_id,omitempty"`

	User User `gorm:"foreignKey:UserID;" json:"-"`
}

type CheckIn struct {
	Base
	UserID          uint      `gorm:"not null;index" json:"user_id"`
	CheckinDate     time.Time `gorm:"type:date;not null;index" json:"checkin_date"`
	RewardPoints    int       `gorm:"default:0;not null" json:"reward_points"`
	ConsecutiveDays int       `gorm:"default:1;not null" json:"consecutive_days"`

	User User `gorm:"foreignKey:UserID;" json:"-"`

	// Unique constraint on user_id + checkin_date
	_ struct{} `gorm:"uniqueIndex:idx_user_date,expression:user_id,checkin_date"`
}

// ── Catalog ──────────────────────────────────────────────────────────

type Category struct {
	Base
	Slug        string  `gorm:"size:120;uniqueIndex;not null" json:"slug"`
	Name        string  `gorm:"size:120;not null" json:"name"`
	Description *string `gorm:"type:text" json:"description,omitempty"`
	Icon        *string `gorm:"size:50" json:"icon,omitempty"`
	SortOrder   int     `gorm:"default:0;not null" json:"sort_order"`
	// IsVisible has no column default, for the same reason as the product flags
	// below: GORM leaves out a zero value when the column declares a default, so
	// a hidden category would be written back as visible.
	IsVisible bool `gorm:"not null" json:"is_visible"`

	Products []Product `gorm:"foreignKey:CategoryID;" json:"-"`
}

type ProductFormField struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Placeholder string   `json:"placeholder,omitempty"`
	Options     []string `json:"options,omitempty"`
	MaxLength   int      `json:"max_length,omitempty"`
}

// ValidateProductForm checks a set of purchase-form answers against a product's
// schema and returns the answers as the JSON an order stores.
//
// requireValues says whether the answers are expected to be complete. A buyer's
// checkout passes true: a mandatory field they left blank has to stop the order.
// A shop saving the product's own schema passes false, because the blanks it
// would otherwise complain about are the buyer's answers, which do not exist yet
// — validating a schema that way made any product with a required field
// impossible to save at all.
func ValidateProductForm(fields []ProductFormField, values map[string]string) (string, error) {
	return ValidateProductFormValues(fields, values, true)
}

// ValidateProductFormValues is ValidateProductForm with the completeness rule
// made explicit; see that function for what the flag means.
func ValidateProductFormValues(fields []ProductFormField, values map[string]string, requireValues bool) (string, error) {
	if len(fields) == 0 {
		if len(values) > 0 {
			return "", fmt.Errorf("商品未配置购买表单")
		}
		return "", nil
	}
	if len(fields) > 20 {
		return "", fmt.Errorf("商品购买表单配置无效")
	}
	if values == nil {
		values = map[string]string{}
	}
	allowed := make(map[string]ProductFormField, len(fields))
	for _, field := range fields {
		field.Key = strings.TrimSpace(field.Key)
		field.Label = strings.TrimSpace(field.Label)
		field.Type = strings.TrimSpace(field.Type)
		// Number fields are contributed by delivery plugins (for example the
		// New-API top-up amount). They share the same schema, so the validator
		// has to know the type or every order for such a product is refused with
		// 「购买表单字段配置无效」.
		if field.Key == "" || field.Label == "" ||
			(field.Type != "text" && field.Type != "select" && field.Type != "number") ||
			allowed[field.Key].Key != "" {
			return "", fmt.Errorf("商品购买表单字段配置无效")
		}
		if field.MaxLength <= 0 || field.MaxLength > 1000 {
			field.MaxLength = 255
		}
		if field.Type == "select" && len(field.Options) == 0 {
			return "", fmt.Errorf("选择字段必须配置选项")
		}
		allowed[field.Key] = field
	}
	for key := range values {
		if _, ok := allowed[key]; !ok {
			return "", fmt.Errorf("购买表单包含未定义字段")
		}
	}
	for key, field := range allowed {
		value := strings.TrimSpace(values[key])
		if field.Required && value == "" && requireValues {
			return "", fmt.Errorf("请填写%s", field.Label)
		}
		if len(value) > field.MaxLength {
			return "", fmt.Errorf("%s内容过长", field.Label)
		}
		if field.Type == "select" && value != "" {
			found := false
			for _, option := range field.Options {
				if value == option {
					found = true
					break
				}
			}
			if !found {
				return "", fmt.Errorf("%s选项无效", field.Label)
			}
		}
		if field.Type == "number" && value != "" {
			if _, err := strconv.ParseInt(value, 10, 64); err != nil {
				return "", fmt.Errorf("%s必须是整数", field.Label)
			}
		}
		if value == "" {
			delete(values, key)
		} else {
			values[key] = value
		}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("购买表单数据无效")
	}
	return string(encoded), nil
}

type Product struct {
	Base
	Slug        string  `gorm:"size:120;uniqueIndex;not null" json:"slug"`
	Name        string  `gorm:"size:120;not null" json:"name"`
	Summary     *string `gorm:"size:255" json:"summary,omitempty"`
	Description *string `gorm:"type:text" json:"description,omitempty"`
	ImagePath   *string `gorm:"size:255" json:"image_path,omitempty"`
	ProductType string  `gorm:"size:32;default:'card';not null;index" json:"product_type"`
	// DeliveryChannel is the product's delivery channel: card (卡密自动发货),
	// manual (商家人工发货) or new_api (New-API 兑换码发货). It is the product-
	// level source of truth for New-API; an empty value on legacy rows falls
	// back to ProductType so old data keeps its meaning.
	DeliveryChannel string `gorm:"size:32;default:'';not null;index" json:"delivery_channel"`
	// MinTopupAmount/MaxTopupAmount bound the buyer-entered top-up amount for
	// New-API redemption products. They are ignored by card and manual
	// products, whose price and stock are what they are. The values are kept on
	// the product (rather than a per-item card) because New-API is a product
	// delivery channel, not a stock of pre-created codes.
	MinTopupAmount       int     `gorm:"default:0;not null" json:"min_topup_amount"`
	MaxTopupAmount       int     `gorm:"default:0;not null" json:"max_topup_amount"`
	DeliveryInstructions *string `gorm:"type:text" json:"delivery_instructions,omitempty"`
	RequireContact       bool    `gorm:"default:false;not null" json:"require_contact"`
	Price                int     `gorm:"not null" json:"price"`
	OriginalPrice        *int    `json:"original_price,omitempty"`
	// StockVisible, AutoDeliver and IsPublished carry no column default on purpose.
	// GORM omits a zero value from the INSERT when the column declares a default,
	// so "default:true" quietly turns an unchecked box back on: the shop hides the
	// stock count, or unpublishes a product, and the row says the opposite.
	StockVisible bool `gorm:"not null" json:"stock_visible"`
	StockCount   int  `gorm:"default:0;not null" json:"stock_count"`
	// SoldCount is delivered volume, not order volume: it goes up when goods
	// actually leave the shop and back down on refund, so the storefront can
	// show a number a buyer can believe.
	SoldCount int `gorm:"default:0;not null;index" json:"sold_count"`
	// IsFeatured marks the products the storefront promotes. Publishing is the
	// buyer-facing gate; featuring is only a placement choice on top of it.
	IsFeatured  bool       `gorm:"default:false;not null;index" json:"is_featured"`
	AutoDeliver bool       `gorm:"not null" json:"auto_deliver"`
	IsPublished bool       `gorm:"not null" json:"is_published"`
	IsArchived  bool       `gorm:"default:false;not null;index" json:"is_archived"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
	SortOrder   int        `gorm:"default:0;not null" json:"sort_order"`
	CategoryID  *uint      `json:"category_id,omitempty"`
	FormSchema  string     `gorm:"type:text" json:"form_schema,omitempty"`

	Category *Category `gorm:"foreignKey:CategoryID;" json:"category,omitempty"`
	Cards    []Card    `gorm:"foreignKey:ProductID;constraint:OnDelete:CASCADE;" json:"-"`
}

type Card struct {
	Base
	ProductID uint       `gorm:"not null;index:idx_product_status" json:"product_id"`
	Content   string     `gorm:"type:text;not null" json:"content"`
	Status    string     `gorm:"size:16;default:'available';not null;index:idx_product_status" json:"status"`
	OrderID   *uint      `json:"order_id,omitempty"`
	SoldAt    *time.Time `json:"sold_at,omitempty"`

	Product Product `gorm:"foreignKey:ProductID;" json:"-"`
	Order   Order   `gorm:"foreignKey:OrderID;" json:"-"`
}

// ── Order & Fulfillment ──────────────────────────────────────────────

type Order struct {
	Base
	OrderNo   string `gorm:"size:64;uniqueIndex;not null" json:"order_no"`
	UserID    uint   `gorm:"not null;index" json:"user_id"`
	ProductID uint   `gorm:"not null;index" json:"product_id"`
	Quantity  int    `gorm:"default:1;not null" json:"quantity"`
	// TopupAmount is the buyer-entered New-API top-up amount. It is stored so the
	// order (and the back office) can show exactly what was purchased and so
	// delivery recomputes quota from the recorded amount instead of trusting any
	// front-end conversion.
	TopupAmount int `gorm:"default:0;not null" json:"topup_amount"`
	UnitPrice   int `gorm:"not null" json:"unit_price"`
	// DiscountAmount is the coupon's cut, taken off before the order goes to
	// NodeLoc: TotalAmount is what the buyer actually pays, so 查单 amounts keep
	// matching without anyone recomputing the coupon later. It is counted in the
	// same unit the storefront prices and charges in, not a rescaled one.
	DiscountAmount int   `gorm:"default:0;not null" json:"discount_amount"`
	CouponID       *uint `json:"coupon_id,omitempty"`
	// 活动快照：历史订单必须保留当时参与的活动与优惠金额，后续改价不影响它。
	ActivityID       *uint  `gorm:"index" json:"activity_id,omitempty"`
	ActivityName     string `gorm:"size:160" json:"activity_name,omitempty"`
	ActivityDiscount int    `gorm:"default:0;not null" json:"activity_discount"`
	ActivitySnapshot string `gorm:"type:text" json:"activity_snapshot,omitempty"`
	// CouponCode is the code as the buyer typed it, kept on the order so a
	// discount explains itself in the back office without a join.
	CouponCode        string     `gorm:"size:64" json:"coupon_code,omitempty"`
	TotalAmount       int        `gorm:"not null" json:"total_amount"`
	Status            string     `gorm:"size:16;default:'pending';not null;index" json:"status"`
	TransactionID     *string    `gorm:"size:64;index" json:"transaction_id,omitempty"`
	PlatformFee       *int       `json:"platform_fee,omitempty"`
	MerchantPoints    *int       `json:"merchant_points,omitempty"`
	PaidAt            *time.Time `json:"paid_at,omitempty"`
	DeliveredAt       *time.Time `json:"delivered_at,omitempty"`
	FulfillmentStatus string     `gorm:"size:32;default:'pending';not null;index" json:"fulfillment_status"`
	CustomerContact   *string    `gorm:"size:255" json:"customer_contact,omitempty"`
	CustomerNote      *string    `gorm:"type:text" json:"customer_note,omitempty"`
	FormValues        string     `gorm:"type:text" json:"form_values,omitempty"`
	DeliveryContent   *string    `gorm:"type:text" json:"delivery_content,omitempty"`
	DeliveryNote      *string    `gorm:"type:text" json:"delivery_note,omitempty"`

	User    *User            `gorm:"foreignKey:UserID;" json:"user,omitempty"`
	Product *Product         `gorm:"foreignKey:ProductID;" json:"product,omitempty"`
	Cards   []Card           `gorm:"foreignKey:OrderID;" json:"-"`
	Records []DeliveryRecord `gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE;" json:"-"`
}

type DeliveryRecord struct {
	Base
	OrderID      uint       `gorm:"not null;index" json:"order_id"`
	Sequence     int        `gorm:"default:1;not null" json:"sequence"`
	DeliveryType string     `gorm:"size:32;not null" json:"delivery_type"`
	Status       string     `gorm:"size:32;not null;index" json:"status"`
	Content      *string    `gorm:"type:text" json:"content,omitempty"`
	Note         *string    `gorm:"type:text" json:"note,omitempty"`
	ActorID      *uint      `json:"actor_id,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`

	Order Order `gorm:"foreignKey:OrderID;" json:"-"`
}

// ── Coupon ───────────────────────────────────────────────────────────

type Coupon struct {
	Base
	Code           string     `gorm:"size:64;uniqueIndex;not null" json:"code"`
	DiscountType   string     `gorm:"size:16;not null" json:"discount_type"`
	DiscountValue  int        `gorm:"not null" json:"discount_value"`
	MinOrderAmount int        `gorm:"default:0;not null" json:"min_order_amount"`
	MaxUses        int        `gorm:"default:0;not null" json:"max_uses"`
	UsedCount      int        `gorm:"default:0;not null" json:"used_count"`
	ValidFrom      *time.Time `json:"valid_from,omitempty"`
	ValidUntil     *time.Time `json:"valid_until,omitempty"`
	// IsActive has no column default so that a coupon created in the "off" state
	// really is off; see the note on the product flags.
	IsActive bool `gorm:"not null" json:"is_active"`

	// Description is the storefront-facing one-liner ("新人首单立减 5 元").
	Description *string `gorm:"size:255" json:"description,omitempty"`
	// Advertised puts the code on the storefront's own promo shelf. It is off by
	// default: a code the shop means for one customer stays unlisted until the
	// shop says otherwise, and turning it on is a decision the owner makes while
	// looking at the whole promotion.
	Advertised bool `gorm:"default:false;not null" json:"advertised"`
	// Scope limits what the code can buy. all is the pre-existing behaviour;
	// category and product narrow it so a promotion cannot be spent on the one
	// item that was never meant to be discounted.
	Scope      string `gorm:"size:16;default:'all';not null" json:"scope"`
	CategoryID *uint  `json:"category_id,omitempty"`
	ProductID  *uint  `json:"product_id,omitempty"`
	// PerUserLimit caps how many orders one account may place with the code
	// (0 = unlimited). Without it a shared code gets spent until MaxUses runs
	// out by whoever heard about it first.
	PerUserLimit int `gorm:"default:0;not null" json:"per_user_limit"`
}

// ── Plugins ──────────────────────────────────────────────────────────

// Plugin is one extension the shop owner installs. The shop ships the runtime;
// what makes a plugin a plugin is this enrollment: which provider it is, whether
// it is switched on, and the non-secret settings it runs with.
//
// Settings holds provider configuration such as a base URL or a default template
// and is stored as JSON. It is never the place for keys: ConfigSecrets holds the
// credential half, is tagged json:"-", and never leaves the server through any
// read route.
type Plugin struct {
	Base
	// Key is the stable provider key (external-card-v1, for example). One row per
	// key: enrolling the same provider twice would give two copies of the same
	// order hook and deliver every purchase twice.
	Key         string `gorm:"size:64;uniqueIndex;not null" json:"key"`
	Name        string `gorm:"size:120;not null" json:"name"`
	Description string `gorm:"type:text" json:"description,omitempty"`
	Version     string `gorm:"size:32" json:"version"`
	Author      string `gorm:"size:120" json:"author,omitempty"`
	// IsEnabled is the owner's switch. Turning it off stops new orders from being
	// fulfilled through the plugin; it never rewrites orders already placed.
	IsEnabled bool   `gorm:"default:false;not null;index" json:"is_enabled"`
	Settings  string `gorm:"type:text" json:"settings,omitempty"`
	// ConfigSecrets is the credential half of the configuration (JSON). It is
	// never marshalled to a client.
	ConfigSecrets string `gorm:"type:text" json:"-"`
	// ConfigSchema is the JSON description of the settings form the back office
	// renders, so 插件管理 stays one generic screen as providers are added.
	ConfigSchema string `gorm:"type:text" json:"config_schema,omitempty"`
	// Capabilities is the JSON list of what this plugin can do (fulfill, form,
	// notify). The storefront and the back office read it to decide what to show.
	Capabilities string     `gorm:"type:text" json:"capabilities,omitempty"`
	InstalledAt  *time.Time `json:"installed_at,omitempty"`
	// ValidationWarning is a transient answer, not a column: it carries the
	// provider's own complaint about the configuration that was just stored, so
	// 插件管理 can say what is still missing without refusing the save.
	ValidationWarning string `gorm:"-" json:"validation_warning,omitempty"`
}

// PluginBinding attaches a plugin to one product and carries the value that
// decides which concrete product a purchase resolves to: a plan, a region, a
// template — whatever the provider needs.
//
// Value is normalized to lowercase on write. Matching is deliberately exact
// rather than clever: a purchase with no mapping is a refusal the buyer can read,
// never a silent substitution of the wrong product.
type PluginBinding struct {
	Base
	PluginID  uint `gorm:"not null;uniqueIndex:uniq_plugin_binding;index" json:"plugin_id"`
	ProductID uint `gorm:"not null;uniqueIndex:uniq_plugin_binding;index" json:"product_id"`
	// Value is what the buyer answered (typically a form field) that this binding
	// resolves. Empty matches a product whose plugin needs no such choice.
	Value string `gorm:"size:120;not null;default:'';uniqueIndex:uniq_plugin_binding" json:"value"`
	// MatchField names the purchase-form field whose answer is compared with
	// Value. It is what makes the standard uniform: every plugin says 「用哪个
	// 表单项来匹配」 in the same place, instead of each provider inventing a
	// different way to pick a delivery item.
	MatchField string `gorm:"size:64" json:"match_field,omitempty"`
	// RemoteName labels the resolved product for the back office, so a mapping
	// reads as 「这个选项发的是哪个商品」 without opening the provider.
	RemoteName string `gorm:"size:160" json:"remote_name,omitempty"`
	// RemoteRef is the provider-side identifier (sku, plan id, template id).
	RemoteRef string `gorm:"size:160" json:"remote_ref,omitempty"`
	// Extra is provider-specific JSON handed to the fulfillment hook.
	Extra string `gorm:"type:text" json:"extra,omitempty"`
	// IsEnabled lets one mapping be parked without deleting it.
	IsEnabled bool `gorm:"default:true;not null" json:"is_enabled"`
	SortOrder int  `gorm:"default:0;not null" json:"sort_order"`

	Plugin  Plugin  `gorm:"foreignKey:PluginID;constraint:OnDelete:CASCADE;" json:"-"`
	Product Product `gorm:"foreignKey:ProductID;constraint:OnDelete:CASCADE;" json:"-"`
}

func (PluginBinding) TableName() string { return "plugin_bindings" }

// ── Notification ─────────────────────────────────────────────────────

type Notification struct {
	Base
	UserID  uint    `gorm:"not null;index" json:"user_id"`
	Type    string  `gorm:"size:32;not null" json:"type"`
	Title   string  `gorm:"size:200;not null" json:"title"`
	Content *string `gorm:"type:text" json:"content,omitempty"`
	Link    *string `gorm:"size:500" json:"link,omitempty"`
	IsRead  bool    `gorm:"default:false;not null" json:"is_read"`

	User User `gorm:"foreignKey:UserID;" json:"-"`
}

// ── Audit Log ────────────────────────────────────────────────────────

type AuditLog struct {
	Base
	ActorID *uint   `json:"actor_id,omitempty"`
	Action  string  `gorm:"size:64;not null;index" json:"action"`
	Target  *string `gorm:"size:120" json:"target,omitempty"`
	Detail  *string `gorm:"type:text" json:"detail,omitempty"`
	IP      *string `gorm:"size:64" json:"ip,omitempty"`
	// ActorName is filled by the audit query so the log page can say who acted
	// instead of only a bare user id. It is not a column.
	ActorName string `gorm:"-" json:"actor_name,omitempty"`
}

// ── App Setting ──────────────────────────────────────────────────────

type AppSetting struct {
	Base
	// Key is unique: it is the lookup this table exists for, and an upsert
	// (the RBAC seed marker) needs the constraint to conflict on.
	Key   string  `gorm:"size:64;uniqueIndex:idx_app_settings_key;primarykey" json:"key"`
	Value *string `gorm:"type:text" json:"value,omitempty"`
}

// ── Migrate auto-migrates all models ─────────────────────────────────

func Migrate(db *gorm.DB) error {
	if err := migrateLegacyOAuthIdentities(db); err != nil {
		return err
	}
	return db.AutoMigrate(
		&User{},
		&OAuthIdentity{},
		&OAuthTransaction{},
		&OAuthAttempt{},
		&PointLedger{},
		&CheckIn{},
		&Category{},
		&Product{},
		&Card{},
		&Order{},
		&DeliveryRecord{},
		&Coupon{},
		&Plugin{},
		&PluginBinding{},
		&Notification{},
		&AuditLog{},
		&AppSetting{},

		// 活动营销
		&Activity{},
		&ActivityRule{},
		&ActivityRecord{},
		&ActivityLog{},
		&CouponRecord{},

		// 工单与客服
		&Ticket{},
		&TicketMessage{},
		&TicketLog{},
		&TicketAttachment{},
		&TicketAISession{},
		&AIConversation{},
		&AIMessage{},
		&AIToolDefinition{},
		&AIToolPermission{},
		&AIToolCall{},
		&AIKnowledgeCategory{},
		&AIKnowledge{},
		&AIFeedback{},
		&AIConfig{},
		&AIWorkflowConfig{},
		&AIQuickQuestion{},
		&CustomerServiceAgent{},
		&CustomerServiceAssignment{},
		&QuickReply{},
		&NotificationTemplate{},
		&NotificationLog{},
		&SystemConfig{},
		&OperationLog{},
	)
}

// migrateLegacyOAuthIdentities copies identity rows out of the table GORM
// derived before OAuthIdentity was pinned to oauth_identities. SQLite index
// names are database-wide, so that legacy table's indexes block AutoMigrate
// from creating this model's, which fatal-loops any container upgraded from an
// older image. It runs before AutoMigrate and only while the new table is
// still empty. The legacy table is kept: rolling the image back expects it.
func migrateLegacyOAuthIdentities(db *gorm.DB) error {
	m := db.Migrator()
	if !m.HasTable(legacyOAuthTable) {
		return nil
	}
	if !m.HasTable(&OAuthIdentity{}) {
		return m.CreateTable(&OAuthIdentity{})
	}
	var rows int64
	if err := db.Table("oauth_identities").Count(&rows).Error; err != nil || rows > 0 {
		return err
	}
	// Rows whose user was deleted can never authenticate anyone, and copying
	// them trips the foreign key, so they stay behind.
	return db.Exec(`INSERT INTO oauth_identities
		(id, created_at, updated_at, deleted_at, user_id, provider, provider_uid,
		 username, display_name, avatar_url, scope, access_token, refresh_token)
		SELECT id, created_at, updated_at, deleted_at, user_id, provider, provider_uid,
		 username, display_name, avatar_url, scope, access_token, refresh_token
		FROM ` + legacyOAuthTable + `
		WHERE user_id IN (SELECT id FROM users)`).Error
}
