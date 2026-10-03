package http

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	middleware "github.com/kaoqy/Nodeloc-Store/internal/app/httpserver"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/activity/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/activity/domain"
)

// Handler 是活动模块的 HTTP 边界：后台管理、买家活动中心、优惠券领取。
// Defaults 提供配置中心里的活动默认值；未接线时使用保守默认。
type Defaults interface {
	PerUserLimit(ctx context.Context) int
	AllowStacking(ctx context.Context) bool
}

type Handler struct {
	service  *application.Service
	defaults Defaults
}

// SetDefaults attaches the configuration-centre defaults.
func (h *Handler) SetDefaults(defaults Defaults) { h.defaults = defaults }

func NewHandler(service *application.Service) *Handler {
	if service == nil {
		panic("activity: nil service")
	}
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router gin.IRouter, jwtConfig *config.JWTConfig, accounts middleware.AccountReader) {
	guard := func(resource, action string) gin.HandlerFunc {
		return middleware.RequirePermission(accounts, resource, action)
	}

	// ── 买家侧 ──
	store := router.Group("/api/v1/store/activities")
	store.GET("", h.listPublic)
	store.GET("/:id", h.getPublic)

	me := router.Group("/api/v1/me", middleware.JWTMiddleware(jwtConfig))
	me.GET("/activity-records", h.myRecords)

	claims := router.Group("/api/v1/activities", middleware.OptionalJWTMiddleware(jwtConfig))
	claims.POST("/:id/claim", h.claimCoupon)

	// ── 后台 ──
	admin := router.Group("/api/v1/admin/activities", middleware.JWTMiddleware(jwtConfig))
	admin.GET("", guard("activities", "view"), h.list)
	admin.GET("/:id", guard("activities", "view"), h.get)
	admin.POST("", guard("activities", "manage"), h.create)
	admin.PUT("/:id", guard("activities", "manage"), h.update)
	admin.DELETE("/:id", guard("activities", "manage"), h.remove)
	admin.POST("/:id/duplicate", guard("activities", "manage"), h.duplicate)
	admin.POST("/:id/status", guard("activities", "manage"), h.setStatus)
	admin.GET("/:id/stats", guard("activities", "view"), h.stats)
	admin.GET("/:id/records", guard("activities", "view"), h.records)
	admin.GET("/:id/logs", guard("activities", "view"), h.logs)

	coupons := router.Group("/api/v1/admin/coupon-records", middleware.JWTMiddleware(jwtConfig))
	coupons.GET("", guard("coupons", "view"), h.couponRecords)

	// 活动总览与列表是两条互不冲突的路径：Gin 不允许静态段和 :id 通配
	// 落在同一个位置，所以总览单独开一个前缀。
	overview := router.Group("/api/v1/admin/activity-overview", middleware.JWTMiddleware(jwtConfig))
	overview.GET("", guard("activities", "view"), h.overview)
}

// activityWire 是 saveRequest 的解码中转：时间字段用 FlexibleTime，
// 其余字段直接落到 Activity。这样前端 datetime-local 的分钟精度也能存下来。
type activityWire struct {
	domain.Activity
	StartAt FlexibleTime `json:"start_at"`
	EndAt   FlexibleTime `json:"end_at"`
}

func (w *activityWire) apply() domain.Activity {
	out := w.Activity
	out.StartAt = w.StartAt.Ptr()
	out.EndAt = w.EndAt.Ptr()
	return out
}

// flexibleSaveRequest 与 saveRequest 同构，但 Activity 走上面的中转。
type flexibleSaveRequest struct {
	Activity json.RawMessage       `json:"activity"`
	Rules    []domain.ActivityRule `json:"rules"`
}

// bindSaveRequest 解析保存活动的请求体，容忍 datetime-local 与 RFC3339 两种时间写法。
func bindSaveRequest(c *gin.Context) (saveRequest, error) {
	var raw flexibleSaveRequest
	if err := c.ShouldBindJSON(&raw); err != nil {
		return saveRequest{}, err
	}
	var out saveRequest
	if len(raw.Activity) > 0 {
		var wire activityWire
		if err := json.Unmarshal(raw.Activity, &wire); err != nil {
			return saveRequest{}, err
		}
		out.Activity = wire.apply()
	}
	out.Rules = raw.Rules
	return out, nil
}

