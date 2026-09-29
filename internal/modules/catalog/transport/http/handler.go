package http

import (
	"encoding/csv"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	middleware "github.com/kaoqy/Nodeloc-Store/internal/app/httpserver"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/catalog/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/catalog/domain"
)

type Handler struct {
	service *application.Service
}

func NewHandler(service *application.Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes installs the public store routes and permission-gated admin
// routes. Every admin route names the permission it needs, so a 运营 account can
// work the catalogue without ever reaching 设置 or 角色权限.
func (h *Handler) RegisterRoutes(router gin.IRouter, jwtConfig *config.JWTConfig, accounts middleware.AccountReader) {
	store := router.Group("/api/v1/store")
	store.GET("/products", h.listPublicProducts)
	store.GET("/products/:slug", h.getPublicProduct)
	store.GET("/categories", h.listPublicCategories)
	store.GET("/stats", h.storeStats)
	// The promo shelf is public on purpose: a shop that advertises a code wants
	// buyers to find it before they reach checkout.
	store.GET("/coupons", h.listStoreCoupons)

	// A code is quoted, not guessed: only an account the store already knows
	// may ask what a coupon would take off, and the answer never reveals
	// another buyer's usage.
	shop := router.Group("/api/v1/store", middleware.JWTMiddleware(jwtConfig))
	shop.POST("/coupons/quote", h.quoteCoupon)

	guard := func(resource, action string) gin.HandlerFunc {
		return middleware.RequirePermission(accounts, resource, action)
	}

	admin := router.Group("/api/v1/admin")
	admin.Use(middleware.JWTMiddleware(jwtConfig))

	admin.GET("/products", guard("products", "view"), h.listProducts)
	// A sibling of /products/:id would collide with the wildcard, so the
	// restocking queue sits at its own path rather than under /products.
	admin.GET("/low-stock", guard("products", "view"), h.listLowStock)
	admin.GET("/products/:id", guard("products", "view"), h.getProduct)
	admin.POST("/products", guard("products", "manage"), h.createProduct)
	admin.PUT("/products/:id", guard("products", "manage"), h.updateProduct)
	admin.DELETE("/products/:id", guard("products", "manage"), h.deleteProduct)

	admin.GET("/cards", guard("cards", "view"), h.listAllCards)
	admin.GET("/cards/export", guard("cards", "manage"), h.exportCards)
	admin.GET("/products/:id/cards", guard("cards", "view"), h.listCards)
	admin.POST("/products/:id/cards", guard("cards", "manage"), h.addCard)
	admin.POST("/products/:id/cards/generate", guard("cards", "manage"), h.generateCards)
	admin.POST("/products/:id/cards/batch-status", guard("cards", "manage"), h.batchCardStatus)
	admin.POST("/products/:id/cards/batch-delete", guard("cards", "manage"), h.batchDeleteCards)
	admin.PUT("/products/:id/cards/:card_id", guard("cards", "manage"), h.updateCard)
	admin.DELETE("/products/:id/cards/:card_id", guard("cards", "manage"), h.deleteCard)
	admin.POST("/products/:id/cards/batch-add", guard("cards", "manage"), h.batchAddCards)

	admin.GET("/categories", guard("categories", "view"), h.listCategories)
	admin.POST("/categories", guard("categories", "manage"), h.createCategory)
	admin.PUT("/categories/:id", guard("categories", "manage"), h.updateCategory)
	admin.DELETE("/categories/:id", guard("categories", "manage"), h.deleteCategory)

	admin.GET("/coupons", guard("coupons", "view"), h.listCoupons)
	admin.POST("/coupons", guard("coupons", "manage"), h.createCoupon)
	admin.PUT("/coupons/:id", guard("coupons", "manage"), h.updateCoupon)
	admin.DELETE("/coupons/:id", guard("coupons", "manage"), h.deleteCoupon)
}

// productQuery reads the storefront's listing parameters. Only the values a
// caller can actually name are parsed here; an unknown sort is refused further
// down rather than silently becoming the default ordering, because a filter chip
// that quietly did nothing is worse to debug than a 400 that says so.
func productQuery(c *gin.Context) domain.ProductQuery {
	query := domain.ProductQuery{
		Search: strings.TrimSpace(c.Query("q")),
		Sort:   strings.TrimSpace(c.Query("sort")),
	}
	if value, err := strconv.ParseUint(strings.TrimSpace(c.Query("category")), 10, 32); err == nil && value > 0 {
		id := uint(value)
		query.CategoryID = &id
	}
	query.FeaturedOnly = c.Query("featured") == "true"
	query.InStockOnly = c.Query("in_stock") == "true"
	query.Limit = positiveInt(c.Query("limit"), 24)
	query.Offset = positiveInt(c.Query("offset"), 0)
	return query
}

func positiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func (h *Handler) listPublicProducts(c *gin.Context) {
	products, total, err := h.service.PublicProducts(c.Request.Context(), productQuery(c))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":  products,
		"total": total,
		"limit": len(products),
	})
}

