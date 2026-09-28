package system

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	middleware "github.com/kaoqy/Nodeloc-Store/internal/app/httpserver"
	"github.com/kaoqy/Nodeloc-Store/internal/authz"
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
	guard := func(resource, action string) gin.HandlerFunc {
		return middleware.RequirePermission(accounts, resource, action)
	}
	admin := router.Group("/api/v1/admin", middleware.JWTMiddleware(jwtConfig))
	admin.GET("/settings", guard("settings", "view"), h.GetSettings)
	admin.PUT("/settings", guard("settings", "manage"), h.SaveSettings)
	admin.POST("/settings", guard("settings", "manage"), h.SaveSettings)
	admin.POST("/settings/oauth-test", guard("settings", "manage"), h.TestOAuth)
	admin.POST("/settings/payment-test", guard("settings", "manage"), h.TestPayment)
	admin.GET("/stats", guard("stats", "view"), h.Stats)
	admin.GET("/permissions", guard("roles", "view"), h.ListPermissions)
	admin.GET("/roles", guard("roles", "view"), h.ListRoles)
	admin.PUT("/roles/:role/permissions", guard("roles", "manage"), h.SaveRolePermissions)
}

// ListPermissions hands the role editor the grantable vocabulary, so it can
// render checkboxes instead of letting someone type policy strings.
func (h *Handler) ListPermissions(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"data": authz.PermissionCatalog()})
}

// ListRoles returns every back-office role with its current grants. super_admin
// reports the wildcard it actually holds rather than an expanded list, because
// the editor refuses to write to that role.
func (h *Handler) ListRoles(c *gin.Context) {
	permissions := authz.RolePermissions()
	headcounts, err := h.service.RoleHeadcounts()
	if err != nil {
		// The matrix is still worth showing without the counts.
		log.Printf("[system] role headcounts: %v", err)
		headcounts = nil
	}
	type roleRow struct {
		Role        string   `json:"role"`
		Label       string   `json:"label"`
		Editable    bool     `json:"editable"`
		Permissions []string `json:"permissions"`
		UserCount   int64    `json:"user_count"`
	}
	rows := make([]roleRow, 0, len(authz.Roles))
	for _, role := range authz.Roles {
		granted := permissions[role]
		if role == "super_admin" {
			granted = []string{"*:*"}
		}
		if granted == nil {
			granted = []string{}
		}
		rows = append(rows, roleRow{
			Role:        role,
			Label:       roleLabel(role),
			Editable:    role != "super_admin",
			Permissions: granted,
			UserCount:   headcounts[role],
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

// SaveRolePermissions replaces one role's grants in a single write, so a saved
// matrix is never half-applied.
func (h *Handler) SaveRolePermissions(c *gin.Context) {
	role := strings.TrimSpace(c.Param("role"))
	var body struct {
		Permissions *[]string `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Permissions == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数不完整或格式错误"})
		return
	}
	if err := authz.SetRolePermissions(role, *body.Permissions); err != nil {
		status := http.StatusInternalServerError
		if role == "super_admin" || strings.Contains(err.Error(), "unknown role") ||
			strings.Contains(err.Error(), "malformed permission") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "role": role, "permissions": authz.PermissionsOf(role)})
}

func roleLabel(role string) string {
	switch role {
	case "super_admin":
		return "超级管理员"
	case "admin":
		return "管理员"
	case "operator":
		return "运营"
	case "support":
		return "客服"
	}
	return role
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
