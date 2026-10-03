package http

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	middleware "github.com/kaoqy/Nodeloc-Store/internal/app/httpserver"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/domain"
)

// Handler 是工单与相关配置的 HTTP 边界。
type Handler struct {
	service *application.Service
}

func NewHandler(service *application.Service) *Handler {
	if service == nil {
		panic("support: nil service")
	}
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router gin.IRouter, jwtConfig *config.JWTConfig, accounts middleware.AccountReader) {
	guard := func(resource, action string) gin.HandlerFunc {
		return middleware.RequirePermission(accounts, resource, action)
	}

	// 买家侧 AI 对话入口已下线（2026-10 计划第 1 步）：只停注册，保留 handler 代码。
	// 工单入口在下方 /api/v1/me/tickets，买家仍然可以提交并跟进人工工单。

	me := router.Group("/api/v1/me/tickets", middleware.JWTMiddleware(jwtConfig))
	me.GET("", h.myTickets)
	me.POST("", h.createTicket)
	me.GET("/:id", h.myTicket)
	me.POST("/:id/messages", h.replyTicket)
	me.POST("/:id/transfer", h.transferTicket)
	me.POST("/:id/read", h.markTicketRead)
	me.POST("/:id/rate", h.rateTicket)
	me.POST("/:id/reopen", h.reopenTicket)
	me.POST("/:id/cancel", h.cancelTicket)

	// ── 后台：工单中心 ──
	adminTickets := router.Group("/api/v1/admin/tickets", middleware.JWTMiddleware(jwtConfig))
	adminTickets.GET("", guard("tickets", "view"), h.adminList)
	adminTickets.GET("/:id", guard("tickets", "view"), h.adminDetail)
	adminTickets.POST("/:id/messages", guard("tickets", "manage"), h.adminReply)
	adminTickets.POST("/:id/status", guard("tickets", "manage"), h.adminSetStatus)
	adminTickets.POST("/:id/assign", guard("tickets", "assign"), h.adminAssign)
	adminTickets.POST("/:id/auto-assign", guard("tickets", "assign"), h.adminAutoAssign)
	adminTickets.POST("/:id/transfer", guard("tickets", "manage"), h.adminTransfer)
	adminTickets.POST("/:id/read", guard("tickets", "view"), h.adminMarkRead)
	adminTickets.GET("/:id/summary", guard("tickets", "view"), h.adminSummary)

	// 工单统计单独一个前缀，避免 /stats 与 /:id 在 Gin 里冲突。
	adminTicketStats := router.Group("/api/v1/admin/ticket-stats", middleware.JWTMiddleware(jwtConfig))
	adminTicketStats.GET("", guard("tickets", "view"), h.adminStats)

	// 后台 AI 配置、AI 工具、知识库与 AI 快捷问题入口已下线（2026-10 计划第 1 步）。
	// 按迁移方案只停注册、保留 handler 与仓储方法，观察一个版本后确认无调用再删除。
	// 表结构不动：AutoMigrate 不会删列，ai_conversations / ai_messages 先原样保留。

	// ── 后台：客服人员 ──
	agents := router.Group("/api/v1/admin/agents", middleware.JWTMiddleware(jwtConfig))
	agents.GET("", guard("agents", "view"), h.listAgents)
	agents.POST("", guard("agents", "manage"), h.saveAgent)
	agents.PUT("/:id", guard("agents", "manage"), h.updateAgent)
	agents.DELETE("/:id", guard("agents", "manage"), h.deleteAgent)

	// ── 后台：配置中心（通知模板 + 系统配置）──
	config := router.Group("/api/v1/admin/config-center", middleware.JWTMiddleware(jwtConfig))
	config.GET("/templates", guard("notification_templates", "view"), h.listTemplates)
	config.POST("/templates", guard("notification_templates", "manage"), h.saveTemplate)
	config.PUT("/templates/:id", guard("notification_templates", "manage"), h.updateTemplate)
	config.DELETE("/templates/:id", guard("notification_templates", "manage"), h.deleteTemplate)
	config.GET("/logs", guard("notification_templates", "view"), h.listNotificationLogs)
	config.GET("/system", guard("config_center", "view"), h.listSystemConfigs)
	config.PUT("/system", guard("config_center", "manage"), h.saveSystemConfig)

	// ── 后台：快捷回复 ──
	replies := router.Group("/api/v1/admin/quick-replies", middleware.JWTMiddleware(jwtConfig))
	replies.GET("", guard("quick_replies", "view"), h.listQuickReplies)
	replies.POST("", guard("quick_replies", "manage"), h.saveQuickReply)
	replies.PUT("/:id", guard("quick_replies", "manage"), h.updateQuickReply)
	replies.DELETE("/:id", guard("quick_replies", "manage"), h.deleteQuickReply)
	replies.POST("/:id/render", guard("quick_replies", "view"), h.renderQuickReply)
}

