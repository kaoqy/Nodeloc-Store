package http

import (
	"encoding/csv"
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
// logs:view grant: the filtered log page, the action names it suggests, and the
// CSV download of whatever that page is showing.
func (h *Handler) RegisterRoutes(router gin.IRouter, jwtConfig *config.JWTConfig, accounts middleware.AccountReader) {
	router.GET("/api/v1/admin/audit-logs",
		middleware.JWTMiddleware(jwtConfig),
		middleware.RequirePermission(accounts, "logs", "view"),
		h.ListAuditLogs)
	router.GET("/api/v1/admin/audit-logs/actions",
		middleware.JWTMiddleware(jwtConfig),
		middleware.RequirePermission(accounts, "logs", "view"),
		h.ListAuditActions)
	router.GET("/api/v1/admin/audit-logs/export",
		middleware.JWTMiddleware(jwtConfig),
		middleware.RequirePermission(accounts, "logs", "view"),
		h.ExportAuditLogs)
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

// exportLogColumns is the CSV header. The id comes first because that is what
// the log page numbers its rows with, so a line in the file can be pointed back
// at the screen. The actor column carries the username, because a download that
// only said "actor_id 7" cannot be read by anyone who was not looking at the
// page when it happened.
var exportLogColumns = []string{"日志ID", "时间", "操作者", "操作类型", "对象", "详情", "IP 地址"}

// ExportAuditLogs writes the filtered log as a CSV download. The UTF-8 BOM is
// not decoration: Excel opens a Chinese header row as mojibake without it.
func (h *Handler) ExportAuditLogs(c *gin.Context) {
	filter, err := parseAuditFilters(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_query"})
		return
	}

	rows, truncated, err := h.service.ExportLogs(c.Request.Context(), filter)
	if err != nil {
		log.Printf("[audit] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "审计日志暂时导不出来，请稍后再试。", "code": "internal_error"})
		return
	}

	// The filename carries no request input, so nothing from the query string can
	// reach a response header.
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="audit-logs-`+time.Now().Format("20060102")+`.csv"`)
	// The page reports these back to the owner instead of counting the rows
	// itself: a detail carrying a newline would be miscounted.
	c.Header("X-Export-Rows", strconv.Itoa(len(rows)))
	if truncated {
		c.Header("X-Export-Truncated", "1")
	}
	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(c.Writer)
	_ = writer.Write(exportLogColumns)
	for _, entry := range rows {
		_ = writer.Write([]string{
			strconv.FormatUint(uint64(entry.ID), 10),
			entry.CreatedAt.Format(time.RFC3339),
			auditActor(entry),
			entry.Action,
			safeCell(deref(entry.Target)),
			safeCell(deref(entry.Detail)),
			safeCell(deref(entry.IP)),
		})
	}
	writer.Flush()
	if truncated {
		_, _ = c.Writer.Write([]byte("# 结果已截断，请缩小日期范围或加筛选条件后再导出\n"))
	}
}

// auditActor names who acted: a member by username, the shop itself as 系统, and
// an account that has since been purged by id rather than as a blank.
func auditActor(entry domain.AuditLog) string {
	if entry.ActorName != "" {
		return safeCell(entry.ActorName)
	}
	if entry.ActorID == nil {
		return "系统"
	}
	return fmt.Sprintf("#%d（账号已删除）", *entry.ActorID)
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// safeCell defuses spreadsheet formulas. Targets and details are built partly
// from what users type, and a cell starting with = + - @ executes when the shop
// owner opens the export in Excel — a download of logs has to stay data.
func safeCell(value string) string {
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + value
	}
	return value
}

func parseAuditQuery(c *gin.Context) (domain.LogFilter, error) {
	filter, err := parseAuditFilters(c)
	if err != nil {
		return filter, err
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
	return filter, nil
}

// parseAuditFilters reads the conditions the log page filters by, minus
// pagination. The list and the CSV download both use it, so a download can only
// ever carry the batch the page was showing.
func parseAuditFilters(c *gin.Context) (domain.LogFilter, error) {
	filter := domain.LogFilter{
		Action: strings.TrimSpace(c.Query("action")),
		Search: strings.TrimSpace(c.Query("search")),
	}

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
