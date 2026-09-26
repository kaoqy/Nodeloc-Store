package system

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	middleware "github.com/kaoqy/Nodeloc-Store/internal/app/httpserver"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
)
type Handler struct {
	service *Service
}

func newHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterPublicRoutes exposes the wizard endpoints (used in bootstrap mode).
func (h *Handler) RegisterPublicRoutes(router gin.IRouter) {
	group := router.Group("/api/v1/system")
	group.GET("/status", h.Status)
	group.POST("/install", h.Install)
}

// RegisterRoutes adds the admin settings endpoints to a booted router.
func (h *Handler) RegisterRoutes(router gin.IRouter, jwtConfig *config.JWTConfig, accounts middleware.AccountReader) {
	h.RegisterPublicRoutes(router)
	admin := router.Group("/api/v1/admin", middleware.JWTMiddleware(jwtConfig), middleware.RequireAdmin(accounts))
	admin.GET("/settings", h.GetSettings)
	admin.PUT("/settings", h.SaveSettings)
	admin.POST("/settings", h.SaveSettings)
	admin.POST("/settings/oauth-test", h.TestOAuth)
	admin.POST("/settings/payment-test", h.TestPayment)
	admin.GET("/stats", h.Stats)
}

func (h *Handler) Status(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.Status())
}

func (h *Handler) Install(c *gin.Context) {
	var req InstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数不完整或格式错误"})
		return
	}
	if err := h.service.Install(req); err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrAlreadyInstalled):
			status = http.StatusConflict
		case errors.Is(err, ErrInvalidInput):
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "initialized": true})
}

func (h *Handler) GetSettings(c *gin.Context) {
	settings, err := h.service.GetSettings()
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, settings)
}

func (h *Handler) SaveSettings(c *gin.Context) {
	var body struct {
		Settings RuntimeConfig `json:"settings"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数不完整或格式错误"})
		return
	}
	if err := h.service.SaveSettings(body.Settings); err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) TestOAuth(c *gin.Context) {
	url, err := h.service.TestOAuth()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "authorize_url": url})
}

func (h *Handler) TestPayment(c *gin.Context) {
	ok, msg := h.service.TestPayment(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"ok": ok, "msg": msg})
}

func (h *Handler) Stats(c *gin.Context) {
	days, err := strconv.Atoi(c.DefaultQuery("days", "30"))
	if err != nil || days <= 0 {
		days = 30
	}
	stats, err := h.service.Stats(c.Request.Context(), days)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *Handler) writeError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrNotInstalled):
		status = http.StatusServiceUnavailable
	case errors.Is(err, ErrInvalidInput):
		status = http.StatusBadRequest
	case errors.Is(err, ErrSavedButRestartFailed):
		status = http.StatusOK
	}
	message := err.Error()
	if status == http.StatusInternalServerError {
		log.Printf("[system] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
	}
	c.JSON(status, gin.H{"error": message})
}