func (h *Handler) storeStats(c *gin.Context) {
	stats, err := h.service.StoreStats(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"stats": stats, "coupons_enabled": h.service.CouponsEnabled()})
}

// listStoreCoupons is the storefront's promo shelf: the codes the shop chose to
// advertise and a buyer could still use right now. An empty shelf answers with
// an empty list, because "the shop turned coupons off" and "nothing is running
// today" look identical from the buyer's side and both render as no chip.
func (h *Handler) listStoreCoupons(c *gin.Context) {
	coupons, err := h.service.StorefrontCoupons(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	if coupons == nil {
		coupons = []application.StorefrontCoupon{}
	}
	c.JSON(http.StatusOK, gin.H{"data": coupons, "enabled": h.service.CouponsEnabled()})
}

func (h *Handler) quoteCoupon(c *gin.Context) {
	var request struct {
		Code      string `json:"code"`
		ProductID uint   `json:"product_id"`
		Slug      string `json:"product_slug"`
		Quantity  int    `json:"quantity"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, errors.Join(application.ErrInvalidQuery, err))
		return
	}
	userID := contextUserID(c)
	productID := request.ProductID
	if productID == 0 && strings.TrimSpace(request.Slug) != "" {
		product, err := h.service.GetPublicProduct(c.Request.Context(), request.Slug)
		if err != nil {
			respondError(c, err)
			return
		}
		productID = product.ID
	}
	if productID == 0 {
		respondError(c, fmt.Errorf("%w: 请先告诉我要试算哪件商品。", application.ErrInvalidQuery))
		return
	}
	product, err := h.service.GetProduct(c.Request.Context(), productID)
	if err != nil {
		respondError(c, err)
		return
	}
	quantity := request.Quantity
	if quantity <= 0 {
		quantity = 1
	}
	quote, err := h.service.QuoteCoupon(c.Request.Context(), userID, productID, quantity, product.Price, request.Code)
	if err != nil {
		message := couponMessage(err)
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":   message,
			"code":    couponReason(err),
			"message": message,
		})
		return
	}
	c.JSON(http.StatusOK, quote)
}

// couponReason turns a rejection into the code the storefront switches on.
func couponReason(err error) string {
	var refusal *application.CouponError
	if errors.As(err, &refusal) {
		return refusal.CouponCode()
	}
	return "coupon_invalid"
}

// couponMessage is written for the person typing the code, not for the log.
func couponMessage(err error) string {
	var refusal *application.CouponError
	if errors.As(err, &refusal) {
		return refusal.CouponMessage()
	}
	return "优惠码无法使用。"
}

func (h *Handler) getPublicProduct(c *gin.Context) {
	product, err := h.service.GetPublicProduct(c.Request.Context(), c.Param("slug"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": product})
}

func (h *Handler) listPublicCategories(c *gin.Context) {
	categories, err := h.service.ListPublicCategories(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": categories})
}

// listProducts is the back office's catalogue table: same filters as the shop,
// plus the unpublished rows.
func (h *Handler) listProducts(c *gin.Context) {
	query := productQuery(c)
	// The storefront pages 24 at a time; the back office table reads the whole
	// catalogue at once, so an absent limit means "as many as one page allows".
	if strings.TrimSpace(c.Query("limit")) == "" {
		query.Limit = 100
	}
	products, total, err := h.service.AdminProducts(c.Request.Context(), query)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": products, "total": total})
}

// listLowStock answers "what must I restock", using the shop's own threshold
// rather than a hard-coded number the owner cannot tune.
func (h *Handler) listLowStock(c *gin.Context) {
	products, err := h.service.LowStockProducts(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	threshold := h.service.AlertThreshold()
	c.JSON(http.StatusOK, gin.H{"data": products, "threshold": threshold})
}

func contextUserID(c *gin.Context) uint {
	value, exists := c.Get(middleware.UserIDKey)
	if !exists {
		return 0
	}
	switch id := value.(type) {
	case uint:
		return id
	case int:
		if id > 0 {
			return uint(id)
		}
	}
	return 0
}

func (h *Handler) getProduct(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	product, err := h.service.GetProduct(c.Request.Context(), id)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": product})
}

func (h *Handler) createProduct(c *gin.Context) {
	var input productInput
	if !bindJSON(c, &input) {
		return
	}
	input.Product.StockVisible = onUnlessTurnedOff(input.StockVisible)
	input.Product.IsPublished = onUnlessTurnedOff(input.IsPublished)
	if err := h.service.CreateProduct(c.Request.Context(), &input.Product); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": input.Product})
}

func (h *Handler) updateProduct(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var input domain.Product
	if !bindJSON(c, &input) {
		return
	}
	product, err := h.service.UpdateProduct(c.Request.Context(), id, &input)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": product})
}

func (h *Handler) deleteProduct(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteProduct(c.Request.Context(), id); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) listCards(c *gin.Context) {
	productID, ok := parseID(c, "id")
	if !ok {
		return
	}
	cards, err := h.service.ListCards(c.Request.Context(), productID)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": cards})
}

func (h *Handler) addCard(c *gin.Context) {
	productID, ok := parseID(c, "id")
	if !ok {
		return
	}
	var card domain.Card
	if !bindJSON(c, &card) {
		return
	}
	if err := h.service.AddCard(c.Request.Context(), productID, &card); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": card})
}

// listAllCards backs the inventory screen: every product or a selected one,
// by status, with a key search and server-side paging.
func (h *Handler) listAllCards(c *gin.Context) {
	cards, total, err := h.service.ListCardsFiltered(c.Request.Context(), cardFilter(c))
	if err != nil {
		respondError(c, err)
		return
	}
	rows := make([]gin.H, 0, len(cards))
	for _, card := range cards {
		rows = append(rows, cardRow(card))
	}
	c.JSON(http.StatusOK, gin.H{"data": rows, "total": total})
}

// cardFilter reads the inventory screen's query string. product_id is optional:
// without it the shop sees its whole key stock in one list.
func cardFilter(c *gin.Context) domain.CardFilter {
	filter := domain.CardFilter{
		Status: strings.TrimSpace(c.Query("status")),
		Search: strings.TrimSpace(c.Query("q")),
	}
	if value, err := strconv.ParseUint(strings.TrimSpace(c.Query("product_id")), 10, 32); err == nil && value > 0 {
		id := uint(value)
		filter.ProductID = &id
	}
	filter.Limit = positiveInt(c.Query("limit"), 50)
	filter.Offset = positiveInt(c.Query("offset"), 0)
	return filter
}

// cardRow flattens a card with the two names a shop owner reads it by: which
// product it belongs to, and which order consumed it.
func cardRow(card domain.Card) gin.H {
	row := gin.H{
		"id":         card.ID,
		"product_id": card.ProductID,
		"content":    card.Content,
		"status":     card.Status,
		"created_at": card.CreatedAt,
		"sold_at":    card.SoldAt,
		"order_id":   card.OrderID,
	}
	if card.Product.ID != 0 {
		row["product_name"] = card.Product.Name
		row["product_slug"] = card.Product.Slug
	}
	if card.Order.ID != 0 {
		row["order_no"] = card.Order.OrderNo
	}
	return row
}

// exportCards streams the current selection as a CSV download so a shop owner
// can move keys between tools. The UTF-8 BOM is not decoration: Excel opens a
// Chinese header row as mojibake without it.
func (h *Handler) exportCards(c *gin.Context) {
	cards, truncated, err := h.service.ExportCards(c.Request.Context(), cardFilter(c))
	if err != nil {
		respondError(c, err)
		return
	}
	name := "cards"
	if id := c.Query("product_id"); id != "" {
		name = "cards-" + id
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="`+sanitizeFilename(name)+`.csv"`)
	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(c.Writer)
	_ = writer.Write([]string{"id", "商品", "卡密", "状态", "售出时间", "订单号"})
	for _, card := range cards {
		soldAt := ""
		if card.SoldAt != nil {
			soldAt = card.SoldAt.Format(time.RFC3339)
		}
		_ = writer.Write([]string{
			strconv.FormatUint(uint64(card.ID), 10),
			card.Product.Name,
			card.Content,
			card.Status,
			soldAt,
			card.Order.OrderNo,
		})
	}
	writer.Flush()
	if truncated {
		_, _ = c.Writer.Write([]byte("# 结果已截断，请缩小筛选范围后再导出\n"))
	}
}

// sanitizeFilename keeps a query-string value out of a response header.
func sanitizeFilename(value string) string {
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			builder.WriteRune(r)
		}
	}
	if builder.Len() == 0 {
		return "cards"
	}
	return builder.String()
}

