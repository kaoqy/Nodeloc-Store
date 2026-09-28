package http

import (
	"log"
	"net/http"
	"strconv"
	"strings"

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

// RegisterRoutes registers GET /api/v1/admin/audit-logs for a role that holds
// the logs:view grant.
func (h *Handler) RegisterRoutes(router gin.IRouter, jwtConfig *config.JWTConfig, accounts middleware.AccountReader) {
	router.GET("/api/v1/admin/audit-logs",
		middleware.JWTMiddleware(jwtConfig),
		middleware.RequirePermission(accounts, "logs", "view"),
		h.ListAuditLogs)
}

// ListAuditLogs returns audit logs using page, limit, and optional action query parameters.
func (h *Handler) ListAuditLogs(c *gin.Context) {
	page, err := positiveIntQuery(c, "page", 1)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_query"})
		return
	}
	limit, err := positiveIntQuery(c, "limit", 20)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_query"})
		return
	}

	result, err := h.service.QueryLogs(c.Request.Context(), domain.LogFilter{
		Action: strings.TrimSpace(c.Query("action")),
		Page:   page,
		Limit:  limit,
	})
	if err != nil {
		// The log page is the shop's own record of what happened, so a failure here
		// is worth a line of server log rather than only a bare 500.
		log.Printf("[audit] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "审计日志暂时读不出来，请稍后再试。", "code": "internal_error"})
		return
	}
	c.JSON(http.StatusOK, result)
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
