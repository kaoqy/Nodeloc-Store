package infrastructure

import (
	"context"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/catalog/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GormProductRepo struct{ db *gorm.DB }
type GormCardRepo struct{ db *gorm.DB }
type GormCategoryRepo struct{ db *gorm.DB }
type GormCouponRepo struct{ db *gorm.DB }

func NewProductRepo(db *gorm.DB) *GormProductRepo   { return &GormProductRepo{db: db} }
func NewCardRepo(db *gorm.DB) *GormCardRepo         { return &GormCardRepo{db: db} }
func NewCategoryRepo(db *gorm.DB) *GormCategoryRepo { return &GormCategoryRepo{db: db} }
func NewCouponRepo(db *gorm.DB) *GormCouponRepo     { return &GormCouponRepo{db: db} }

func (r *GormProductRepo) List(ctx context.Context, publishedOnly bool) ([]domain.Product, error) {
	var products []domain.Product
	q := r.db.WithContext(ctx).Preload("Category").Where("is_archived = ?", false)
	if publishedOnly {
		q = q.Where("is_published = ?", true)
	}
	err := q.Order("sort_order ASC, id DESC").Find(&products).Error
	return products, err
}

func (r *GormProductRepo) GetByID(ctx context.Context, id uint) (*domain.Product, error) {
	var product domain.Product
	if err := r.db.WithContext(ctx).Preload("Category").First(&product, id).Error; err != nil {
		return nil, err
	}
	return &product, nil
}

func (r *GormProductRepo) GetBySlug(ctx context.Context, slug string, publishedOnly bool) (*domain.Product, error) {
	var product domain.Product
	q := r.db.WithContext(ctx).Preload("Category").Where("slug = ? AND is_archived = ?", slug, false)
	if publishedOnly {
		q = q.Where("is_published = ?", true)
	}
	if err := q.First(&product).Error; err != nil {
		return nil, err
	}
	return &product, nil
}

func (r *GormProductRepo) Create(ctx context.Context, product *domain.Product) error {
	return r.db.WithContext(ctx).Create(product).Error
}

func (r *GormProductRepo) Update(ctx context.Context, product *domain.Product) error {
	return r.db.WithContext(ctx).Save(product).Error
}

func (r *GormProductRepo) UpdateStockCount(ctx context.Context, productID uint, count int) error {
	return r.db.WithContext(ctx).Model(&domain.Product{}).Where("id = ?", productID).UpdateColumn("stock_count", count).Error
}

func (r *GormProductRepo) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&domain.Product{}, id).Error
}

func (r *GormCardRepo) ListByProduct(ctx context.Context, productID uint) ([]domain.Card, error) {
	var cards []domain.Card
	err := r.db.WithContext(ctx).Where("product_id = ?", productID).Order("id ASC").Find(&cards).Error
	return cards, err
}

func (r *GormCardRepo) GetByID(ctx context.Context, id uint) (*domain.Card, error) {
	var card domain.Card
	if err := r.db.WithContext(ctx).First(&card, id).Error; err != nil {
		return nil, err
	}
	return &card, nil
}

func (r *GormCardRepo) Create(ctx context.Context, card *domain.Card) error {
	return r.db.WithContext(ctx).Create(card).Error
}

func (r *GormCardRepo) BulkCreate(ctx context.Context, cards []domain.Card) error {
	if len(cards) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(cards, 500).Error
}

func (r *GormCardRepo) CreateBatch(ctx context.Context, cards []domain.Card) error {
	return r.BulkCreate(ctx, cards)
}

func (r *GormCardRepo) Update(ctx context.Context, card *domain.Card) error {
	return r.db.WithContext(ctx).Save(card).Error
}

func (r *GormCardRepo) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&domain.Card{}, id).Error
}

func (r *GormCardRepo) CountByProduct(ctx context.Context, productID uint) (int64, int64, error) {
	var available, total int64
	r.db.WithContext(ctx).Model(&domain.Card{}).Where("product_id = ? AND status = ?", productID, domain.CardStatusAvailable).Count(&available)
	r.db.WithContext(ctx).Model(&domain.Card{}).Where("product_id = ?", productID).Count(&total)
	return available, total, nil
}