// ── 买家侧 ──

func (h *Handler) myTickets(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录。", "code": "unauthenticated"})
		return
	}
	views, total, err := h.service.ListTickets(c.Request.Context(), domain.TicketFilter{
		Keyword: strings.TrimSpace(c.Query("q")),
		Status:  strings.TrimSpace(c.Query("status")),
		Limit:   positiveInt(c.Query("limit"), 20),
		Offset:  positiveInt(c.Query("offset"), 0),
	})
	if err != nil {
		respondError(c, err)
		return
	}
	own := make([]domain.TicketView, 0, len(views))
	for _, view := range views {
		if view.UserID == userID {
			own = append(own, view)
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": own, "total": len(own), "all": total})
}

type createTicketRequest struct {
	Type     string `json:"type"`
	Subject  string `json:"subject" binding:"required"`
	Content  string `json:"content"`
	OrderNo  string `json:"order_no"`
	Priority string `json:"priority"`
	Contact  string `json:"contact"`
}

func (h *Handler) createTicket(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录。", "code": "unauthenticated"})
		return
	}
	var request createTicketRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 请填写工单标题。", domain.ErrInvalidInput))
		return
	}
	ticket, err := h.service.CreateTicket(c.Request.Context(), application.CreateTicketInput{
		UserID: userID, Type: request.Type, Subject: request.Subject,
		Content: request.Content, OrderNo: strings.TrimSpace(request.OrderNo),
		Priority: request.Priority, Source: "web", Contact: request.Contact,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": ticket})
}

func (h *Handler) myTicket(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录。", "code": "unauthenticated"})
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if _, err := h.service.RequireTicketAccess(c.Request.Context(), id, userID, "user"); err != nil {
		respondError(c, err)
		return
	}
	detail, err := h.service.TicketDetail(c.Request.Context(), id, false)
	if err != nil {
		respondError(c, err)
		return
	}
	_ = h.service.MarkUserRead(c.Request.Context(), id)
	c.JSON(http.StatusOK, gin.H{"data": detail})
}

type messageRequest struct {
	Content string `json:"content" binding:"required"`
}

func (h *Handler) replyTicket(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录。", "code": "unauthenticated"})
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var request messageRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 请填写消息内容。", domain.ErrInvalidInput))
		return
	}
	if _, err := h.service.RequireTicketAccess(c.Request.Context(), id, userID, "user"); err != nil {
		respondError(c, err)
		return
	}
	message, suggestTransfer, err := h.service.HandleTicketMessage(c.Request.Context(), id, userID, request.Content)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": message, "suggest_transfer": suggestTransfer})
}

type transferRequest struct {
	Reason string `json:"reason"`
}