type saveRequest struct {
	Activity domain.Activity       `json:"activity"`
	Rules    []domain.ActivityRule `json:"rules"`
}

func (h *Handler) list(c *gin.Context) {
	filter := domain.ListFilter{
		Status:  strings.TrimSpace(c.Query("status")),
		Type:    strings.TrimSpace(c.Query("type")),
		Search:  strings.TrimSpace(c.Query("q")),
		Keyword: strings.TrimSpace(c.Query("keyword")),
		Sort:    strings.TrimSpace(c.Query("sort")),
		Limit:   positiveInt(c.Query("limit"), 20),
		Offset:  positiveInt(c.Query("offset"), 0),
	}
	items, total, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items, "total": total, "limit": filter.Limit, "offset": filter.Offset})
}

func (h *Handler) get(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	activity, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": activity})
}

func (h *Handler) create(c *gin.Context) {
	request, err := bindSaveRequest(c)
	if err != nil {
		respondError(c, requestError(err))
		return
	}
	// 配置中心的活动默认值：新建时没填的项按店铺策略补齐，
	// 这样「默认每人限次」「默认禁止叠加」在后台改一次就对后续活动生效。
	if h.defaults != nil {
		if request.Activity.PerUserLimit <= 0 {
			request.Activity.PerUserLimit = h.defaults.PerUserLimit(c.Request.Context())
		}
		request.Activity.AllowStacking = request.Activity.AllowStacking && h.defaults.AllowStacking(c.Request.Context())
	}
	activity, err := h.service.Create(c.Request.Context(), application.SaveOptions{
		Activity:  request.Activity,
		Rules:     request.Rules,
		ActorID:   contextUserID(c),
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.Set(middleware.AuditDetailKey, "创建活动 "+activity.Name)
	c.JSON(http.StatusCreated, gin.H{"data": activity})
}

func (h *Handler) update(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	request, err := bindSaveRequest(c)
	if err != nil {
		respondError(c, requestError(err))
		return
	}
	activity, err := h.service.Update(c.Request.Context(), id, application.SaveOptions{
		Activity:  request.Activity,
		Rules:     request.Rules,
		ActorID:   contextUserID(c),
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.Set(middleware.AuditDetailKey, "更新活动 "+activity.Name)
	c.JSON(http.StatusOK, gin.H{"data": activity})
}

func (h *Handler) remove(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), id, contextUserID(c), c.ClientIP()); err != nil {
		respondError(c, err)
		return
	}
	c.Set(middleware.AuditDetailKey, "删除活动 "+strconv.FormatUint(uint64(id), 10))
	c.Status(http.StatusNoContent)
}

func (h *Handler) duplicate(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	activity, err := h.service.Duplicate(c.Request.Context(), id, contextUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	c.Set(middleware.AuditDetailKey, "复制活动 "+activity.Name)
	c.JSON(http.StatusCreated, gin.H{"data": activity})
}

type statusRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

func (h *Handler) setStatus(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var request statusRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, requestError(err))
		return
	}
	activity, err := h.service.SetStatus(c.Request.Context(), id, request.Status, contextUserID(c), c.ClientIP(), strings.TrimSpace(request.Reason))
	if err != nil {
		respondError(c, err)
		return
	}
	c.Set(middleware.AuditDetailKey, "活动 "+activity.Name+" 状态改为 "+models.ActivityStatusLabels[activity.Status])
	c.JSON(http.StatusOK, gin.H{"data": activity})
}

func (h *Handler) stats(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	stats, err := h.service.Stats(c.Request.Context(), id)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": stats})
}

func (h *Handler) overview(c *gin.Context) {
	filter := domain.ListFilter{
		Status: strings.TrimSpace(c.Query("status")),
		Type:   strings.TrimSpace(c.Query("type")),
		Search: strings.TrimSpace(c.Query("q")),
		Limit:  200,
	}
	items, total, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		respondError(c, err)
		return
	}
	var running, participants, discounts int64
	for _, item := range items {
		if item.Status == models.ActivityStatusRunning {
			running++
		}
		participants += item.UserCount
		discounts += item.DiscountTotal
	}
	c.JSON(http.StatusOK, gin.H{
		"total":        total,
		"running":      running,
		"participants": participants,
		"discounts":    discounts,
		"data":         items,
	})
}

func (h *Handler) records(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	records, total, err := h.service.Records(c.Request.Context(), id, positiveInt(c.Query("limit"), 20), positiveInt(c.Query("offset"), 0))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": records, "total": total})
}