func (r *GormCardRepo) UpdateProductStock(ctx context.Context, productID uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&domain.Card{}).Where("product_id = ? AND status = ?", productID, domain.CardStatusAvailable).Count(&count).Error; err != nil {
			return err
		}
		return tx.Model(&domain.Product{}).Where("id = ?", productID).
			UpdateColumn("stock_count", count).Error
	})
}

func (r *GormCardRepo) CountByStatus(ctx context.Context, productID uint, status string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.Card{}).
		Where("product_id = ? AND status = ?", productID, status).Count(&count).Error
	return count, err
}

func (r *GormCardRepo) TakeAvailable(ctx context.Context, productID uint, orderID uint) (*domain.Card, error) {
	var card domain.Card
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("product_id = ? AND status = ?", productID, domain.CardStatusAvailable).
			Order("id ASC").First(&card).Error; err != nil {
			return err
		}
		now := time.Now()
		result := tx.Model(&domain.Card{}).
			Where("id = ? AND status = ?", card.ID, domain.CardStatusAvailable).
			Updates(map[string]any{"status": domain.CardStatusSold, "order_id": orderID, "sold_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		if err := tx.Model(&domain.Product{}).Where("id = ?", productID).
			UpdateColumn("stock_count", gorm.Expr("CASE WHEN stock_count > 0 THEN stock_count - 1 ELSE 0 END")).Error; err != nil {
			return err
		}
		card.Status = domain.CardStatusSold
		card.OrderID = &orderID
		card.SoldAt = &now
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &card, nil
}

func (r *GormCategoryRepo) List(ctx context.Context, visibleOnly bool) ([]domain.Category, error) {
	var categories []domain.Category
	q := r.db.WithContext(ctx)
	if visibleOnly {
		q = q.Where("is_visible = ?", true)
	}
	err := q.Order("sort_order ASC, id ASC").Find(&categories).Error
	return categories, err
}

func (r *GormCategoryRepo) GetByID(ctx context.Context, id uint) (*domain.Category, error) {
	var category domain.Category
	if err := r.db.WithContext(ctx).First(&category, id).Error; err != nil {
		return nil, err
	}
	return &category, nil
}

func (r *GormCategoryRepo) Create(ctx context.Context, category *domain.Category) error {
	return r.db.WithContext(ctx).Create(category).Error
}

func (r *GormCategoryRepo) Update(ctx context.Context, category *domain.Category) error {
	return r.db.WithContext(ctx).Save(category).Error
}

func (r *GormCategoryRepo) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&domain.Category{}, id).Error
}

func (r *GormCouponRepo) List(ctx context.Context) ([]domain.Coupon, error) {
	var coupons []domain.Coupon
	err := r.db.WithContext(ctx).Order("id DESC").Find(&coupons).Error
	return coupons, err
}

func (r *GormCouponRepo) GetByID(ctx context.Context, id uint) (*domain.Coupon, error) {
	var coupon domain.Coupon
	if err := r.db.WithContext(ctx).First(&coupon, id).Error; err != nil {
		return nil, err
	}
	return &coupon, nil
}

func (r *GormCouponRepo) GetByCode(ctx context.Context, code string) (*domain.Coupon, error) {
	var coupon domain.Coupon
	if err := r.db.WithContext(ctx).Where("code = ?", code).First(&coupon).Error; err != nil {
		return nil, err
	}
	return &coupon, nil
}

func (r *GormCouponRepo) Create(ctx context.Context, coupon *domain.Coupon) error {
	return r.db.WithContext(ctx).Create(coupon).Error
}

func (r *GormCouponRepo) Update(ctx context.Context, coupon *domain.Coupon) error {
	return r.db.WithContext(ctx).Save(coupon).Error
}

func (r *GormCouponRepo) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&domain.Coupon{}, id).Error
}
