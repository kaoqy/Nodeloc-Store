package http

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
	middleware "github.com/kaoqy/Nodeloc-Store/internal/app/httpserver"
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

// RegisterRoutes registers GET /api/v1/admin/audit-logs with JWT + admin middleware.
func (h *Handler) RegisterRoutes(router gin.IRoutes, jwtConfig *config.JWTConfig) {
	admin := router.Group("/api/v1/admin")
	admin.Use(middleware.JWTMiddleware(jwtConfig), middleware.RequireAdmin())
	admin.GET("/audit-logs", h.ListAuditLogs)
}

// ListAuditLogs returns audit logs using page, limit, and optional action query parameters.
func (h *Handler) ListAuditLogs(c *gin.Context) {
	page, err := positiveIntQuery(c, "page", 1)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	limit, err := positiveIntQuery(c, "limit", 20)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.service.QueryLogs(c.Request.Context(), domain.LogFilter{
		Action: strings.TrimSpace(c.Query("action")),
		Page:   page,
		Limit:  limit,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query audit logs"})
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

func (e *queryError) Error() string { return e.name + " must be a positive integer" }