func (h *Handler) logs(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	logs, total, err := h.service.Logs(c.Request.Context(), id, positiveInt(c.Query("limit"), 20), positiveInt(c.Query("offset"), 0))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": logs, "total": total})
}

func (h *Handler) couponRecords(c *gin.Context) {
	userID := uint(positiveInt(c.Query("user_id"), 0))
	records, total, err := h.service.CouponRecords(c.Request.Context(), userID, positiveInt(c.Query("limit"), 20), positiveInt(c.Query("offset"), 0))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": records, "total": total})
}

// ── 买家侧 ──

func (h *Handler) listPublic(c *gin.Context) {
	items, total, err := h.service.PublicActivities(c.Request.Context(), strings.TrimSpace(c.Query("type")), positiveInt(c.Query("limit"), 50))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items, "total": total})
}

func (h *Handler) getPublic(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	activity, err := h.service.PublicActivity(c.Request.Context(), id)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": activity})
}

func (h *Handler) myRecords(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录。", "code": "unauthenticated"})
		return
	}
	records, total, err := h.service.RecordsForUser(c.Request.Context(), userID, positiveInt(c.Query("limit"), 20), positiveInt(c.Query("offset"), 0))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": records, "total": total})
}

func (h *Handler) claimCoupon(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	userID := contextUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录再领取优惠券。", "code": "unauthenticated"})
		return
	}
	record, err := h.service.ClaimCoupon(c.Request.Context(), id, userID, "claim", c.ClientIP())
	if err != nil {
		respondError(c, err)
		return
	}
	c.Set(middleware.AuditDetailKey, "领取优惠券 #"+strconv.FormatUint(uint64(id), 10))
	c.JSON(http.StatusCreated, gin.H{"data": record})
}

// ── 辅助 ──

func contextUserID(c *gin.Context) uint {
	value, exists := c.Get(middleware.UserIDKey)
	if !exists {
		return 0
	}
	switch id := value.(type) {
	case uint:
		return id
	case int:
		if id > 0 {
			return uint(id)
		}
	}
	return 0
}

func currentUserID(c *gin.Context) (uint, bool) {
	id := contextUserID(c)
	return id, id > 0
}

func positiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func parseID(c *gin.Context, name string) (uint, bool) {
	value, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || value == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "地址里的编号不对，请从列表页重新进入。", "code": "invalid_id"})
		return 0, false
	}
	return uint(value), true
}

func requestError(err error) error {
	return err
}

func respondError(c *gin.Context, err error) {
	status, code, message := errorCopy(err)
	if status == http.StatusInternalServerError {
		log.Printf("[activity] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
	}
	c.JSON(status, gin.H{"error": message, "code": code})
}

func errorCopy(err error) (int, string, string) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, domain.ErrActivityNotFound):
		return http.StatusNotFound, "not_found", "这个活动不存在，或者已经被删除。"
	case errors.Is(err, domain.ErrInvalidInput):
		return http.StatusBadRequest, "invalid_input", detail(err, "活动信息填得不对，请检查后重试。")
	case errors.Is(err, domain.ErrStockExhausted):
		return http.StatusConflict, "stock_exhausted", "活动库存已经用完了。"
	case errors.Is(err, domain.ErrQuotaExhausted):
		return http.StatusConflict, "quota_exhausted", "活动名额已经用完了。"
	case errors.Is(err, domain.ErrPerUserLimit):
		return http.StatusConflict, "per_user_limit", "这次活动你已经达到参与次数上限了。"
	case errors.Is(err, domain.ErrNotRunning):
		return http.StatusConflict, "not_running", "活动还没有开始，或者已经结束了。"
	case errors.Is(err, domain.ErrNotApplicable):
		return http.StatusUnprocessableEntity, "not_applicable", "这次活动不适用于当前商品或账号。"
	default:
		return http.StatusInternalServerError, "internal_error", "活动服务暂时不可用，请稍后再试。"
	}
}

// detail 保留校验错误里的具体说明，其余情况用兜底文案。
func detail(err error, fallback string) string {
	message := strings.TrimSpace(err.Error())
	if index := strings.Index(message, ":"); index >= 0 {
		if tail := strings.TrimSpace(message[index+1:]); tail != "" {
			return tail
		}
	}
	return fallback
}
