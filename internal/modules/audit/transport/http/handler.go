package http

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	middleware "github.com/kaoqy/Nodeloc-Store/internal/app/httpserver"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit/domain"
)

// Handler exposes audit use cases over HTTP.
type Handler struct {
	service *application.Service
}

func NewHandler(service *application.Service) *Handler {
	if service == nil {
		panic("audit: nil service")
	}
	return &Handler{service: service}
}

// RegisterRoutes registers the audit-log reads for a role that holds the
// logs:view grant: the filtered log page and the action names it suggests.
func (h *Handler) RegisterRoutes(router gin.IRouter, jwtConfig *config.JWTConfig, accounts middleware.AccountReader) {
	router.GET("/api/v1/admin/audit-logs",
		middleware.JWTMiddleware(jwtConfig),
		middleware.RequirePermission(accounts, "logs", "view"),
		h.ListAuditLogs)
	router.GET("/api/v1/admin/audit-logs/actions",
		middleware.JWTMiddleware(jwtConfig),
		middleware.RequirePermission(accounts, "logs", "view"),
		h.ListAuditActions)
}

// ListAuditLogs returns audit logs for the page, limit, action, search, actor
// and date-range query parameters.
func (h *Handler) ListAuditLogs(c *gin.Context) {
	filter, err := parseAuditQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_query"})
		return
	}

	result, err := h.service.QueryLogs(c.Request.Context(), filter)
	if err != nil {
		// The log page is the shop's own record of what happened, so a failure here
		// is worth a line of server log rather than only a bare 500.
		log.Printf("[audit] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "审计日志暂时读不出来，请稍后再试。", "code": "internal_error"})
		return
	}
	c.JSON(http.StatusOK, result)
}

// ListAuditActions reports which actions this shop has recorded so far.
func (h *Handler) ListAuditActions(c *gin.Context) {
	actions, err := h.service.ListActions(c.Request.Context())
	if err != nil {
		log.Printf("[audit] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "操作类型暂时读不出来，请稍后再试。", "code": "internal_error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": actions})
}

func parseAuditQuery(c *gin.Context) (domain.LogFilter, error) {
	filter := domain.LogFilter{
		Action: strings.TrimSpace(c.Query("action")),
		Search: strings.TrimSpace(c.Query("search")),
	}

	page, err := positiveIntQuery(c, "page", 1)
	if err != nil {
		return filter, err
	}
	limit, err := positiveIntQuery(c, "limit", 20)
	if err != nil {
		return filter, err
	}
	filter.Page, filter.Limit = page, limit

	if raw := strings.TrimSpace(c.Query("actor")); raw != "" {
		if strings.EqualFold(raw, "system") {
			filter.SystemOnly = true
		} else {
			id, parseErr := strconv.ParseUint(raw, 10, 64)
			if parseErr != nil || id == 0 {
				return filter, errors.New("操作者要填用户 ID（数字），或者选择「系统操作」。")
			}
			actorID := uint(id)
			filter.ActorID = &actorID
		}
	}

	since, err := dayQuery(c, "since", "开始日期")
	if err != nil {
		return filter, err
	}
	until, err := dayQuery(c, "until", "结束日期")
	if err != nil {
		return filter, err
	}
	filter.Since = since
	if until != nil {
		// The picked end day stays included: the bound is the next midnight.
		next := until.AddDate(0, 0, 1)
		filter.Before = &next
	}
	if filter.Since != nil && filter.Before != nil && !filter.Since.Before(*filter.Before) {
		return filter, errors.New("开始日期不能晚于结束日期，请重新选一段时间。")
	}
	return filter, nil
}

// dayQuery reads one YYYY-MM-DD date from the log page's range picker. Local
// midnight keeps the window on the wall clock the owner is reading.
func dayQuery(c *gin.Context, name, label string) (*time.Time, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil, nil
	}
	value, err := time.ParseInLocation("2006-01-02", raw, time.Local)
	if err != nil {
		return nil, fmt.Errorf("%s要写成 2026-09-29 这样的日期。", label)
	}
	return &value, nil
}

func positiveIntQuery(c *gin.Context, name string, fallback int) (int, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, &queryError{name: name}
	}
	return value, nil
}

type queryError struct{ name string }

// Written for the person reading the log page: the pagination parameters come
// from the browser's address bar, so the sentence names the page rather than
// repeating the raw query key.
func (e *queryError) Error() string {
	if e.name == "limit" {
		return "每页条数要是 1 以上的整数。"
	}
	return "页码要是 1 以上的整数。"
}
