package infrastructure

import (
	"context"
	"errors"
	"strings"
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

// asTaken replaces a unique-index collision with the catalogue rule it broke.
// Left alone, the driver's own sentence ("UNIQUE constraint failed:
// products.slug") travels all the way to the shop owner's screen, where it reads
// as a fault rather than as the duplicate slug it is.
func asTaken(err error, taken error) error {
	if !isDuplicateKey(err) {
		return err
	}
	return taken
}

// isDuplicateKey recognises both drivers by hand because GORM's error translation
// is not turned on for this project: SQLite names the index, MySQL reports 1062.
func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	text := err.Error()
	return strings.Contains(text, "UNIQUE constraint failed") ||
		strings.Contains(text, "Error 1062") ||
		strings.Contains(text, "duplicate key value")
}

// ListQuery applies the storefront's filters in the database. Manual-delivery
// products always count as in stock, because 现货 for them is the shop owner's
// time, not a card row.
func (r *GormProductRepo) ListQuery(ctx context.Context, query domain.ProductQuery) ([]domain.Product, int64, error) {
	apply := func(db *gorm.DB) *gorm.DB {
		db = db.Where("is_archived = ?", false)
		if query.PublishedOnly {
			db = db.Where("is_published = ?", true)
		}
		if query.CategoryID != nil && *query.CategoryID != 0 {
			db = db.Where("category_id = ?", *query.CategoryID)
		}
		if query.FeaturedOnly {
			db = db.Where("is_featured = ?", true)
		}
		if query.InStockOnly {
			db = db.Where("(product_type = ? OR stock_count > 0)", domain.ProductTypeManual)
		}
		if pattern := strings.ToLower(strings.TrimSpace(query.Search)); pattern != "" {
			like := "%" + pattern + "%"
			db = db.Where("LOWER(name) LIKE ? OR LOWER(COALESCE(summary, '')) LIKE ? OR LOWER(COALESCE(description, '')) LIKE ?", like, like, like)
		}
		return db
	}

	var total int64
	if err := apply(r.db.WithContext(ctx).Model(&domain.Product{})).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 24
	}
	if limit > 100 {
		limit = 100
	}
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}
	var products []domain.Product
	err := apply(r.db.WithContext(ctx).Preload("Category")).
		Order(productOrder(query.Sort)).
		Limit(limit).Offset(offset).
		Find(&products).Error
	return products, total, err
}

func productOrder(sort string) string {
	switch sort {
	case domain.SortSalesDesc:
		return "sold_count DESC, sort_order ASC, id DESC"
	case domain.SortPriceAsc:
		return "price ASC, id DESC"
	case domain.SortPriceDesc:
		return "price DESC, id DESC"
	case domain.SortNewest:
		return "created_at DESC, id DESC"
	}
	return "sort_order ASC, id DESC"
}