func (h *Handler) transferTicket(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录。", "code": "unauthenticated"})
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var request transferRequest
	_ = c.ShouldBindJSON(&request)
	if _, err := h.service.RequireTicketAccess(c.Request.Context(), id, userID, "user"); err != nil {
		respondError(c, err)
		return
	}
	result, err := h.service.TransferToHuman(c.Request.Context(), domain.TransferInput{
		TicketID: id, UserID: userID, Reason: request.Reason, Channel: domain.ChannelTicket,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *Handler) markTicketRead(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录。", "code": "unauthenticated"})
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if _, err := h.service.RequireTicketAccess(c.Request.Context(), id, userID, "user"); err != nil {
		respondError(c, err)
		return
	}
	if err := h.service.MarkUserRead(c.Request.Context(), id); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type rateRequest struct {
	Satisfaction int    `json:"satisfaction"`
	Comment      string `json:"comment"`
}

func (h *Handler) rateTicket(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录。", "code": "unauthenticated"})
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var request rateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 评价内容无法解析。", domain.ErrInvalidInput))
		return
	}
	if !h.ratingEnabled(c) {
		respondError(c, fmt.Errorf("%w: 当前未启用工单评价。", domain.ErrInvalidInput))
		return
	}
	if request.Satisfaction < 1 || request.Satisfaction > 5 {
		respondError(c, fmt.Errorf("%w: 满意度请在 1 到 5 分之间。", domain.ErrInvalidInput))
		return
	}
	if _, err := h.service.RequireTicketAccess(c.Request.Context(), id, userID, "user"); err != nil {
		respondError(c, err)
		return
	}
	if err := h.service.RateTicket(c.Request.Context(), id, request.Satisfaction, request.Comment); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// allowReopen / allowCancel / ratingEnabled 读取配置中心的开关，
// 让「后台配置页上的选项」真的决定接口行为，而不是只存进数据库。
func (h *Handler) allowReopen(c *gin.Context) bool {
	return h.service.SystemConfigBool(c.Request.Context(), "ticket", "allow_reopen", true)
}

func (h *Handler) allowCancel(c *gin.Context) bool {
	return h.service.SystemConfigBool(c.Request.Context(), "ticket", "allow_cancel", true)
}

func (h *Handler) ratingEnabled(c *gin.Context) bool {
	return h.service.SystemConfigBool(c.Request.Context(), "ticket", "enable_rating", true)
}

func (h *Handler) reopenTicket(c *gin.Context) {
	if !h.allowReopen(c) {
		respondError(c, fmt.Errorf("%w: 当前配置不允许用户重新打开工单。", domain.ErrInvalidInput))
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录。", "code": "unauthenticated"})
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if _, err := h.service.RequireTicketAccess(c.Request.Context(), id, userID, "user"); err != nil {
		respondError(c, err)
		return
	}
	ticket, err := h.service.SetTicketStatus(c.Request.Context(), id, models.TicketStatusPendingHuman, userID, "user", "用户重新打开工单，回到人工队列")
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": ticket})
}

func (h *Handler) cancelTicket(c *gin.Context) {
	if !h.allowCancel(c) {
		respondError(c, fmt.Errorf("%w: 当前配置不允许用户撤销工单。", domain.ErrInvalidInput))
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录。", "code": "unauthenticated"})
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if _, err := h.service.RequireTicketAccess(c.Request.Context(), id, userID, "user"); err != nil {
		respondError(c, err)
		return
	}
	ticket, err := h.service.SetTicketStatus(c.Request.Context(), id, models.TicketStatusCancelled, userID, "user", "用户撤销工单")
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": ticket})
}

// ── 后台工单 ──

func (h *Handler) adminList(c *gin.Context) {
	filter := domain.TicketFilter{
		Status:    strings.TrimSpace(c.Query("status")),
		Handler:   strings.TrimSpace(c.Query("handler")),
		Priority:  strings.TrimSpace(c.Query("priority")),
		Type:      strings.TrimSpace(c.Query("type")),
		Keyword:   strings.TrimSpace(c.Query("q")),
		Attention: strings.TrimSpace(c.Query("attention")),
		Limit:     positiveInt(c.Query("limit"), 20),
		Offset:    positiveInt(c.Query("offset"), 0),
	}
	if c.Query("mine") == "1" {
		if agent, err := h.service.AgentByUser(c.Request.Context(), contextUserID(c)); err == nil && agent != nil {
			filter.AgentID = agent.ID
		}
	}
	items, total, err := h.service.ListTickets(c.Request.Context(), filter)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items, "total": total})
}

func (h *Handler) adminStats(c *gin.Context) {
	agentID := uint(0)
	if c.Query("mine") == "1" {
		if agent, err := h.service.AgentByUser(c.Request.Context(), contextUserID(c)); err == nil && agent != nil {
			agentID = agent.ID
		}
	}
	stats, err := h.service.TicketStats(c.Request.Context(), agentID)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": stats})
}

func (h *Handler) adminDetail(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if _, err := h.service.RequireTicketAccess(c.Request.Context(), id, contextUserID(c), contextRole(c)); err != nil {
		respondError(c, err)
		return
	}
	detail, err := h.service.TicketDetail(c.Request.Context(), id, true)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": detail})
}

type adminMessageRequest struct {
	Content  string `json:"content" binding:"required"`
	Internal bool   `json:"internal"`
}

func (h *Handler) adminReply(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	userID := contextUserID(c)
	if _, err := h.service.RequireTicketAccess(c.Request.Context(), id, userID, contextRole(c)); err != nil {
		respondError(c, err)
		return
	}
	var request adminMessageRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 请填写回复内容。", domain.ErrInvalidInput))
		return
	}
	message, err := h.service.AddMessage(c.Request.Context(), id, domain.SenderAgent, userID, "", request.Content, request.Internal)
	if err != nil {
		respondError(c, err)
		return
	}
	c.Set(middleware.AuditDetailKey, "回复工单 #"+strconv.FormatUint(uint64(id), 10))
	c.JSON(http.StatusCreated, gin.H{"data": message})
}

type adminStatusRequest struct {
	Status string `json:"status" binding:"required"`
	Detail string `json:"detail"`
}

func (h *Handler) adminSetStatus(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var request adminStatusRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 请选择工单状态。", domain.ErrInvalidInput))
		return
	}
	ticket, err := h.service.SetTicketStatus(c.Request.Context(), id, request.Status, contextUserID(c), "agent", request.Detail)
	if err != nil {
		respondError(c, err)
		return
	}
	c.Set(middleware.AuditDetailKey, "工单 "+ticket.TicketNo+" 状态改为 "+models.TicketStatusLabels[ticket.Status])
	c.JSON(http.StatusOK, gin.H{"data": ticket})
}

type assignRequest struct {
	AgentID uint `json:"agent_id"`
}

func (h *Handler) adminAssign(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var request assignRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.AgentID == 0 {
		respondError(c, fmt.Errorf("%w: 请选择客服。", domain.ErrInvalidInput))
		return
	}
	ticket, err := h.service.AssignAgent(c.Request.Context(), id, request.AgentID, contextUserID(c), "manual")
	if err != nil {
		respondError(c, err)
		return
	}
	c.Set(middleware.AuditDetailKey, "工单 "+ticket.TicketNo+" 指派客服")
	c.JSON(http.StatusOK, gin.H{"data": ticket})
}

func (h *Handler) adminAutoAssign(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	ticket, err := h.service.AutoAssign(c.Request.Context(), id, contextUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": ticket})
}

func (h *Handler) adminTransfer(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var request transferRequest
	_ = c.ShouldBindJSON(&request)
	ticket, err := h.service.SetTicketStatus(c.Request.Context(), id, models.TicketStatusPendingHuman, contextUserID(c), "agent", request.Reason)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": ticket})
}

func (h *Handler) adminMarkRead(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.MarkTicketRead(c.Request.Context(), id); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) adminSummary(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	summary, plan, err := h.service.SummaryForTicket(c.Request.Context(), id)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"summary": summary, "suggested_plan": plan})
}

