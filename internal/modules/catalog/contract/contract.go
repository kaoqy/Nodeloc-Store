package contract

import (
	"context"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/catalog/domain"
)

// ProductRepo defines persistence operations for catalog products.
type ProductRepo interface {
	// ListQuery is the storefront read: filtered, ranked and paged in the
	// database, returning the total the page belongs to.
	ListQuery(ctx context.Context, query domain.ProductQuery) ([]domain.Product, int64, error)
	GetByID(ctx context.Context, id uint) (*domain.Product, error)
	GetBySlug(ctx context.Context, slug string, publishedOnly bool) (*domain.Product, error)
	Create(ctx context.Context, product *domain.Product) error
	Update(ctx context.Context, product *domain.Product) error
	Delete(ctx context.Context, id uint) error
	UpdateStockCount(ctx context.Context, productID uint, count int) error
	// CountByCategory maps category id to the number of products a buyer can
	// currently reach, which is what the category chips are labelled with.
	CountByCategory(ctx context.Context, publishedOnly bool) (map[uint]int64, error)
	Stats(ctx context.Context, publishedOnly bool) (domain.StoreStats, error)
	ListLowStock(ctx context.Context, threshold int) ([]domain.Product, error)
}

// CardRepo defines persistence and stock operations for product cards.
type CardRepo interface {
	ListByProduct(ctx context.Context, productID uint) ([]domain.Card, error)
	// ListFiltered is the back-office inventory view across products, with the
	// owning order loaded so a sold card can be traced back to a buyer.
	ListFiltered(ctx context.Context, filter domain.CardFilter) ([]domain.Card, int64, error)
	GetByID(ctx context.Context, id uint) (*domain.Card, error)
	Create(ctx context.Context, card *domain.Card) error
	CreateBatch(ctx context.Context, cards []domain.Card) error
	Update(ctx context.Context, card *domain.Card) error
	Delete(ctx context.Context, id uint) error
	CountByStatus(ctx context.Context, productID uint, status string) (int64, error)
	TakeAvailable(ctx context.Context, productID uint, orderID uint) (*domain.Card, error)
	// ExistingContents returns the card values a product already holds, so an
	// import can skip duplicates instead of selling the same key twice.
	ExistingContents(ctx context.Context, productID uint) (map[string]struct{}, error)
	// BulkStatus and BulkDelete act on the ids the shop owner selected and
	// return how many rows actually moved; cards already delivered are left
	// alone, because they belong to a buyer now.
	BulkStatus(ctx context.Context, productID uint, ids []uint, status string) (int64, error)
	BulkDelete(ctx context.Context, productID uint, ids []uint) (int64, error)
}

// CategoryRepo defines persistence operations for product categories.
type CategoryRepo interface {
	List(ctx context.Context, visibleOnly bool) ([]domain.Category, error)
	GetByID(ctx context.Context, id uint) (*domain.Category, error)
	Create(ctx context.Context, category *domain.Category) error
	Update(ctx context.Context, category *domain.Category) error
	Delete(ctx context.Context, id uint) error
}

// CouponRepo defines persistence operations for coupons.
type CouponRepo interface {
	List(ctx context.Context) ([]domain.Coupon, error)
	// ListAdvertised is the storefront's promo shelf: only codes the shop put on
	// display and left switched on. The validity window and the remaining quota
	// are judged above the store, because they need the clock and live orders.
	ListAdvertised(ctx context.Context) ([]domain.Coupon, error)
	GetByID(ctx context.Context, id uint) (*domain.Coupon, error)
	GetByCode(ctx context.Context, code string) (*domain.Coupon, error)
	Create(ctx context.Context, coupon *domain.Coupon) error
	Update(ctx context.Context, coupon *domain.Coupon) error
	Delete(ctx context.Context, id uint) error
	// UsedBy counts the orders an account has already placed with this code,
	// including the pending ones, so 每人限用 can be enforced before the money
	// moves rather than reconciled after.
	UsedBy(ctx context.Context, couponID, userID uint) (int64, error)
	// UsedTotal counts the live orders a code sits on, shop-wide. 总量限用 is
	// checked against this rather than the coupon's used_count, because that
	// column only moves when NodeLoc confirms a payment: two buyers racing to
	// check out would both be told 额度还有 for the last code.
	UsedTotal(ctx context.Context, couponID uint) (int64, error)
}
