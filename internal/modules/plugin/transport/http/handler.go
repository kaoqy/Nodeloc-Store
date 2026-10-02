package http

import (
	"encoding/json"
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

// RegisterRoutes wires 插件管理 and the public 插件绑定 lookups.
//
// The admin routes are gated per resource. The one public read is the product
// descriptor the storefront uses to render a plugin's choices; it exposes no
// credential and no provider-side identifier the buyer would not otherwise see.
func (h *Handler) RegisterRoutes(engine gin.IRouter, jwtConfig *config.JWTConfig, accounts middleware.AccountReader) {
	guard := func(action string) gin.HandlerFunc {
		return middleware.RequirePermission(accounts, "plugins", action)
	}
	api := engine.Group("/api/v1/plugins")
	api.Use(middleware.JWTMiddleware(jwtConfig))

	api.GET("", guard("view"), h.Catalog)
	api.POST("", guard("manage"), h.Enroll)
	api.GET("/installed", guard("view"), h.Installed)
	api.GET("/bindings", guard("view"), h.Bindings)
	api.POST("/bindings", guard("manage"), h.CreateBinding)
	api.PUT("/bindings/:id", guard("manage"), h.UpdateBinding)
	api.DELETE("/bindings/:id", guard("manage"), h.DeleteBinding)
	// A provider's configuration form is already part of the catalog answer
	// (manifest.config_schema), so there is no separate schema route: one would
	// have to be /plugins/:key/schema next to /plugins/:id, and Gin refuses two
	// different wildcard names at the same position — it panics at registration
	// time, which would take the whole storefront down on every container build.
	api.GET("/:id", guard("view"), h.Get)

	admin := engine.Group("/api/v1/admin/plugins")
	admin.Use(middleware.JWTMiddleware(jwtConfig))
	admin.POST("/:id/enable", guard("manage"), h.SetEnabled)
	admin.POST("/:id/disable", guard("manage"), h.SetEnabled)
	admin.PUT("/:id/config", guard("manage"), h.UpdateConfig)
	admin.DELETE("/:id", guard("manage"), h.Uninstall)

	// The storefront asks what a product needs from a plugin. It is public
	// because it renders before login and says nothing secret.
	public := engine.Group("/api/v1")
	public.GET("/products/:id/plugin", h.DescribeProduct)
}

func (h *Handler) Catalog(c *gin.Context) {
	entries, err := h.service.Catalog(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": entries})
}

func (h *Handler) Installed(c *gin.Context) {
	plugins, err := h.service.Plugins(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": plugins})
}

func (h *Handler) Get(c *gin.Context) {
	plugin, err := h.service.Plugin(c.Request.Context(), idParam(c))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": plugin})
}

func (h *Handler) Enroll(c *gin.Context) {
	var request struct {
		Key string `json:"key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, domain.ErrInvalidInput)
		return
	}
	plugin, err := h.service.Enroll(c.Request.Context(), request.Key)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": plugin})
}

func (h *Handler) SetEnabled(c *gin.Context) {
	enabled := c.FullPath() == "/api/v1/admin/plugins/:id/enable"
	plugin, err := h.service.SetEnabled(c.Request.Context(), idParam(c), enabled)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": plugin})
}

func (h *Handler) UpdateConfig(c *gin.Context) {
	// The values are read as raw JSON first. A checkbox in the browser is a real
	// boolean, and binding straight into map[string]string makes Gin answer 400 on
	// {"notify_buyer": true} — which reads as 「保存失败」 on a form the owner just
	// filled in correctly. flexibleText accepts either spelling.
	var request struct {
		Settings map[string]flexibleText `json:"settings"`
		Secrets  map[string]flexibleText `json:"secrets"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, domain.ErrInvalidInput)
		return
	}
	plugin, err := h.service.UpdateConfig(c.Request.Context(), idParam(c), flatten(request.Settings), flatten(request.Secrets))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": plugin})
}

func (h *Handler) Uninstall(c *gin.Context) {
	if err := h.service.Uninstall(c.Request.Context(), idParam(c)); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "插件已移除"})
}

func (h *Handler) Bindings(c *gin.Context) {
	pluginID := uint(atoiDefault(c.Query("plugin_id"), 0))
	productID := uint(atoiDefault(c.Query("product_id"), 0))
	bindings, err := h.service.Bindings(c.Request.Context(), pluginID, productID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": bindings})
}

func (h *Handler) CreateBinding(c *gin.Context) {
	var request application.BindingInput
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, domain.ErrInvalidInput)
		return
	}
	binding, err := h.service.UpsertBinding(c.Request.Context(), request)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": binding})
}

func (h *Handler) UpdateBinding(c *gin.Context) {
	var request application.BindingInput
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, domain.ErrInvalidInput)
		return
	}
	binding, err := h.service.UpdateBinding(c.Request.Context(), idParam(c), request)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": binding})
}

func (h *Handler) DeleteBinding(c *gin.Context) {
	if err := h.service.DeleteBinding(c.Request.Context(), idParam(c)); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "匹配规则已删除"})
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

// flexibleText is a JSON scalar that may arrive quoted or bare, so a plugin
// setting can be sent as "on" or true without the request being rejected.
type flexibleText string

func (value *flexibleText) UnmarshalJSON(raw []byte) error {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		*value = ""
		return nil
	}
	if text[0] == '"' {
		var decoded string
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return err
		}
		*value = flexibleText(strings.TrimSpace(decoded))
		return nil
	}
	*value = flexibleText(text)
	return nil
}

func flatten(values map[string]flexibleText) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = string(value)
	}
	return out
}

func idParam(c *gin.Context) uint {
	id, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 32)
	if err != nil {
		return 0
	}
	return uint(id)
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
