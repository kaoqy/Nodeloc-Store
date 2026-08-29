package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/catalog/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/catalog/domain"
)

var (
	ErrInvalidProductType = errors.New("invalid product type")
	ErrInvalidCardStatus  = errors.New("invalid card status")
	ErrManualProductCard  = errors.New("cards can only be assigned to card products")
)

type Service struct {
	products   contract.ProductRepo
	cards      contract.CardRepo
	categories contract.CategoryRepo
	coupons    contract.CouponRepo
}

func NewService(products contract.ProductRepo, cards contract.CardRepo, categories contract.CategoryRepo, coupons contract.CouponRepo) *Service {
	return &Service{
		products:   products,
		cards:      cards,
		categories: categories,
		coupons:    coupons,
	}
}

func (s *Service) ListPublicProducts(ctx context.Context) ([]domain.Product, error) {
	return s.products.List(ctx, true)
}

func (s *Service) ListProducts(ctx context.Context) ([]domain.Product, error) {
	return s.products.List(ctx, false)
}

func (s *Service) GetPublicProduct(ctx context.Context, slug string) (*domain.Product, error) {
	return s.products.GetBySlug(ctx, strings.TrimSpace(slug), true)
}

func (s *Service) GetProduct(ctx context.Context, id uint) (*domain.Product, error) {
	return s.products.GetByID(ctx, id)
}

func (s *Service) CreateProduct(ctx context.Context, product *domain.Product) error {
	if product == nil {
		return errors.New("product is required")
	}
	if err := normalizeProduct(product); err != nil {
		return err
	}
	product.StockCount = 0
	return s.products.Create(ctx, product)
}

func (s *Service) UpdateProduct(ctx context.Context, id uint, input *domain.Product) (*domain.Product, error) {
	if input == nil {
		return nil, errors.New("product is required")
	}
	product, err := s.products.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	stockCount := product.StockCount
	createdAt := product.CreatedAt
	deletedAt := product.DeletedAt
	*product = *input
	product.ID = id
	product.CreatedAt = createdAt
	product.DeletedAt = deletedAt
	product.StockCount = stockCount
	if err := normalizeProduct(product); err != nil {
		return nil, err
	}
	if product.ProductType == domain.ProductTypeManual {
		product.StockCount = 0
	}
	if err := s.products.Update(ctx, product); err != nil {
		return nil, err
	}
	return product, nil
}

func (s *Service) DeleteProduct(ctx context.Context, id uint) error {
	return s.products.Delete(ctx, id)
}

func (s *Service) ListCards(ctx context.Context, productID uint) ([]domain.Card, error) {
	if _, err := s.products.GetByID(ctx, productID); err != nil {
		return nil, err
	}
	return s.cards.ListByProduct(ctx, productID)
}

func (s *Service) AddCard(ctx context.Context, productID uint, card *domain.Card) error {
	if card == nil {
		return errors.New("card is required")
	}
	if err := s.ensureCardProduct(ctx, productID); err != nil {
		return err
	}
	card.ProductID = productID
	card.Content = strings.TrimSpace(card.Content)
	if card.Content == "" {
		return errors.New("card content is required")
	}
	if card.Status == "" {
		card.Status = domain.CardStatusAvailable
	}
	if !validCardStatus(card.Status) {
		return ErrInvalidCardStatus
	}
	if err := s.cards.Create(ctx, card); err != nil {
		return err
	}
	return s.SyncStock(ctx, productID)
}

func (s *Service) AddCards(ctx context.Context, productID uint, contents []string) ([]domain.Card, error) {
	if err := s.ensureCardProduct(ctx, productID); err != nil {
		return nil, err
	}
	cards := make([]domain.Card, 0, len(contents))
	for _, content := range contents {
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}
		cards = append(cards, domain.Card{
			ProductID: productID,
			Content:   content,
			Status:    domain.CardStatusAvailable,
		})
	}
	if len(cards) == 0 {
		return nil, errors.New("at least one non-empty card is required")
	}
	if err := s.cards.CreateBatch(ctx, cards); err != nil {
		return nil, err
	}
	if err := s.SyncStock(ctx, productID); err != nil {
		return nil, err
	}
	return cards, nil
}

