package application

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/config"
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
	features   config.FeaturesConfig
	now        func() time.Time
}

func NewService(products contract.ProductRepo, cards contract.CardRepo, categories contract.CategoryRepo, coupons contract.CouponRepo, features config.FeaturesConfig) *Service {
	return &Service{
		products:   products,
		cards:      cards,
		categories: categories,
		coupons:    coupons,
		features:   features,
		now:        time.Now,
	}
}

// CouponsEnabled reports whether codes may be spent at all, so the storefront
// can hide the field instead of rejecting every input the buyer types.
func (s *Service) CouponsEnabled() bool { return s.features.CouponsOn() }

// AlertThreshold is the stock level below which a card product counts as
// needing restocking, so the back office and the low-stock list agree.
func (s *Service) AlertThreshold() int { return s.features.AlertThreshold() }

// PublicProducts is the storefront listing: filtered, ranked and paged in the
// database, with the total that the page belongs to.
func (s *Service) PublicProducts(ctx context.Context, query domain.ProductQuery) ([]domain.Product, int64, error) {
	query.PublishedOnly = true
	return s.filteredProducts(ctx, query)
}

// AdminProducts is the same read without the publishing gate.
func (s *Service) AdminProducts(ctx context.Context, query domain.ProductQuery) ([]domain.Product, int64, error) {
	query.PublishedOnly = false
	return s.filteredProducts(ctx, query)
}

func (s *Service) filteredProducts(ctx context.Context, query domain.ProductQuery) ([]domain.Product, int64, error) {
	if !domain.ValidSort(query.Sort) {
		return nil, 0, fmt.Errorf("%w: 不支持这种排序方式（%s），请从列表页重新筛选。", ErrInvalidQuery, briefValue(query.Sort))
	}
	if query.Limit <= 0 {
		query.Limit = 24
	}
	return s.products.ListQuery(ctx, query)
}

// PublicCategories returns each visible category with how many products a buyer
// can actually reach under it, which is what the home page chips are labelled
// with. An empty category stays listed — hiding it would make the shop owner's
// taxonomy disappear without explanation.
func (s *Service) PublicCategories(ctx context.Context) ([]domain.CategoryView, error) {
	categories, err := s.categories.List(ctx, true)
	if err != nil {
		return nil, err
	}
	counts, err := s.products.CountByCategory(ctx, true)
	if err != nil {
		return nil, err
	}
	views := make([]domain.CategoryView, 0, len(categories))
	for _, category := range categories {
		views = append(views, domain.CategoryView{Category: category, ProductCount: counts[category.ID]})
	}
	return views, nil
}

func (s *Service) StoreStats(ctx context.Context) (domain.StoreStats, error) {
	return s.products.Stats(ctx, true)
}