// ── 后台 AI 配置 ──

func (h *Handler) listAgents(c *gin.Context) {
	agents, err := h.service.Agents(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": agents})
}

func (h *Handler) saveAgent(c *gin.Context) {
	var request domain.CustomerServiceAgent
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 客服配置无法解析。", domain.ErrInvalidInput))
		return
	}
	agent, err := h.service.SaveAgent(c.Request.Context(), request)
	if err != nil {
		respondError(c, err)
		return
	}
	c.Set(middleware.AuditDetailKey, "保存客服配置")
	c.JSON(http.StatusCreated, gin.H{"data": agent})
}

func (h *Handler) updateAgent(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var request domain.CustomerServiceAgent
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 客服配置无法解析。", domain.ErrInvalidInput))
		return
	}
	request.ID = id
	agent, err := h.service.SaveAgent(c.Request.Context(), request)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": agent})
}

func (h *Handler) deleteAgent(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteAgent(c.Request.Context(), id); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ── 快捷回复 ──

func (h *Handler) listQuickReplies(c *gin.Context) {
	replies, err := h.service.QuickReplies(c.Request.Context(), false,
		strings.TrimSpace(c.Query("ticket_type")), strings.TrimSpace(c.Query("role")))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": replies})
}

func (h *Handler) saveQuickReply(c *gin.Context) {
	var request domain.QuickReply
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 快捷回复无法解析。", domain.ErrInvalidInput))
		return
	}
	reply, err := h.service.SaveQuickReply(c.Request.Context(), request)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": reply})
}