// CountByCategory labels the storefront's category chips. Products with no
// category are left out; they belong to 全部, not to a chip.
func (r *GormProductRepo) CountByCategory(ctx context.Context, publishedOnly bool) (map[uint]int64, error) {
	type row struct {
		CategoryID uint
		Total      int64
	}
	db := r.db.WithContext(ctx).Model(&domain.Product{}).
		Select("category_id, COUNT(*) AS total").
		Where("is_archived = ? AND category_id IS NOT NULL", false)
	if publishedOnly {
		db = db.Where("is_published = ?", true)
	}
	var rows []row
	if err := db.Group("category_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	counts := make(map[uint]int64, len(rows))
	for _, item := range rows {
		counts[item.CategoryID] = item.Total
	}
	return counts, nil
}

func (r *GormProductRepo) Stats(ctx context.Context, publishedOnly bool) (domain.StoreStats, error) {
	var stats domain.StoreStats
	aggregate := struct {
		Products int64 `gorm:"column:products"`
		Stock    int64 `gorm:"column:stock"`
		Sales    int64 `gorm:"column:sales"`
	}{}
	db := r.db.WithContext(ctx).Model(&domain.Product{}).
		Select("COUNT(*) AS products, COALESCE(SUM(stock_count), 0) AS stock, COALESCE(SUM(sold_count), 0) AS sales").
		Where("is_archived = ?", false)
	if publishedOnly {
		db = db.Where("is_published = ?", true)
	}
	if err := db.Scan(&aggregate).Error; err != nil {
		return stats, err
	}
	stats.Products, stats.Stock, stats.Sales = aggregate.Products, aggregate.Stock, aggregate.Sales

	categories := r.db.WithContext(ctx).Model(&domain.Category{}).Where("is_visible = ?", true)
	if !publishedOnly {
		categories = r.db.WithContext(ctx).Model(&domain.Category{})
	}
	err := categories.Count(&stats.Categories).Error
	return stats, err
}

// ListLowStock is the restocking queue: card products whose remaining keys have
// dropped to the shop's warning threshold.
func (r *GormProductRepo) ListLowStock(ctx context.Context, threshold int) ([]domain.Product, error) {
	var products []domain.Product
	err := r.db.WithContext(ctx).Preload("Category").
		Where("is_archived = ? AND is_published = ? AND product_type = ? AND stock_count <= ?",
			false, true, domain.ProductTypeCard, threshold).
		Order("stock_count ASC, id ASC").Limit(100).Find(&products).Error
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
	return asTaken(r.db.WithContext(ctx).Create(product).Error, domain.ErrProductSlugTaken)
}

func (r *GormProductRepo) Update(ctx context.Context, product *domain.Product) error {
	return asTaken(r.db.WithContext(ctx).Save(product).Error, domain.ErrProductSlugTaken)
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

// ListFiltered is the back office's card inventory read, across products when
// no product is selected. The owning order is loaded so 已售 cards can name the
// buyer's order instead of an opaque id.
func (r *GormCardRepo) ListFiltered(ctx context.Context, filter domain.CardFilter) ([]domain.Card, int64, error) {
	apply := func(db *gorm.DB) *gorm.DB {
		if filter.ProductID != nil && *filter.ProductID != 0 {
			db = db.Where("product_id = ?", *filter.ProductID)
		}
		if status := strings.TrimSpace(filter.Status); status != "" && status != "all" {
			db = db.Where("status = ?", status)
		}
		if pattern := strings.ToLower(strings.TrimSpace(filter.Search)); pattern != "" {
			db = db.Where("LOWER(content) LIKE ?", "%"+pattern+"%")
		}
		return db
	}

	var total int64
	if err := apply(r.db.WithContext(ctx).Model(&domain.Card{})).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	var cards []domain.Card
	err := apply(r.db.WithContext(ctx)).
		Preload("Product").Preload("Order").
		Order("id ASC").Limit(limit).Offset(offset).
		Find(&cards).Error
	return cards, total, err
}

func (r *GormCardRepo) ExistingContents(ctx context.Context, productID uint) (map[string]struct{}, error) {
	var contents []string
	if err := r.db.WithContext(ctx).Model(&domain.Card{}).
		Where("product_id = ?", productID).Pluck("content", &contents).Error; err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(contents))
	for _, content := range contents {
		set[content] = struct{}{}
	}
	return set, nil
}

// BulkStatus moves selected cards between 可用 and 停用. Cards already sold are
// skipped on purpose: they belong to a buyer's order, and quietly re-issuing one
// would hand out the same key twice.
func (r *GormCardRepo) BulkStatus(ctx context.Context, productID uint, ids []uint, status string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).Model(&domain.Card{}).
		Where("id IN ? AND product_id = ? AND status IN ?", ids, productID,
			[]string{domain.CardStatusAvailable, domain.CardStatusDisabled}).
		Update("status", status)
	return result.RowsAffected, result.Error
}

