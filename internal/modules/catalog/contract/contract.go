package contract

import (
	"context"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/catalog/domain"
)

// ProductRepo defines persistence operations for catalog products.
type ProductRepo interface {
	List(ctx context.Context, publishedOnly bool) ([]domain.Product, error)
	GetByID(ctx context.Context, id uint) (*domain.Product, error)
	GetBySlug(ctx context.Context, slug string, publishedOnly bool) (*domain.Product, error)
	Create(ctx context.Context, product *domain.Product) error
	Update(ctx context.Context, product *domain.Product) error
	Delete(ctx context.Context, id uint) error
	UpdateStockCount(ctx context.Context, productID uint, count int) error
}

// CardRepo defines persistence and stock operations for product cards.
type CardRepo interface {
	ListByProduct(ctx context.Context, productID uint) ([]domain.Card, error)
	GetByID(ctx context.Context, id uint) (*domain.Card, error)
	Create(ctx context.Context, card *domain.Card) error
	CreateBatch(ctx context.Context, cards []domain.Card) error
	Update(ctx context.Context, card *domain.Card) error
	Delete(ctx context.Context, id uint) error
	CountByStatus(ctx context.Context, productID uint, status string) (int64, error)
	TakeAvailable(ctx context.Context, productID uint, orderID uint) (*domain.Card, error)
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
	GetByID(ctx context.Context, id uint) (*domain.Coupon, error)
	GetByCode(ctx context.Context, code string) (*domain.Coupon, error)
	Create(ctx context.Context, coupon *domain.Coupon) error
	Update(ctx context.Context, coupon *domain.Coupon) error
	Delete(ctx context.Context, id uint) error
}