type generateCardsRequest struct {
	Count  int    `json:"count"`
	Prefix string `json:"prefix"`
}

func (h *Handler) generateCards(c *gin.Context) {
	productID, ok := parseID(c, "id")
	if !ok {
		return
	}
	var request generateCardsRequest
	if !bindJSON(c, &request) {
		return
	}
	result, err := h.service.GenerateCards(c.Request.Context(), productID, request.Count, request.Prefix)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

type cardBatchRequest struct {
	CardIDs []uint `json:"card_ids"`
	IDs     []uint `json:"ids"`
	Status  string `json:"status"`
}

// selected returns either spelling of the id list; the SPA and the curl examples
// in the README both send card_ids, and a body that says ids should not fail.
func (r cardBatchRequest) selected() []uint {
	if len(r.CardIDs) > 0 {
		return r.CardIDs
	}
	return r.IDs
}

func (h *Handler) batchCardStatus(c *gin.Context) {
	productID, ok := parseID(c, "id")
	if !ok {
		return
	}
	var request cardBatchRequest
	if !bindJSON(c, &request) {
		return
	}
	moved, err := h.service.SetCardStatus(c.Request.Context(), productID, request.selected(), request.Status)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"updated": moved})
}

func (h *Handler) batchDeleteCards(c *gin.Context) {
	productID, ok := parseID(c, "id")
	if !ok {
		return
	}
	var request cardBatchRequest
	if !bindJSON(c, &request) {
		return
	}
	deleted, err := h.service.DeleteCards(c.Request.Context(), productID, request.selected())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

type batchAddCardsRequest struct {
	Cards    []string `json:"cards"`
	Contents []string `json:"contents"`
	Content  string   `json:"content"`
}

func (h *Handler) batchAddCards(c *gin.Context) {
	productID, ok := parseID(c, "id")
	if !ok {
		return
	}
	var request batchAddCardsRequest
	if !bindJSON(c, &request) {
		return
	}
	contents := request.Cards
	if len(contents) == 0 {
		contents = request.Contents
	}
	if len(contents) == 0 && request.Content != "" {
		contents = strings.Split(strings.ReplaceAll(request.Content, "\r\n", "\n"), "\n")
	}
	result, err := h.service.ImportCards(c.Request.Context(), productID, contents)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (h *Handler) updateCard(c *gin.Context) {
	productID, ok := parseID(c, "id")
	if !ok {
		return
	}
	cardID, ok := parseID(c, "card_id")
	if !ok {
		return
	}
	var input domain.Card
	if !bindJSON(c, &input) {
		return
	}
	card, err := h.service.UpdateCard(c.Request.Context(), productID, cardID, &input)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": card})
}

func (h *Handler) deleteCard(c *gin.Context) {
	productID, ok := parseID(c, "id")
	if !ok {
		return
	}
	cardID, ok := parseID(c, "card_id")
	if !ok {
		return
	}
	if err := h.service.DeleteCard(c.Request.Context(), productID, cardID); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) listCategories(c *gin.Context) {
	categories, err := h.service.ListCategories(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": categories})
}

func (h *Handler) createCategory(c *gin.Context) {
	var input categoryInput
	if !bindJSON(c, &input) {
		return
	}
	input.Category.IsVisible = onUnlessTurnedOff(input.IsVisible)
	if err := h.service.CreateCategory(c.Request.Context(), &input.Category); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": input.Category})
}

func (h *Handler) updateCategory(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var input domain.Category
	if !bindJSON(c, &input) {
		return
	}
	category, err := h.service.UpdateCategory(c.Request.Context(), id, &input)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": category})
}

func (h *Handler) deleteCategory(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteCategory(c.Request.Context(), id); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) listCoupons(c *gin.Context) {
	coupons, err := h.service.ListCoupons(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": coupons})
}

func (h *Handler) createCoupon(c *gin.Context) {
	var input couponInput
	if !bindJSON(c, &input) {
		return
	}
	// A code the payload says nothing about arrives switched on, which is what
	// 新建优惠码 has always meant; 前台展示 is the opposite and waits to be asked.
	input.Coupon.IsActive = onUnlessTurnedOff(input.IsActive)
	if err := h.service.CreateCoupon(c.Request.Context(), &input.Coupon); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": input.Coupon})
}

func (h *Handler) updateCoupon(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var input domain.Coupon
	if !bindJSON(c, &input) {
		return
	}
	coupon, err := h.service.UpdateCoupon(c.Request.Context(), id, &input)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": coupon})
}

func (h *Handler) deleteCoupon(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteCoupon(c.Request.Context(), id); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// onUnlessTurnedOff reads a create-time switch that used to live in the column
// default: a payload that never mentioned it means "on", and a payload that said
// false now actually gets false. The columns no longer carry DEFAULT true, so
// this is the only place that assumption survives — which is the point, since it
// stops a turned-off box from being written back as turned on.
func onUnlessTurnedOff(flag *bool) bool {
	return flag == nil || *flag
}

// The create payloads shadow the row's bools with *bools so the handler can tell
// "absent" from "off". Updates keep the plain row shape: a PUT replaces the row,
// and both SPAs send it in full.
type productInput struct {
	domain.Product
	StockVisible *bool `json:"stock_visible"`
	IsPublished  *bool `json:"is_published"`
}

type categoryInput struct {
	domain.Category
	IsVisible *bool `json:"is_visible"`
}

type couponInput struct {
	domain.Coupon
	IsActive *bool `json:"is_active"`
}

func bindJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		// The form the shop owner filled is the thing at fault, so the answer says
		// what to do rather than quoting Go's JSON parser.
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "提交的内容无法解析，请检查表单后重试。",
			"code":   "invalid_request",
			"detail": err.Error(),
		})
		return false
	}
	return true
}

func parseID(c *gin.Context, name string) (uint, bool) {
	value, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || value == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "地址里的编号不对，请从列表页重新进入。",
			"code":  "invalid_id",
		})
		return 0, false
	}
	return uint(value), true
}