// LowStockProducts is the restocking queue at the shop's own threshold.
func (s *Service) LowStockProducts(ctx context.Context) ([]domain.Product, error) {
	return s.products.ListLowStock(ctx, s.features.AlertThreshold())
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
	soldCount := product.SoldCount
	createdAt := product.CreatedAt
	deletedAt := product.DeletedAt
	*product = *input
	product.ID = id
	product.CreatedAt = createdAt
	product.DeletedAt = deletedAt
	product.StockCount = stockCount
	// 销量 is a fact about orders, not a form field: the edit screen never sends
	// it, and a blank input must not wipe the number the storefront shows.
	product.SoldCount = soldCount
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
		return fmt.Errorf("%w: 卡密内容不能为空。", domain.ErrInvalidInput)
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

func (s *Service) UpdateCard(ctx context.Context, productID, cardID uint, input *domain.Card) (*domain.Card, error) {
	if input == nil {
		return nil, errors.New("card is required")
	}
	card, err := s.cards.GetByID(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if card.ProductID != productID {
		return nil, fmt.Errorf("%w: 这张卡密并不属于地址里的商品。", domain.ErrInvalidInput)
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, fmt.Errorf("%w: 卡密内容不能为空。", domain.ErrInvalidInput)
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
		return fmt.Errorf("%w: 这张卡密并不属于地址里的商品。", domain.ErrInvalidInput)
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
		return fmt.Errorf("%w: 分类名称和 slug 都要填写。", domain.ErrInvalidInput)
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
		return nil, fmt.Errorf("%w: 分类名称和 slug 都要填写。", domain.ErrInvalidInput)
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

// StorefrontCoupon is one promotion on the shop's own shelf: the code, what it
// takes off, and what a buyer needs to decide whether it is worth using. Nothing
// about other accounts' usage is in here, and the quota is counted from live
// orders, so a code that just ran out of uses leaves the shelf rather than
// failing the buyer at checkout.
type StorefrontCoupon struct {
	Code           string     `json:"code"`
	DiscountType   string     `json:"discount_type"`
	DiscountValue  int        `json:"discount_value"`
	MinOrderAmount int        `json:"min_order_amount"`
	Scope          string     `json:"scope"`
	ProductID      *uint      `json:"product_id,omitempty"`
	CategoryID     *uint      `json:"category_id,omitempty"`
	PerUserLimit   int        `json:"per_user_limit"`
	Description    string     `json:"description,omitempty"`
	ValidUntil     *time.Time `json:"valid_until,omitempty"`
	// Remaining is the number of uses left, absent when the code is unlimited.
	Remaining *int `json:"remaining,omitempty"`
}

// StorefrontCoupons lists the advertised promotions that a buyer could actually
// use right now. The window is judged with the same comparisons couponApplies
// makes at checkout, so the shelf never shows a code the quote box then refuses.
func (s *Service) StorefrontCoupons(ctx context.Context) ([]StorefrontCoupon, error) {
	if !s.features.CouponsOn() {
		return nil, nil
	}
	coupons, err := s.coupons.ListAdvertised(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	list := make([]StorefrontCoupon, 0, len(coupons))
	for index := range coupons {
		coupon := coupons[index]
		if coupon.ValidFrom != nil && now.Before(*coupon.ValidFrom) {
			continue
		}
		if coupon.ValidUntil != nil && now.After(*coupon.ValidUntil) {
			continue
		}
		entry := StorefrontCoupon{
			Code:           coupon.Code,
			DiscountType:   coupon.DiscountType,
			DiscountValue:  coupon.DiscountValue,
			MinOrderAmount: coupon.MinOrderAmount,
			Scope:          coupon.Scope,
			ProductID:      coupon.ProductID,
			CategoryID:     coupon.CategoryID,
			PerUserLimit:   coupon.PerUserLimit,
			ValidUntil:     coupon.ValidUntil,
		}
		if coupon.Description != nil {
			entry.Description = *coupon.Description
		}
		if coupon.MaxUses > 0 {
			used, err := s.coupons.UsedTotal(ctx, coupon.ID)
			if err != nil {
				return nil, err
			}
			left := coupon.MaxUses - int(used)
			if left <= 0 {
				continue
			}
			entry.Remaining = &left
		}
		list = append(list, entry)
	}
	// A deadline is the part of a promotion a buyer acts on, so the codes about
	// to end read first and the standing ones sink to the bottom.
	sort.SliceStable(list, func(i, j int) bool {
		first, second := list[i].ValidUntil, list[j].ValidUntil
		switch {
		case first == nil:
			return false
		case second == nil:
			return true
		default:
			return first.Before(*second)
		}
	})
	return list, nil
}

// CouponQuote is the answer the storefront shows under the 优惠码 field. It
// carries the reason a code was refused in the shop's own words, because the
// buyer needs to know whether to change the code, add quantity, or give up.
type CouponQuote struct {
	Code          string `json:"code"`
	Accepted      bool   `json:"accepted"`
	Discount      int    `json:"discount"`
	Payable       int    `json:"payable"`
	OriginalTotal int    `json:"original_total"`
	Description   string `json:"description,omitempty"`

	// couponID is kept off the wire: the buyer sees the code, the payment
	// module needs the row it points at.
	couponID uint
}

// DiscountFor is the checkout-side contract: given what the buyer is about to
// order, how much does this code take off? The payment module asks this before
// any money moves, and the discounted total is what NodeLoc is told to collect.
func (s *Service) DiscountFor(ctx context.Context, userID uint, productID uint, quantity, unitPrice int, code string) (discount int, couponID uint, err error) {
	quote, err := s.QuoteCoupon(ctx, userID, productID, quantity, unitPrice, code)
	if err != nil {
		return 0, 0, err
	}
	return quote.Discount, quote.couponID, nil
}

// QuoteCoupon evaluates a code against a would-be order.
func (s *Service) QuoteCoupon(ctx context.Context, userID uint, productID uint, quantity, unitPrice int, code string) (*CouponQuote, error) {
	if !s.features.CouponsOn() {
		return nil, ErrCouponDisabled
	}
	normalized := strings.ToUpper(strings.TrimSpace(code))
	if normalized == "" {
		return nil, ErrCouponNotFound
	}
	if quantity <= 0 {
		quantity = 1
	}
	coupon, err := s.coupons.GetByCode(ctx, normalized)
	if err != nil {
		return nil, ErrCouponNotFound
	}
	product, err := s.products.GetByID(ctx, productID)
	if err != nil {
		return nil, err
	}
	total := unitPrice * quantity
	if err := couponApplies(coupon, product, total, s.now()); err != nil {
		return nil, err
	}
	if coupon.MaxUses > 0 {
		// Live orders, not coupon.UsedCount: that column only moves when NodeLoc
		// confirms the payment, so the last code of a 限量 promotion would be sold
		// to everyone who asked before the first buyer paid.
		committed, err := s.coupons.UsedTotal(ctx, coupon.ID)
		if err != nil {
			return nil, err
		}
		if committed >= int64(coupon.MaxUses) {
			return nil, ErrCouponExhausted
		}
	}
	if coupon.PerUserLimit > 0 && userID != 0 {
		used, err := s.coupons.UsedBy(ctx, coupon.ID, userID)
		if err != nil {
			return nil, err
		}
		if used >= int64(coupon.PerUserLimit) {
			return nil, ErrCouponPerUser
		}
	}

	discount := couponDiscount(coupon, total)
	quote := &CouponQuote{
		Code:          coupon.Code,
		Accepted:      true,
		Discount:      discount,
		Payable:       total - discount,
		OriginalTotal: total,
	}
	if coupon.Description != nil {
		quote.Description = *coupon.Description
	}
	quote.couponID = coupon.ID
	return quote, nil
}

// couponApplies runs the rules that make a code spendable at all: the window,
// the shop's minimum, and the product scope it was written for.
func couponApplies(coupon *domain.Coupon, product *domain.Product, total int, now time.Time) error {
	if !coupon.IsActive {
		return ErrCouponInactive
	}
	if coupon.ValidFrom != nil && now.Before(*coupon.ValidFrom) {
		return ErrCouponNotStarted
	}
	if coupon.ValidUntil != nil && now.After(*coupon.ValidUntil) {
		return ErrCouponExpired
	}
	switch coupon.Scope {
	case "product":
		if coupon.ProductID == nil || product.ID != *coupon.ProductID {
			return ErrCouponNotApplicable
		}
	case "category":
		if coupon.CategoryID == nil || product.CategoryID == nil || *product.CategoryID != *coupon.CategoryID {
			return ErrCouponNotApplicable
		}
	}
	if coupon.MinOrderAmount > 0 && total < coupon.MinOrderAmount {
		return ErrCouponMinAmount
	}
	return nil
}

// couponDiscount never takes the whole order: NodeLoc is asked to collect a
// payment, and a 0 amount order is not a payment. A 100% code therefore leaves
// one fen behind rather than being rejected outright.
func couponDiscount(coupon *domain.Coupon, total int) int {
	if total <= 1 {
		return 0
	}
	discount := 0
	switch coupon.DiscountType {
	case "percent":
		discount = total * coupon.DiscountValue / 100
	case "fixed":
		discount = coupon.DiscountValue
	}
	if discount < 0 {
		return 0
	}
	if limit := total - 1; discount > limit {
		return limit
	}
	return discount
}

// ListCardsFiltered backs the back office's inventory screen: any product or a
// selected one, by status, with a content search and server-side paging.
func (s *Service) ListCardsFiltered(ctx context.Context, filter domain.CardFilter) ([]domain.Card, int64, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	return s.cards.ListFiltered(ctx, filter)
}

// maxExportCards bounds a download: a shop with a hundred thousand keys should
// filter before exporting rather than pull the whole table into memory.
const maxExportCards = 20000

// ExportCards walks the filtered inventory page by page for the CSV download.
// The second return says the selection was cut short, so a truncated file
// never reads like a complete one.
func (s *Service) ExportCards(ctx context.Context, filter domain.CardFilter) ([]domain.Card, bool, error) {
	const pageSize = 500
	filter.Limit = pageSize
	filter.Offset = 0
	var (
		cards     []domain.Card
		truncated bool
	)
	for {
		batch, _, err := s.cards.ListFiltered(ctx, filter)
		if err != nil {
			return nil, false, err
		}
		cards = append(cards, batch...)
		if len(batch) < pageSize {
			return cards, truncated, nil
		}
		if len(cards)+pageSize > maxExportCards {
			return cards, true, nil
		}
		filter.Offset += pageSize
	}
}

// ImportResult reports what a bulk paste did, because the shop owner needs to
// know that 40 of the 120 lines they pasted were already in stock.
type ImportResult struct {
	Created []domain.Card `json:"created"`
	Skipped int           `json:"skipped"`
	Blank   int           `json:"blank"`
}

// ImportCards adds lines to a product, dropping blanks and duplicates. Card
// keys are only worth their scarcity, so a repeated paste must not create a
// second copy of a key that was already sold.
func (s *Service) ImportCards(ctx context.Context, productID uint, contents []string) (*ImportResult, error) {
	if err := s.ensureCardProduct(ctx, productID); err != nil {
		return nil, err
	}
	existing, err := s.cards.ExistingContents(ctx, productID)
	if err != nil {
		return nil, err
	}
	result := &ImportResult{Created: make([]domain.Card, 0, len(contents))}
	seen := make(map[string]struct{}, len(contents))
	for _, content := range contents {
		content = strings.TrimSpace(content)
		switch {
		case content == "":
			result.Blank++
			continue
		}
		if _, dup := existing[content]; dup {
			result.Skipped++
			continue
		}
		if _, dup := seen[content]; dup {
			result.Skipped++
			continue
		}
		seen[content] = struct{}{}
		result.Created = append(result.Created, domain.Card{
			ProductID: productID,
			Content:   content,
			Status:    domain.CardStatusAvailable,
		})
	}
	if len(result.Created) == 0 {
		if result.Blank > 0 || result.Skipped > 0 {
			return result, nil
		}
		return nil, fmt.Errorf("%w: 至少要填写一条非空的卡密。", domain.ErrInvalidInput)
	}
	if err := s.cards.CreateBatch(ctx, result.Created); err != nil {
		return nil, err
	}
	if err := s.SyncStock(ctx, productID); err != nil {
		return nil, err
	}
	return result, nil
}

// cardAlphabet leaves out the characters a buyer transcribes wrong: no I/O, no
// 0/1. Codes are read off a screen and typed by hand.
const cardAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// GenerateCards produces a batch of keys for a product. Shop owners used to
// paste these from an external generator; generating them here keeps the format
// consistent and makes "I have 200 keys left" a statement the shop can verify.
func (s *Service) GenerateCards(ctx context.Context, productID uint, count int, prefix string) (*ImportResult, error) {
	if err := s.ensureCardProduct(ctx, productID); err != nil {
		return nil, err
	}
	if count <= 0 || count > 500 {
		return nil, fmt.Errorf("%w: 一次生成的卡密数量要在 1 到 500 之间。", ErrInvalidQuery)
	}
	prefix = strings.ToUpper(strings.TrimSpace(prefix))
	if len(prefix) > 16 {
		return nil, fmt.Errorf("%w: 前缀最长 16 个字符。", ErrInvalidQuery)
	}
	for _, r := range prefix {
		if !strings.ContainsRune(cardAlphabet+"-", r) {
			return nil, fmt.Errorf("%w: 前缀只能使用字母、数字和短横线。", ErrInvalidQuery)
		}
	}

	existing, err := s.cards.ExistingContents(ctx, productID)
	if err != nil {
		return nil, err
	}
	cards := make([]domain.Card, 0, count)
	issued := make(map[string]struct{}, count)
	for attempt := 0; len(cards) < count && attempt < count*20; attempt++ {
		body, err := randomCardBody(16)
		if err != nil {
			return nil, err
		}
		content := prefix + body
		if _, taken := existing[content]; taken {
			continue
		}
		if _, dup := issued[content]; dup {
			continue
		}
		issued[content] = struct{}{}
		cards = append(cards, domain.Card{ProductID: productID, Content: content, Status: domain.CardStatusAvailable})
	}
	if len(cards) == 0 {
		return nil, errors.New("could not generate unique card keys")
	}
	if err := s.cards.CreateBatch(ctx, cards); err != nil {
		return nil, err
	}
	if err := s.SyncStock(ctx, productID); err != nil {
		return nil, err
	}
	return &ImportResult{Created: cards}, nil
}

func randomCardBody(length int) (string, error) {
	out := make([]byte, length)
	limit := big.NewInt(int64(len(cardAlphabet)))
	for i := range out {
		index, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", fmt.Errorf("generate card key: %w", err)
		}
		out[i] = cardAlphabet[index.Int64()]
	}
	return string(out), nil
}

// SetCardStatus enables or disables selected keys and reports how many moved.
func (s *Service) SetCardStatus(ctx context.Context, productID uint, ids []uint, status string) (int64, error) {
	if len(ids) == 0 {
		return 0, fmt.Errorf("%w: 请先勾选要操作的卡密。", ErrInvalidQuery)
	}
	if status != domain.CardStatusAvailable && status != domain.CardStatusDisabled {
		return 0, ErrInvalidCardStatus
	}
	if err := s.ensureCardProduct(ctx, productID); err != nil {
		return 0, err
	}
	moved, err := s.cards.BulkStatus(ctx, productID, ids, status)
	if err != nil {
		return 0, err
	}
	if err := s.SyncStock(ctx, productID); err != nil {
		return 0, err
	}
	return moved, nil
}

// DeleteCards removes selected keys, and never a key a buyer already paid for:
// that card is part of their order record.
func (s *Service) DeleteCards(ctx context.Context, productID uint, ids []uint) (int64, error) {
	if len(ids) == 0 {
		return 0, fmt.Errorf("%w: 请先勾选要操作的卡密。", ErrInvalidQuery)
	}
	if err := s.ensureCardProduct(ctx, productID); err != nil {
		return 0, err
	}
	deleted, err := s.cards.BulkDelete(ctx, productID, ids)
	if err != nil {
		return 0, err
	}
	if err := s.SyncStock(ctx, productID); err != nil {
		return 0, err
	}
	return deleted, nil
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
		return fmt.Errorf("%w: 商品名称和 slug 都要填写。", domain.ErrInvalidInput)
	}
	if product.Price < 0 {
		return fmt.Errorf("%w: 商品价格不能是负数。", domain.ErrInvalidInput)
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
		return fmt.Errorf("%w: 优惠码本身不能为空。", domain.ErrInvalidInput)
	}
	if coupon.DiscountType != "fixed" && coupon.DiscountType != "percent" {
		return fmt.Errorf("%w: 抵扣方式只能是 fixed（立减）或 percent（折扣）。", domain.ErrInvalidInput)
	}
	if coupon.DiscountValue <= 0 {
		return fmt.Errorf("%w: 抵扣数额要大于 0。", domain.ErrInvalidInput)
	}
	if coupon.DiscountType == "percent" && coupon.DiscountValue > 100 {
		return fmt.Errorf("%w: 折扣比例不能超过 100%%。", domain.ErrInvalidInput)
	}
	if coupon.MinOrderAmount < 0 || coupon.MaxUses < 0 {
		return fmt.Errorf("%w: 门槛金额和限量次数不能是负数。", domain.ErrInvalidInput)
	}
	if coupon.ValidFrom != nil && coupon.ValidUntil != nil && coupon.ValidUntil.Before(*coupon.ValidFrom) {
		return fmt.Errorf("%w: 失效时间要晚于生效时间。", domain.ErrInvalidInput)
	}
	// Scope decides what the code may buy. Anything unrecognized is treated as
	// the whole shop rather than silently matching nothing.
	coupon.Scope = strings.ToLower(strings.TrimSpace(coupon.Scope))
	switch coupon.Scope {
	case "", "all":
		coupon.Scope = "all"
		coupon.ProductID = nil
		coupon.CategoryID = nil
	case "product":
		coupon.CategoryID = nil
		if coupon.ProductID == nil || *coupon.ProductID == 0 {
			return fmt.Errorf("%w: 指定商品可用的优惠码要先选好商品。", domain.ErrInvalidInput)
		}
	case "category":
		coupon.ProductID = nil
		if coupon.CategoryID == nil || *coupon.CategoryID == 0 {
			return fmt.Errorf("%w: 指定分类可用的优惠码要先选好分类。", domain.ErrInvalidInput)
		}
	default:
		return fmt.Errorf("%w: 适用范围只能是 all、product 或 category。", domain.ErrInvalidInput)
	}
	if coupon.PerUserLimit < 0 {
		return fmt.Errorf("%w: 每人限用次数不能是负数。", domain.ErrInvalidInput)
	}
	if coupon.Description != nil {
		description := strings.TrimSpace(*coupon.Description)
		if len([]rune(description)) > 255 {
			return fmt.Errorf("%w: 优惠码说明最长 255 个字。", domain.ErrInvalidInput)
		}
		if description == "" {
			coupon.Description = nil
		} else {
			coupon.Description = &description
		}
	}
	return nil
}

func validCardStatus(status string) bool {
	return status == domain.CardStatusAvailable || status == domain.CardStatusSold || status == domain.CardStatusDisabled
}

// briefValue shortens a rejected query value before it is echoed back in the
// error copy, so a paste box full of text cannot become a page of error message.
func briefValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "空值"
	}
	if runes := []rune(value); len(runes) > 24 {
		return string(runes[:24]) + "…"
	}
	return value
}
