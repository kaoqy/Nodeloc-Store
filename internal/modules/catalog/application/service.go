package application

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
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
	// wake belongs to the money side: it delivers the orders that paid for a
	// product whose stock has just grown. Nil until the container has wired both
	// modules, and a catalogue without it still stocks shelves — those orders go
	// out on the next background sweep instead of on the spot.
	wake deliveryWake
}

// deliveryWake is the payment module's answer to "keys just landed on this
// shelf". The interface is declared here rather than imported because the
// architecture rules let a module reach another only through a contract, and
// this need belongs to the restock button, not to payment.
type deliveryWake interface {
	ReleaseProductBacklog(ctx context.Context, productID uint) (int, error)
	WaitingOrdersByProduct(ctx context.Context) (map[uint]int64, error)
}

// SetDeliveryWake attaches the payment module so a restock can hand over the
// orders that were waiting for the keys it just added. The container calls it
// after wiring payment, which itself is wired against this catalogue for 优惠码
// pricing.
func (s *Service) SetDeliveryWake(wake deliveryWake) { s.wake = wake }

// releaseBacklog is the last act of a restock, and returns how many waiting
// orders actually left. A wake that fails must not turn a finished import into
// an error: the keys are in stock, the buyer's order is intact, and the sweep
// delivers it minutes later.
func (s *Service) releaseBacklog(ctx context.Context, productID uint) int {
	if s.wake == nil {
		return 0
	}
	released, err := s.wake.ReleaseProductBacklog(ctx, productID)
	if err != nil {
		log.Printf("catalog: restock product %d: waiting orders could not be delivered yet: %v", productID, err)
		return 0
	}
	return released
}

// waitingOrders is the restocking queue's "已付款在等" column, and is empty when
// the catalogue is wired without the payment side.
func (s *Service) waitingOrders(ctx context.Context) map[uint]int64 {
	if s.wake == nil {
		return map[uint]int64{}
	}
	counts, err := s.wake.WaitingOrdersByProduct(ctx)
	if err != nil {
		log.Printf("catalog: waiting order counts: %v", err)
		return map[uint]int64{}
	}
	return counts
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

// LowStockProduct is one row of the restocking queue: the product the shop must
// refill and how many buyers have already paid for it and are waiting.
type LowStockProduct struct {
	domain.Product
	WaitingOrders int64 `json:"waiting_orders"`
}

// LowStockProducts is the restocking queue at the shop's own threshold.
func (s *Service) LowStockProducts(ctx context.Context) ([]LowStockProduct, error) {
	products, err := s.products.ListLowStock(ctx, s.features.AlertThreshold())
	if err != nil {
		return nil, err
	}
	waiting := s.waitingOrders(ctx)
	rows := make([]LowStockProduct, 0, len(products))
	for _, product := range products {
		rows = append(rows, LowStockProduct{Product: product, WaitingOrders: waiting[product.ID]})
	}
	// The queue is already ordered by how empty the shelf is; buyers who paid move
	// it to the front, so restocking starts where money is waiting.
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].WaitingOrders > rows[j].WaitingOrders })
	return rows, nil
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

// UpdateProduct applies a patch: fields left zero are treated as "unchanged"
// unless the caller explicitly asks for them.
//
// 两个入口共用这一个方法：商品编辑页提交完整表单，列表页的上下架/推荐只
// 发一个布尔值。早期版本把请求体整对象覆盖到记录上，于是列表页点一下
// 「下架」就会把未提交的字段清零——那正是这套 _Set 标记要解决的问题。
func (s *Service) UpdateProduct(ctx context.Context, id uint, input *domain.Product) (*domain.Product, error) {
	return s.UpdateProductPatch(ctx, id, input, nil)
}

