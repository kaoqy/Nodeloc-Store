package http

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

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

// RegisterRoutes installs the public store routes and JWT-protected admin routes.
func (h *Handler) RegisterRoutes(router gin.IRouter, jwtConfig *config.JWTConfig) {
	store := router.Group("/api/v1/store")
	store.GET("/products", h.listPublicProducts)
	store.GET("/products/:slug", h.getPublicProduct)
	store.GET("/categories", h.listPublicCategories)

	admin := router.Group("/api/v1/admin")
	admin.Use(middleware.JWTMiddleware(jwtConfig), middleware.RequireAdmin())

	admin.GET("/products", h.listProducts)
	admin.POST("/products", h.createProduct)
	admin.PUT("/products/:id", h.updateProduct)
	admin.DELETE("/products/:id", h.deleteProduct)

	admin.GET("/products/:id/cards", h.listCards)
	admin.POST("/products/:id/cards", h.addCard)
	admin.PUT("/products/:id/cards/:card_id", h.updateCard)
	admin.DELETE("/products/:id/cards/:card_id", h.deleteCard)
	admin.POST("/products/:id/cards/batch-add", h.batchAddCards)

	admin.GET("/categories", h.listCategories)
	admin.POST("/categories", h.createCategory)
	admin.PUT("/categories/:id", h.updateCategory)
	admin.DELETE("/categories/:id", h.deleteCategory)

	admin.GET("/coupons", h.listCoupons)
	admin.POST("/coupons", h.createCoupon)
	admin.PUT("/coupons/:id", h.updateCoupon)
	admin.DELETE("/coupons/:id", h.deleteCoupon)
}

func (h *Handler) listPublicProducts(c *gin.Context) {
	products, err := h.service.ListPublicProducts(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": products})
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

func (h *Handler) listProducts(c *gin.Context) {
	products, err := h.service.ListProducts(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": products})
}

func (h *Handler) createProduct(c *gin.Context) {
	var product domain.Product
	if !bindJSON(c, &product) {
		return
	}
	if err := h.service.CreateProduct(c.Request.Context(), &product); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": product})
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
	cards, err := h.service.AddCards(c.Request.Context(), productID, contents)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": cards, "count": len(cards)})
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
	var category domain.Category
	if !bindJSON(c, &category) {
		return
	}
	if err := h.service.CreateCategory(c.Request.Context(), &category); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": category})
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
	var coupon domain.Coupon
	if !bindJSON(c, &coupon) {
		return
	}
	if err := h.service.CreateCoupon(c.Request.Context(), &coupon); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": coupon})
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

func bindJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "detail": err.Error()})
		return false
	}
	return true
}

func parseID(c *gin.Context, name string) (uint, bool) {
	value, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || value == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + name})
		return 0, false
	}
	return uint(value), true
}

func respondError(c *gin.Context, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, gorm.ErrRecordNotFound) {
		status = http.StatusNotFound
	}
	c.JSON(status, gin.H{"error": err.Error()})
}
