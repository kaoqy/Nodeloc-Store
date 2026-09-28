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
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数不完整或格式错误", "code": "invalid_request"})
		return
	}
	if err := authz.SetRolePermissions(role, *body.Permissions); err != nil {
		h.writeError(c, err)
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数不完整或格式错误", "code": "invalid_request"})
		return
	}
	if err := h.service.Install(req); err != nil {
		h.writeError(c, err)
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数不完整或格式错误", "code": "invalid_request"})
		return
	}
	if err := h.service.SaveSettings(body.Settings); err != nil {
		// The row is already written by the time the rebuild fails, so this is not
		// a refused save. Answering 200 with only an `error` field let the page
		// print「已保存，立即生效」 straight over it; the owner has to learn the
		// setting waits for a container restart instead.
		if errors.Is(err, ErrSavedButRestartFailed) {
			log.Printf("[system] settings saved but the runtime rebuild failed: %v", err)
			c.JSON(http.StatusOK, gin.H{
				"ok":              true,
				"restart_pending": true,
				"message":         "设置已经保存，但商店没能自动重建运行时：这一项要等容器重启后才生效。",
			})
			return
		}
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "restart_pending": false})
}

func (h *Handler) TestOAuth(c *gin.Context) {
	url, err := h.service.TestOAuth()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "msg": probeCopy(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "authorize_url": url})
}

// probeCopy words the OAuth probe's failure for the settings page, which shows
// this sentence as the result. The infrastructure errors it can carry are written
// in English for the log, so the two the shop can actually reach are named here;
// anything else keeps the raw cause attached, the same way the payment probe below
// quotes the provider, because a probe the owner cannot diagnose is useless.
func probeCopy(err error) string {
	switch text := err.Error(); {
	case strings.Contains(text, "configuration is incomplete"):
		return "NodeLoc 登录还没配置完整：请填好「NodeLoc 站点地址」「Client ID」「Client Secret」后再试一次。"
	case strings.Contains(text, "state is required"):
		return "商店没能准备好登录状态参数（state），请再点一次测试；仍然失败请查看商店日志。"
	default:
		return "测试未能完成：" + text
	}
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
	status, code, message := errorCopy(err)
	if status == http.StatusInternalServerError {
		// The owner only sees a generic sentence, so the real cause has to reach
		// the server log or an unexplained 500 cannot be diagnosed.
		log.Printf("[system] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
	}
	c.JSON(status, gin.H{"error": message, "code": code})
}

// errorCopy answers the settings and role screens the way the catalogue and
// identity modules answer theirs: a machine code plus words written for the shop
// owner. Until now these routes quoted Casbin and the database directly, which
// on a Chinese back office reads as「authz: unknown role "mystery"」rather than as
// a role the shop does not have.
func errorCopy(err error) (int, string, string) {
	switch {
	case errors.Is(err, ErrAlreadyInstalled):
		return http.StatusConflict, "already_installed", "商店已经初始化过了，要改这些请直接去「系统设置」。"
	case errors.Is(err, ErrNotInstalled):
		return http.StatusServiceUnavailable, "not_installed", "商店还没完成初始化，请先走完初始化向导。"
	case errors.Is(err, ErrInvalidInput):
		return http.StatusBadRequest, "invalid_input", detail(err, "填写的内容不符合要求，请检查后重试。")
	case errors.Is(err, authz.ErrEnforcerNotReady):
		return http.StatusServiceUnavailable, "authz_unavailable", "权限服务还没就绪，请稍后再试；一直如此需要重启商店容器。"
	case errors.Is(err, authz.ErrSuperAdminLocked):
		return http.StatusBadRequest, "super_admin_locked", "超级管理员的权限由系统保留，不在这个页面里改——改坏了没人还能进后台。"
	case errors.Is(err, authz.ErrUnknownRole):
		return http.StatusBadRequest, "unknown_role", "商店里没有这个角色：" + detail(err, "请从角色列表里选择。")
	case errors.Is(err, authz.ErrMalformedPermission):
		return http.StatusBadRequest, "malformed_permission", "这条权限写得不对：" + detail(err, "格式应为「资源:操作」。")
	case errors.Is(err, authz.ErrUnknownPermission):
		return http.StatusBadRequest, "unknown_permission", "这条权限不在商店的清单里：" + detail(err, "请从权限清单里勾选。")
	}
	return http.StatusInternalServerError, "internal_error", "操作未能完成，请稍后再试；反复出现请查看商店日志。"
}

// detail keeps the value a wrapped sentinel carries（authz unknown role: "mystery"
// → 引号里的角色名）so the answer can name what was wrong rather than repeat a
// generic sentence.
func detail(err error, fallback string) string {
	_, tail, found := strings.Cut(err.Error(), ": ")
	if found && strings.TrimSpace(tail) != "" {
		return tail
	}
	return fallback
}