// respondError answers with what a caller can act on: the status, a machine code,
// and words the person reading them can understand. Every catalogue failure has
// a name, so the driver's own English ("record not found", "UNIQUE constraint
// failed: products.slug") never reaches a Chinese screen as if it were the shop's
// own message.
func respondError(c *gin.Context, err error) {
	status, code, message := errorCopy(err)
	if status == http.StatusInternalServerError {
		// The client only sees a generic message, so the real cause has to reach
		// the server log or unexplained 500s cannot be diagnosed.
		log.Printf("[catalog] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
	}
	c.JSON(status, gin.H{"error": message, "code": code})
}

// errorCopy keys the wording off the rule that broke, the same way identity answers
// its callers. A validation error carries the specific complaint, so its own tail
// is kept; the rest get a fixed sentence written for the back office.
func errorCopy(err error) (int, string, string) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return http.StatusNotFound, "not_found", "这条记录不存在，或者已经被删除。"
	case errors.Is(err, domain.ErrInvalidInput):
		return http.StatusBadRequest, "invalid_input", detail(err, "信息填得不对，请检查后重试。")
	case errors.Is(err, application.ErrInvalidQuery):
		return http.StatusBadRequest, "invalid_query", detail(err, "查询条件不对，请调整后重试。")
	case errors.Is(err, application.ErrInvalidProductType):
		return http.StatusBadRequest, "invalid_product_type", "商品类型只能是 card（卡密自动发货）或 manual（手动发货）。"
	case errors.Is(err, application.ErrInvalidCardStatus):
		return http.StatusBadRequest, "invalid_card_status", "卡密状态只能是 available（可用）或 disabled（停用）。"
	case errors.Is(err, application.ErrManualProductCard):
		return http.StatusBadRequest, "manual_product", "手动发货的商品没有卡密库存，无需添加或管理卡密。"
	case errors.Is(err, domain.ErrProductSlugTaken):
		return http.StatusConflict, "slug_taken", "这个 slug 已经有商品在用，换一个再保存。"
	case errors.Is(err, domain.ErrCategorySlugTaken):
		return http.StatusConflict, "slug_taken", "这个 slug 已经有分类在用，换一个再保存。"
	case errors.Is(err, domain.ErrCouponCodeTaken):
		return http.StatusConflict, "code_taken", "这个优惠码已经存在，不要重复创建。"
	}
	// A quoted code can be refused for a reason the buyer acts on differently from
	// a form error, and the refusal already carries its own words.
	var refusal *application.CouponError
	if errors.As(err, &refusal) {
		return http.StatusUnprocessableEntity, refusal.CouponCode(), refusal.CouponMessage()
	}
	return http.StatusInternalServerError, "internal_error", "服务器开小差了，请稍后再试。"
}

// detail keeps the reason a wrapped sentinel carries ("invalid input: 商品名称和
// slug 都要填写。" → the part after the colon) and falls back when there is none.
func detail(err error, fallback string) string {
	_, tail, found := strings.Cut(err.Error(), ": ")
	if found && strings.TrimSpace(tail) != "" {
		return tail
	}
	return fallback
}