func (s *Service) UpdateCard(ctx context.Context, productID, cardID uint, input *domain.Card) (*domain.Card, error) {
	if input == nil {
		return nil, errors.New("card is required")
	}
	card, err := s.cards.GetByID(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if card.ProductID != productID {
		return nil, errors.New("card does not belong to product")
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, errors.New("card content is required")
	}
	status := input.Status
	if status == "" {
		status = card.Status
	}
	if !validCardStatus(status) {
		return nil, ErrInvalidCardStatus
	}
	card.Content = content
	card.Status = status
	if err := s.cards.Update(ctx, card); err != nil {
		return nil, err
	}
	if err := s.SyncStock(ctx, productID); err != nil {
		return nil, err
	}
	return card, nil
}

func (s *Service) DeleteCard(ctx context.Context, productID, cardID uint) error {
	card, err := s.cards.GetByID(ctx, cardID)
	if err != nil {
		return err
	}
	if card.ProductID != productID {
		return errors.New("card does not belong to product")
	}
	if err := s.cards.Delete(ctx, cardID); err != nil {
		return err
	}
	return s.SyncStock(ctx, productID)
}

func (s *Service) DeliverCard(ctx context.Context, productID, orderID uint) (*domain.Card, error) {
	if orderID == 0 {
		return nil, errors.New("order id is required")
	}
	if err := s.ensureCardProduct(ctx, productID); err != nil {
		return nil, err
	}
	return s.cards.TakeAvailable(ctx, productID, orderID)
}

func (s *Service) SyncStock(ctx context.Context, productID uint) error {
	product, err := s.products.GetByID(ctx, productID)
	if err != nil {
		return err
	}
	if product.ProductType == domain.ProductTypeManual {
		return s.products.UpdateStockCount(ctx, productID, 0)
	}
	count, err := s.cards.CountByStatus(ctx, productID, domain.CardStatusAvailable)
	if err != nil {
		return err
	}
	if count > int64(^uint(0)>>1) {
		return fmt.Errorf("stock count overflow: %d", count)
	}
	return s.products.UpdateStockCount(ctx, productID, int(count))
}

func (s *Service) ListPublicCategories(ctx context.Context) ([]domain.Category, error) {
	return s.categories.List(ctx, true)
}

func (s *Service) ListCategories(ctx context.Context) ([]domain.Category, error) {
	return s.categories.List(ctx, false)
}

func (s *Service) CreateCategory(ctx context.Context, category *domain.Category) error {
	if category == nil {
		return errors.New("category is required")
	}
	category.Name = strings.TrimSpace(category.Name)
	category.Slug = strings.TrimSpace(category.Slug)
	if category.Name == "" || category.Slug == "" {
		return errors.New("category name and slug are required")
	}
	return s.categories.Create(ctx, category)
}

func (s *Service) UpdateCategory(ctx context.Context, id uint, input *domain.Category) (*domain.Category, error) {
	if input == nil {
		return nil, errors.New("category is required")
	}
	category, err := s.categories.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	createdAt := category.CreatedAt
	deletedAt := category.DeletedAt
	*category = *input
	category.ID = id
	category.CreatedAt = createdAt
	category.DeletedAt = deletedAt
	category.Name = strings.TrimSpace(category.Name)
	category.Slug = strings.TrimSpace(category.Slug)
	if category.Name == "" || category.Slug == "" {
		return nil, errors.New("category name and slug are required")
	}
	if err := s.categories.Update(ctx, category); err != nil {
		return nil, err
	}
	return category, nil
}

func (s *Service) DeleteCategory(ctx context.Context, id uint) error {
	return s.categories.Delete(ctx, id)
}

func (s *Service) ListCoupons(ctx context.Context) ([]domain.Coupon, error) {
	return s.coupons.List(ctx)
}

func (s *Service) GetCouponByCode(ctx context.Context, code string) (*domain.Coupon, error) {
	return s.coupons.GetByCode(ctx, strings.ToUpper(strings.TrimSpace(code)))
}

func (s *Service) CreateCoupon(ctx context.Context, coupon *domain.Coupon) error {
	if coupon == nil {
		return errors.New("coupon is required")
	}
	if err := normalizeCoupon(coupon); err != nil {
		return err
	}
	return s.coupons.Create(ctx, coupon)
}

func (s *Service) UpdateCoupon(ctx context.Context, id uint, input *domain.Coupon) (*domain.Coupon, error) {
	if input == nil {
		return nil, errors.New("coupon is required")
	}
	coupon, err := s.coupons.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	createdAt := coupon.CreatedAt
	deletedAt := coupon.DeletedAt
	usedCount := coupon.UsedCount
	*coupon = *input
	coupon.ID = id
	coupon.CreatedAt = createdAt
	coupon.DeletedAt = deletedAt
	coupon.UsedCount = usedCount
	if err := normalizeCoupon(coupon); err != nil {
		return nil, err
	}
	if err := s.coupons.Update(ctx, coupon); err != nil {
		return nil, err
	}
	return coupon, nil
}

func (s *Service) DeleteCoupon(ctx context.Context, id uint) error {
	return s.coupons.Delete(ctx, id)
}

func (s *Service) ensureCardProduct(ctx context.Context, productID uint) error {
	product, err := s.products.GetByID(ctx, productID)
	if err != nil {
		return err
	}
	if product.ProductType != domain.ProductTypeCard {
		return ErrManualProductCard
	}
	return nil
}

func normalizeProduct(product *domain.Product) error {
	product.Name = strings.TrimSpace(product.Name)
	product.Slug = strings.TrimSpace(product.Slug)
	product.ProductType = strings.ToLower(strings.TrimSpace(product.ProductType))
	if product.Name == "" || product.Slug == "" {
		return errors.New("product name and slug are required")
	}
	if product.Price < 0 {
		return errors.New("product price cannot be negative")
	}
	if product.ProductType == "" {
		product.ProductType = domain.ProductTypeCard
	}
	if product.ProductType != domain.ProductTypeCard && product.ProductType != domain.ProductTypeManual {
		return ErrInvalidProductType
	}
	product.AutoDeliver = product.ProductType == domain.ProductTypeCard
	return nil
}

func normalizeCoupon(coupon *domain.Coupon) error {
	coupon.Code = strings.ToUpper(strings.TrimSpace(coupon.Code))
	coupon.DiscountType = strings.ToLower(strings.TrimSpace(coupon.DiscountType))
	if coupon.Code == "" {
		return errors.New("coupon code is required")
	}
	if coupon.DiscountType != "fixed" && coupon.DiscountType != "percent" {
		return errors.New("discount type must be fixed or percent")
	}
	if coupon.DiscountValue <= 0 {
		return errors.New("discount value must be positive")
	}
	if coupon.DiscountType == "percent" && coupon.DiscountValue > 100 {
		return errors.New("percentage discount cannot exceed 100")
	}
	if coupon.MinOrderAmount < 0 || coupon.MaxUses < 0 {
		return errors.New("coupon limits cannot be negative")
	}
	if coupon.ValidFrom != nil && coupon.ValidUntil != nil && coupon.ValidUntil.Before(*coupon.ValidFrom) {
		return errors.New("valid_until must be after valid_from")
	}
	return nil
}

func validCardStatus(status string) bool {
	return status == domain.CardStatusAvailable || status == domain.CardStatusSold || status == domain.CardStatusDisabled
}