func (h *Handler) updateQuickReply(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var request domain.QuickReply
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 快捷回复无法解析。", domain.ErrInvalidInput))
		return
	}
	request.ID = id
	reply, err := h.service.SaveQuickReply(c.Request.Context(), request)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": reply})
}

func (h *Handler) deleteQuickReply(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteQuickReply(c.Request.Context(), id); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type renderRequest struct {
	Variables map[string]string `json:"variables"`
}

func (h *Handler) renderQuickReply(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var request renderRequest
	_ = c.ShouldBindJSON(&request)
	content, err := h.service.RenderQuickReply(c.Request.Context(), id, request.Variables)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"content": content})
}

// ── 配置中心 ──

func (h *Handler) listTemplates(c *gin.Context) {
	templates, err := h.service.NotificationTemplates(c.Request.Context(), strings.TrimSpace(c.Query("category")))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": templates})
}

func (h *Handler) saveTemplate(c *gin.Context) {
	var request domain.NotificationTemplate
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 模板内容无法解析。", domain.ErrInvalidInput))
		return
	}
	template, err := h.service.SaveNotificationTemplate(c.Request.Context(), request)
	if err != nil {
		respondError(c, err)
		return
	}
	c.Set(middleware.AuditDetailKey, "保存通知模板 "+template.Name)
	c.JSON(http.StatusCreated, gin.H{"data": template})
}

func (h *Handler) updateTemplate(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var request domain.NotificationTemplate
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 模板内容无法解析。", domain.ErrInvalidInput))
		return
	}
	request.ID = id
	template, err := h.service.SaveNotificationTemplate(c.Request.Context(), request)
	if err != nil {
		respondError(c, err)
		return
	}
	c.Set(middleware.AuditDetailKey, "更新通知模板 "+template.Name)
	c.JSON(http.StatusOK, gin.H{"data": template})
}

func (h *Handler) deleteTemplate(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteNotificationTemplate(c.Request.Context(), id); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) listNotificationLogs(c *gin.Context) {
	logs, total, err := h.service.NotificationLogs(c.Request.Context(),
		strings.TrimSpace(c.Query("status")), positiveInt(c.Query("limit"), 50), positiveInt(c.Query("offset"), 0))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": logs, "total": total})
}

func (h *Handler) listSystemConfigs(c *gin.Context) {
	configs, err := h.service.SystemConfigs(c.Request.Context(), strings.TrimSpace(c.Query("group")))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": configs})
}

func (h *Handler) saveSystemConfig(c *gin.Context) {
	var request domain.SystemConfig
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 配置内容无法解析。", domain.ErrInvalidInput))
		return
	}
	config, err := h.service.SaveSystemConfig(c.Request.Context(), request, contextUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	c.Set(middleware.AuditDetailKey, "更新系统配置 "+config.Group+"/"+config.Key)
	c.JSON(http.StatusOK, gin.H{"data": config})
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

func contextRole(c *gin.Context) string {
	value, exists := c.Get(middleware.UserRoleKey)
	if !exists {
		return ""
	}
	role, _ := value.(string)
	return role
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

func respondError(c *gin.Context, err error) {
	status, code, message := errorCopy(err)
	if status == http.StatusInternalServerError {
		log.Printf("[support] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
	}
	c.JSON(status, gin.H{"error": message, "code": code})
}

func errorCopy(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrTicketNotFound):
		return http.StatusNotFound, "ticket_not_found", "这张工单不存在，或者已经被删除。"
	case errors.Is(err, domain.ErrInvalidInput):
		return http.StatusBadRequest, "invalid_input", detail(err, "信息填得不对，请检查后重试。")
	case errors.Is(err, domain.ErrRateLimited):
		return http.StatusTooManyRequests, "rate_limited", "操作太频繁了，请稍后再试。"
	case errors.Is(err, domain.ErrHumanUnavailable):
		return http.StatusServiceUnavailable, "human_unavailable", "现在没有空闲客服，工单已经排在待人工队列里。"
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden, "forbidden", "你没有权限查看或处理这条内容。"
	default:
		return http.StatusInternalServerError, "internal_error", "客服服务暂时不可用，请稍后再试。"
	}
}

func detail(err error, fallback string) string {
	message := strings.TrimSpace(err.Error())
	if index := strings.Index(message, ":"); index >= 0 {
		if tail := strings.TrimSpace(message[index+1:]); tail != "" {
			return tail
		}
	}
	return fallback
}
