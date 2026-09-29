package models

import (
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

type Product struct {
	Base
	Slug                 string  `gorm:"size:120;uniqueIndex;not null" json:"slug"`
	Name                 string  `gorm:"size:120;not null" json:"name"`
	Summary              *string `gorm:"size:255" json:"summary,omitempty"`
	Description          *string `gorm:"type:text" json:"description,omitempty"`
	ImagePath            *string `gorm:"size:255" json:"image_path,omitempty"`
	ProductType          string  `gorm:"size:32;default:'card';not null;index" json:"product_type"`
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
	UnitPrice int    `gorm:"not null" json:"unit_price"`
	// DiscountAmount is the coupon's cut, taken off before the order goes to
	// NodeLoc: TotalAmount is what the buyer actually pays, so 查单 amounts keep
	// matching without anyone recomputing the coupon later. It is counted in the
	// same unit the storefront prices and charges in, not a rescaled one.
	DiscountAmount int   `gorm:"default:0;not null" json:"discount_amount"`
	CouponID       *uint `json:"coupon_id,omitempty"`
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
		&PointLedger{},
		&CheckIn{},
		&Category{},
		&Product{},
		&Card{},
		&Order{},
		&DeliveryRecord{},
		&Coupon{},
		&Notification{},
		&AuditLog{},
		&AppSetting{},
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