// UpdateProductPatch 允许调用方指明这次请求真正携带了哪些字段。
// changed 为 nil 时按完整表单处理（编辑页），否则只覆盖列出的字段（列表页）。
func (s *Service) UpdateProductPatch(ctx context.Context, id uint, input *domain.Product, changed map[string]bool) (*domain.Product, error) {
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
	if changed == nil {
		*product = *input
	} else {
		// 补丁模式：只把请求里明确出现的字段写回记录。
		if changed["name"] {
			product.Name = input.Name
		}
		if changed["slug"] {
			product.Slug = input.Slug
		}
		if changed["summary"] {
			product.Summary = input.Summary
		}
		if changed["description"] {
			product.Description = input.Description
		}
		if changed["image_path"] {
			product.ImagePath = input.ImagePath
		}
		if changed["product_type"] {
			product.ProductType = input.ProductType
		}
		if changed["delivery_instructions"] {
			product.DeliveryInstructions = input.DeliveryInstructions
		}
		if changed["require_contact"] {
			product.RequireContact = input.RequireContact
		}
		if changed["price"] {
			product.Price = input.Price
		}
		if changed["original_price"] {
			product.OriginalPrice = input.OriginalPrice
		}
		if changed["stock_visible"] {
			product.StockVisible = input.StockVisible
		}
		if changed["is_featured"] {
			product.IsFeatured = input.IsFeatured
		}
		if changed["auto_deliver"] {
			product.AutoDeliver = input.AutoDeliver
		}
		if changed["is_published"] {
			product.IsPublished = input.IsPublished
		}
		if changed["is_archived"] {
			product.IsArchived = input.IsArchived
		}
		if changed["sort_order"] {
			product.SortOrder = input.SortOrder
		}
		if changed["category_id"] {
			product.CategoryID = input.CategoryID
		}
		if changed["form_schema"] {
			product.FormSchema = input.FormSchema
		}
	}
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

// AddCard stocks one key and returns how many of the orders that were waiting
// for this product the restock delivered.
func (s *Service) AddCard(ctx context.Context, productID uint, card *domain.Card) (int, error) {
	if card == nil {
		return 0, errors.New("card is required")
	}
	if err := s.ensureCardProduct(ctx, productID); err != nil {
		return 0, err
	}
	card.ProductID = productID
	card.Content = strings.TrimSpace(card.Content)
	if card.Content == "" {
		return 0, fmt.Errorf("%w: 卡密内容不能为空。", domain.ErrInvalidInput)
	}
	if card.Status == "" {
		card.Status = domain.CardStatusAvailable
	}
	if !validCardStatus(card.Status) {
		return 0, ErrInvalidCardStatus
	}
	if err := s.cards.Create(ctx, card); err != nil {
		return 0, err
	}
	if err := s.SyncStock(ctx, productID); err != nil {
		return 0, err
	}
	if card.Status != domain.CardStatusAvailable {
		return 0, nil
	}
	return s.releaseBacklog(ctx, productID), nil
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
	if status == domain.CardStatusAvailable {
		s.releaseBacklog(ctx, productID)
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

// AdminCoupon is one row of the back office's coupon table: the stored code
// plus what the live orders say about its quota. A buyer is refused against the
// live count, not used_count, which only moves when NodeLoc settles a payment —
// without held/remaining the owner reads 已使用 0 次 while the codes are gone.
type AdminCoupon struct {
	domain.Coupon
	Held      int64 `json:"held"`
	Remaining *int  `json:"remaining"`
}

func (s *Service) ListCoupons(ctx context.Context) ([]AdminCoupon, error) {
	coupons, err := s.coupons.List(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	held, err := s.coupons.HeldByCoupons(ctx, now.Add(-checkoutHoldWindow))
	if err != nil {
		return nil, err
	}
	rows := make([]AdminCoupon, 0, len(coupons))
	for _, coupon := range coupons {
		row := AdminCoupon{Coupon: coupon}
		if coupon.MaxUses > 0 {
			row.Held = held[coupon.ID]
			left := coupon.MaxUses - int(row.Held)
			if left < 0 {
				left = 0
			}
			row.Remaining = &left
		}
		rows = append(rows, row)
	}
	return rows, nil
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
			used, err := s.coupons.UsedTotal(ctx, coupon.ID, now.Add(-checkoutHoldWindow))
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

// checkoutHoldWindow is how long an order a buyer never paid keeps a code's
// quota. A checkout left unpaid is not a sale, and without a cutoff every
// 立即购买 pressed by mistake would burn 限量/每人限用 for good — the code reads
// 已达使用上限 to the next buyer while the shop's 已使用 counter still says 0.
const checkoutHoldWindow = 2 * time.Hour

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
	// Note is the small print the preview alone cannot decide: a guest's 每人限用
	// could not be checked because there was no account to check, so the shop
	// says the rule will be applied at checkout instead of implying it passed.
	Note string `json:"note,omitempty"`

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
	// A guest may only preview a code the shop put on its own promo shelf. A
	// privately-sent code answers the same way a typo does, so the public route
	// cannot be used to learn which codes exist.
	if userID == 0 && !coupon.Advertised {
		return nil, ErrCouponNotFound
	}
	product, err := s.products.GetByID(ctx, productID)
	if err != nil {
		return nil, err
	}
	total := unitPrice * quantity
	now := s.now()
	if err := couponApplies(coupon, product, total, now); err != nil {
		return nil, err
	}
	if coupon.MaxUses > 0 {
		// Live orders, not coupon.UsedCount: that column only moves when NodeLoc
		// confirms the payment, so the last code of a 限量 promotion would be sold
		// to everyone who asked before the first buyer paid.
		committed, err := s.coupons.UsedTotal(ctx, coupon.ID, now.Add(-checkoutHoldWindow))
		if err != nil {
			return nil, err
		}
		if committed >= int64(coupon.MaxUses) {
			return nil, ErrCouponExhausted
		}
	}
	if coupon.PerUserLimit > 0 && userID != 0 {
		used, err := s.coupons.UsedBy(ctx, coupon.ID, userID, now.Add(-checkoutHoldWindow))
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
	if userID == 0 && coupon.PerUserLimit > 0 {
		quote.Note = fmt.Sprintf("本码每人限用 %d 次。现在还没有登录账号，下单时会再核对一次。", coupon.PerUserLimit)
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
// payment, and a 0 amount order is not a payment. A 100% code therefore leaves 1
// 元 behind rather than being rejected outright, and an order of 1 元 or less
// gets no discount at all.
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
// know that 40 of the 120 lines they pasted were already in stock — and that all
// 120 went in anyway.
type ImportResult struct {
	Created []domain.Card `json:"created"`
	// Duplicates counts the lines that repeat a key already on this shelf. They
	// are stocked, not dropped: a shop selling the same 激活码 to everyone has
	// one line per unit sold, and refusing the repeat loses the sale, not the
	// duplicate.
	Duplicates int `json:"duplicates"`
	Blank      int `json:"blank"`
	// Released counts the paid orders this restock was able to deliver on the
	// spot, so the back office can say the goods went out rather than just that
	// the keys arrived.
	Released int `json:"released"`
}

// ImportCards adds every non-blank line to a product. Repeats are counted and
// kept, so pasting the same file twice stocks twice.
func (s *Service) ImportCards(ctx context.Context, productID uint, contents []string) (*ImportResult, error) {
	if err := s.ensureCardProduct(ctx, productID); err != nil {
		return nil, err
	}
	existing, err := s.cards.ExistingContents(ctx, productID)
	if err != nil {
		return nil, err
	}
	result := &ImportResult{Created: make([]domain.Card, 0, len(contents))}
	seen := make(map[string]int, len(contents))
	for _, content := range contents {
		content = strings.TrimSpace(content)
		if content == "" {
			result.Blank++
			continue
		}
		if _, taken := existing[content]; taken || seen[content] > 0 {
			result.Duplicates++
		}
		seen[content]++
		result.Created = append(result.Created, domain.Card{
			ProductID: productID,
			Content:   content,
			Status:    domain.CardStatusAvailable,
		})
	}
	if len(result.Created) == 0 {
		if result.Blank > 0 {
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
	result.Released = s.releaseBacklog(ctx, productID)
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
	return &ImportResult{Created: cards, Released: s.releaseBacklog(ctx, productID)}, nil
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

// SetCardStatus enables or disables selected keys, reports how many moved, and
// for a batch that just went back on the shelf, how many waiting orders that
// released.
func (s *Service) SetCardStatus(ctx context.Context, productID uint, ids []uint, status string) (int64, int, error) {
	if len(ids) == 0 {
		return 0, 0, fmt.Errorf("%w: 请先勾选要操作的卡密。", ErrInvalidQuery)
	}
	if status != domain.CardStatusAvailable && status != domain.CardStatusDisabled {
		return 0, 0, ErrInvalidCardStatus
	}
	if err := s.ensureCardProduct(ctx, productID); err != nil {
		return 0, 0, err
	}
	moved, err := s.cards.BulkStatus(ctx, productID, ids, status)
	if err != nil {
		return 0, 0, err
	}
	if err := s.SyncStock(ctx, productID); err != nil {
		return 0, 0, err
	}
	if status != domain.CardStatusAvailable || moved == 0 {
		return moved, 0, nil
	}
	return moved, s.releaseBacklog(ctx, productID), nil
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
	product.FormSchema = strings.TrimSpace(product.FormSchema)
	if product.FormSchema != "" {
		var fields []models.ProductFormField
		if err := json.Unmarshal([]byte(product.FormSchema), &fields); err != nil {
			return fmt.Errorf("%w: 商品购买表单配置无效", domain.ErrInvalidInput)
		}
		// The schema is validated, not the answers: a required field is exactly
		// what the shop is allowed to declare here, and the buyer's side is what
		// enforces it at checkout.
		if _, err := models.ValidateProductFormValues(fields, map[string]string{}, false); err != nil {
			return fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
		}
		encoded, err := json.Marshal(fields)
		if err != nil {
			return fmt.Errorf("%w: 商品购买表单配置无效", domain.ErrInvalidInput)
		}
		product.FormSchema = string(encoded)
	}
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
