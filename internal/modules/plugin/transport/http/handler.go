package http

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	middleware "github.com/kaoqy/Nodeloc-Store/internal/app/httpserver"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/domain"
)

type Handler struct {
	service *application.Service
}

func NewHandler(service *application.Service) *Handler { return &Handler{service: service} }

// RegisterRoutes wires the storefront's public plugin descriptor.
//
// 插件管理后台页面已下线；支付交付所需的 provider、绑定与履约逻辑仍由
// 内部 service 服务，后台不再暴露安装、卸载或配置管理路由。
func (h *Handler) RegisterRoutes(engine gin.IRouter, jwtConfig *config.JWTConfig, accounts middleware.AccountReader) {
	// The storefront asks what a product needs from a plugin. It is public
	// because it renders before login and says nothing secret.
	public := engine.Group("/api/v1")
	public.GET("/products/:id/plugin", h.DescribeProduct)

	// Minimal product-channel wiring for the New-API provider. This is not the
	// retired plugin-management surface: it only toggles one product's delivery
	// channel and is guarded by the product-editing permission.
	admin := engine.Group("/api/v1/admin", middleware.JWTMiddleware(jwtConfig))
	guard := middleware.RequirePermission(accounts, "products", "manage")
	admin.GET("/products/:id/new-api-delivery", guard, h.GetNewAPIProduct)
	admin.PUT("/products/:id/new-api-delivery", guard, h.SetNewAPIProduct)
}

type newAPIProductRequest struct {
	Enabled bool `json:"enabled"`
}

func (h *Handler) GetNewAPIProduct(c *gin.Context) {
	productID := uint(atoiDefault(c.Param("id"), 0))
	enabled, err := h.service.NewAPIProductEnabled(c.Request.Context(), productID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"enabled": enabled}})
}

func (h *Handler) SetNewAPIProduct(c *gin.Context) {
	productID := uint(atoiDefault(c.Param("id"), 0))
	var request newAPIProductRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "code": "invalid_input"})
		return
	}
	if err := h.service.SetNewAPIProduct(c.Request.Context(), productID, request.Enabled); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"enabled": request.Enabled}})
}

// DescribeProduct answers the storefront's 「这个商品需要插件做什么」 question.
func (h *Handler) DescribeProduct(c *gin.Context) {
	productID := uint(atoiDefault(c.Param("id"), 0))
	describe, err := h.service.DescribeProduct(c.Request.Context(), productID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": describe})
}

func atoiDefault(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

// writeError is the plugin module's own copy table: the admin screen reads
// 「哪一个字段没填」 rather than a stack trace.
func writeError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	code := "internal_error"
	message := "插件操作失败，请稍后重试"

	switch {
	case errors.Is(err, domain.ErrPluginNotFound):
		status, code, message = http.StatusNotFound, "plugin_not_found", "插件不存在"
	case errors.Is(err, domain.ErrBindingNotFound):
		status, code, message = http.StatusNotFound, "binding_not_found", "匹配规则不存在"
	case errors.Is(err, domain.ErrPluginDisabled):
		status, code, message = http.StatusConflict, "plugin_disabled", "这个插件目前是停用状态"
	case errors.Is(err, domain.ErrNoBinding):
		status, code, message = http.StatusConflict, "no_binding", detail(err, "这个商品还没有匹配到对应的交付项目")
	case errors.Is(err, domain.ErrInvalidInput):
		status, code, message = http.StatusBadRequest, "invalid_input", detail(err, "插件配置填写有误")
	}
	c.JSON(status, gin.H{"error": message, "code": code})
}

func detail(err error, fallback string) string {
	_, tail, found := strings.Cut(err.Error(), ": ")
	if found && strings.TrimSpace(tail) != "" {
		return tail
	}
	return fallback
}