// BulkDelete drops every selected key except the ones a buyer already paid for.
// Matching on status <> sold rather than = available matters because disabling a
// key is the step right before deleting it in the inventory workbench: an
// available-only filter would report 0 deleted for cards the owner just turned
// off, and the batch would look as though it had been silently refused.
func (r *GormCardRepo) BulkDelete(ctx context.Context, productID uint, ids []uint) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).
		Where("id IN ? AND product_id = ? AND status <> ?", ids, productID, domain.CardStatusSold).
		Delete(&domain.Card{})
	return result.RowsAffected, result.Error
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
	return asTaken(r.db.WithContext(ctx).Create(category).Error, domain.ErrCategorySlugTaken)
}

func (r *GormCategoryRepo) Update(ctx context.Context, category *domain.Category) error {
	return asTaken(r.db.WithContext(ctx).Save(category).Error, domain.ErrCategorySlugTaken)
}

func (r *GormCategoryRepo) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&domain.Category{}, id).Error
}

func (r *GormCouponRepo) List(ctx context.Context) ([]domain.Coupon, error) {
	var coupons []domain.Coupon
	err := r.db.WithContext(ctx).Order("id DESC").Find(&coupons).Error
	return coupons, err
}

// ListAdvertised is the storefront's promo shelf. Only the switched-on codes the
// shop chose to advertise come back; the window and the quota are judged above.
func (r *GormCouponRepo) ListAdvertised(ctx context.Context) ([]domain.Coupon, error) {
	var coupons []domain.Coupon
	err := r.db.WithContext(ctx).
		Where("advertised = ? AND is_active = ?", true, true).
		Order("id DESC").
		Find(&coupons).Error
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
	return asTaken(r.db.WithContext(ctx).Create(coupon).Error, domain.ErrCouponCodeTaken)
}

func (r *GormCouponRepo) Update(ctx context.Context, coupon *domain.Coupon) error {
	return asTaken(r.db.WithContext(ctx).Save(coupon).Error, domain.ErrCouponCodeTaken)
}

func (r *GormCouponRepo) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&domain.Coupon{}, id).Error
}

// UsedBy counts the orders an account holds against this code. A cancelled
// order is not a spend, and neither is an unpaid one the buyer walked away
// from before since — otherwise every abandoned 立即购买 permanently burns a
// slot of 每人限用 while the shop's counter still reads 0.
func (r *GormCouponRepo) UsedBy(ctx context.Context, couponID, userID uint, since time.Time) (int64, error) {
	var count int64
	err := r.quotaHeld(ctx, couponID, since).Where("user_id = ?", userID).Count(&count).Error
	return count, err
}

// UsedTotal is the same count without the account filter: how many orders this
// code is still committed to, paid ones and checkouts that are fresh enough to
// still be in front of a buyer.
func (r *GormCouponRepo) UsedTotal(ctx context.Context, couponID uint, since time.Time) (int64, error) {
	var count int64
	err := r.quotaHeld(ctx, couponID, since).Count(&count).Error
	return count, err
}

// quotaHeld is the one rule both quota counts share, spelled once so the promo
// shelf, 总量限用 and 每人限用 cannot drift apart.
func (r *GormCouponRepo) quotaHeld(ctx context.Context, couponID uint, since time.Time) *gorm.DB {
	return r.db.WithContext(ctx).Table("orders").
		Where("coupon_id = ? AND deleted_at IS NULL", couponID).
		Where("status <> ?", "cancelled").
		Where("status <> ? OR created_at >= ?", "pending", since)
}

// HeldByCoupons runs quotaHeld's rule for every code at once, because the promo
// table lists them all. Codes nobody is holding simply do not appear.
func (r *GormCouponRepo) HeldByCoupons(ctx context.Context, since time.Time) (map[uint]int64, error) {
	var rows []struct {
		CouponID uint
		Held     int64
	}
	err := r.db.WithContext(ctx).Table("orders").
		Select("coupon_id, COUNT(*) AS held").
		Where("coupon_id IS NOT NULL AND deleted_at IS NULL").
		Where("status <> ?", "cancelled").
		Where("status <> ? OR created_at >= ?", "pending", since).
		Group("coupon_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	held := make(map[uint]int64, len(rows))
	for _, row := range rows {
		held[row.CouponID] = row.Held
	}
	return held, nil
}
