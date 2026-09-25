package system

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
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
func (h *Handler) RegisterRoutes(router gin.IRouter, jwtConfig *config.JWTConfig, auth gin.HandlerFunc) {
	h.RegisterPublicRoutes(router)
	admin := router.Group("/api/v1/admin")
	if auth != nil {
		admin = admin.Group("", auth, h.requireAdmin())
	}
	admin.GET("/settings", h.GetSettings)
	admin.PUT("/settings", h.SaveSettings)
	admin.POST("/settings", h.SaveSettings)
	admin.POST("/settings/oauth-test", h.TestOAuth)
	admin.POST("/settings/payment-test", h.TestPayment)
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

// requireAdmin rejects non-admin authenticated users.
func (h *Handler) requireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := userIDFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		h.service.mu.Lock()
		db := h.service.db
		h.service.mu.Unlock()
		if db == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "not initialized"})
			c.Abort()
			return
		}
		var user models.User
		if err := db.First(&user, id).Error; err != nil || !user.IsAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "需要管理员权限"})
			c.Abort()
			return
		}
		c.Next()
	}
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

func userIDFromContext(c *gin.Context) (uint, bool) {
	v, ok := c.Get("user_id")
	if !ok {
		return 0, false
	}
	switch id := v.(type) {
	case uint:
		return id, id != 0
	case int:
		return uint(id), id > 0
	case float64:
		return uint(id), id > 0
	case string:
		parsed, err := strconv.ParseUint(id, 10, 64)
		return uint(parsed), err == nil && parsed != 0
	default:
		return 0, false
	}
}
