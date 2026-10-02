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

// Handler 是工单、AI 客服与相关配置的 HTTP 边界。
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

	// ── 买家侧：AI 客服与工单 ──
	chat := router.Group("/api/v1/support", middleware.OptionalJWTMiddleware(jwtConfig))
	chat.GET("/config", h.publicConfig)
	chat.GET("/quick-questions", h.publicQuickQuestions)
	chat.POST("/chat", h.chat)
	chat.GET("/conversations/:id", h.conversationHistory)
	chat.POST("/feedback", h.feedback)

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

	// ── 后台：AI 客服配置 ──
	adminAI := router.Group("/api/v1/admin/ai", middleware.JWTMiddleware(jwtConfig))
	adminAI.GET("/config", guard("ai", "view"), h.getConfig)
	adminAI.PUT("/config", guard("ai", "manage"), h.saveConfig)
	adminAI.PUT("/workflow", guard("ai", "manage"), h.saveWorkflow)
	adminAI.GET("/conversations", guard("ai", "view"), h.adminConversations)
	adminAI.GET("/feedback", guard("ai", "view"), h.adminFeedback)

	// ── 后台：AI 工具 ──
	tools := router.Group("/api/v1/admin/ai-tools", middleware.JWTMiddleware(jwtConfig))
	tools.PUT("/:id", guard("ai_tools", "manage"), h.saveTool)
	tools.POST("/:id/permissions", guard("ai_tools", "manage"), h.setToolPermission)
	// 工具清单与调用日志放在独立前缀，避免与 :id 通配冲突。
	toolsIndex := router.Group("/api/v1/admin/ai-tool-index", middleware.JWTMiddleware(jwtConfig))
	toolsIndex.GET("", guard("ai_tools", "view"), h.listTools)
	toolsIndex.GET("/calls", guard("ai_tools", "view"), h.toolCalls)
	toolsIndex.GET("/roles", guard("ai_tools", "view"), h.toolRoles)

	// ── 后台：知识库 ──
	kb := router.Group("/api/v1/admin/knowledge", middleware.JWTMiddleware(jwtConfig))
	kb.GET("/:id", guard("knowledge", "view"), h.getKnowledge)
	kb.PUT("/:id", guard("knowledge", "manage"), h.updateKnowledge)
	kb.DELETE("/:id", guard("knowledge", "manage"), h.deleteKnowledge)
	kb.POST("/:id/test", guard("knowledge", "view"), h.testKnowledge)
	// 分类、导入导出走独立前缀，路径不再与 :id 竞争。
	kbIndex := router.Group("/api/v1/admin/knowledge-index", middleware.JWTMiddleware(jwtConfig))
	kbIndex.GET("", guard("knowledge", "view"), h.listKnowledge)
	kbIndex.POST("", guard("knowledge", "manage"), h.saveKnowledge)
	kbIndex.GET("/categories", guard("knowledge", "view"), h.listKnowledgeCategories)
	kbIndex.POST("/categories", guard("knowledge", "manage"), h.saveKnowledgeCategory)
	kbIndex.DELETE("/categories/:id", guard("knowledge", "manage"), h.deleteKnowledgeCategory)
	kbIndex.POST("/import", guard("knowledge", "manage"), h.importKnowledge)
	kbIndex.GET("/export", guard("knowledge", "manage"), h.exportKnowledge)

	// ── 后台：快捷问题 ──
	quick := router.Group("/api/v1/admin/ai/quick-questions", middleware.JWTMiddleware(jwtConfig))
	quick.GET("", guard("ai", "view"), h.listQuickQuestions)
	quick.POST("", guard("ai", "manage"), h.saveQuickQuestion)
	quick.PUT("/:id", guard("ai", "manage"), h.updateQuickQuestion)
	quick.DELETE("/:id", guard("ai", "manage"), h.deleteQuickQuestion)

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

func (h *Handler) publicConfig(c *gin.Context) {
	config, err := h.service.Config(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	workflow, _ := h.service.Workflow(c.Request.Context())
	enabled := config.IsEnabled
	c.JSON(http.StatusOK, gin.H{
		"enabled": enabled, "agent_name": config.AgentName, "avatar": config.Avatar,
		"greeting": config.Greeting, "guest_allowed": config.GuestAllowed,
		"rating_enabled": config.RatingEnabled,
		"working_hours": func() string {
			if workflow == nil {
				return ""
			}
			return workflow.WorkingHours
		}(),
		"estimate_minutes": func() int {
			if workflow == nil {
				return 30
			}
			return workflow.EstimateReplyMinutes
		}(),
	})
}

func (h *Handler) publicQuickQuestions(c *gin.Context) {
	userID := contextUserID(c)
	position := strings.TrimSpace(c.Query("position"))
	questions, err := h.service.QuickQuestions(c.Request.Context(), true, position)
	if err != nil {
		respondError(c, err)
		return
	}
	out := make([]domain.AIQuickQuestion, 0, len(questions))
	for _, question := range questions {
		if question.RequireLogin && userID == 0 {
			continue
		}
		out = append(out, question)
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

type chatRequest struct {
	Content        string `json:"content" binding:"required"`
	ConversationID uint   `json:"conversation_id"`
	TicketID       uint   `json:"ticket_id"`
	PageContext    string `json:"page_context"`
}

func (h *Handler) chat(c *gin.Context) {
	var request chatRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 请先输入问题。", domain.ErrInvalidInput))
		return
	}
	userID := contextUserID(c)
	if userID == 0 {
		// 游客需要一个稳定的标识来做每日次数限制，用 IP 的散列即可。
		request.PageContext = strings.TrimSpace(request.PageContext)
	}
	reply, err := h.service.Chat(c.Request.Context(), domain.ChatInput{
		UserID: userID, UserRole: contextRole(c), Channel: domain.ChannelWidget,
		PageContext: request.PageContext, Content: request.Content,
		TicketID: request.TicketID, IP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, reply)
}

func (h *Handler) conversationHistory(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	messages, err := h.service.History(c.Request.Context(), id, contextUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": messages})
}

type feedbackRequest struct {
	ConversationID uint   `json:"conversation_id" binding:"required"`
	MessageID      uint   `json:"message_id"`
	TicketID       uint   `json:"ticket_id"`
	Rating         int    `json:"rating"`
	Reason         string `json:"reason"`
	Comment        string `json:"comment"`
}

func (h *Handler) feedback(c *gin.Context) {
	var request feedbackRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 评价内容无法解析。", domain.ErrInvalidInput))
		return
	}
	if err := h.service.Feedback(c.Request.Context(), application.ChatFeedbackInput{
		ConversationID: request.ConversationID, MessageID: request.MessageID,
		TicketID: request.TicketID, UserID: contextUserID(c),
		Rating: request.Rating, Reason: request.Reason, Comment: request.Comment,
	}); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ── 买家工单 ──

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

func (h *Handler) reopenTicket(c *gin.Context) {
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
	ticket, err := h.service.SetTicketStatus(c.Request.Context(), id, models.TicketStatusAIProcessing, userID, "user", "用户重新打开工单")
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": ticket})
}

func (h *Handler) cancelTicket(c *gin.Context) {
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

func (h *Handler) getConfig(c *gin.Context) {
	payload, err := h.service.ConfigForAdmin(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, payload)
}

func (h *Handler) saveConfig(c *gin.Context) {
	var request domain.AIConfig
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 配置内容无法解析。", domain.ErrInvalidInput))
		return
	}
	key := request.APIKeyEnc
	request.APIKeyEnc = strings.TrimSpace(key)
	config, err := h.service.SaveConfig(c.Request.Context(), request)
	if err != nil {
		respondError(c, err)
		return
	}
	c.Set(middleware.AuditDetailKey, "更新 AI 客服配置")
	c.JSON(http.StatusOK, gin.H{"config": config, "has_key": config != nil && config.APIKeyEnc == "__saved__"})
}

func (h *Handler) saveWorkflow(c *gin.Context) {
	var request domain.AIWorkflowConfig
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 配置内容无法解析。", domain.ErrInvalidInput))
		return
	}
	config, err := h.service.SaveWorkflow(c.Request.Context(), request)
	if err != nil {
		respondError(c, err)
		return
	}
	c.Set(middleware.AuditDetailKey, "更新 AI 工作流配置")
	c.JSON(http.StatusOK, gin.H{"workflow": config})
}

func (h *Handler) adminConversations(c *gin.Context) {
	userID := uint(positiveInt(c.Query("user_id"), 0))
	conversations, total, err := h.service.Conversations(c.Request.Context(), userID, positiveInt(c.Query("limit"), 20), positiveInt(c.Query("offset"), 0))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": conversations, "total": total})
}

func (h *Handler) adminFeedback(c *gin.Context) {
	items, err := h.service.FeedbackList(c.Request.Context(), positiveInt(c.Query("limit"), 50))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

// ── AI 工具 ──

func (h *Handler) listTools(c *gin.Context) {
	tools, err := h.service.ToolList(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": tools})
}

func (h *Handler) saveTool(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var request domain.AIToolDefinition
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 工具配置无法解析。", domain.ErrInvalidInput))
		return
	}
	request.ID = id
	tool, err := h.service.SaveTool(c.Request.Context(), request, contextUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	c.Set(middleware.AuditDetailKey, "更新 AI 工具 "+tool.Name)
	c.JSON(http.StatusOK, gin.H{"data": tool})
}

type toolPermissionRequest struct {
	Role    string `json:"role" binding:"required"`
	Allowed bool   `json:"allowed"`
}

func (h *Handler) setToolPermission(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var request toolPermissionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 请选择角色。", domain.ErrInvalidInput))
		return
	}
	if err := h.service.SetToolPermission(c.Request.Context(), id, request.Role, request.Allowed, contextUserID(c)); err != nil {
		respondError(c, err)
		return
	}
	c.Set(middleware.AuditDetailKey, "调整 AI 工具权限 "+request.Role)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) toolCalls(c *gin.Context) {
	calls, total, err := h.service.ToolCalls(c.Request.Context(), application.ToolCallFilterInput{
		ToolKey: strings.TrimSpace(c.Query("tool")),
		Status:  strings.TrimSpace(c.Query("status")),
		UserID:  uint(positiveInt(c.Query("user_id"), 0)),
		Limit:   positiveInt(c.Query("limit"), 50),
		Offset:  positiveInt(c.Query("offset"), 0),
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": calls, "total": total})
}

func (h *Handler) toolRoles(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"data": application.SortedRoleList()})
}

// ── 知识库 ──

func (h *Handler) listKnowledge(c *gin.Context) {
	items, total, err := h.service.KnowledgeList(c.Request.Context(), domain.KnowledgeFilter{
		CategoryID: uint(positiveInt(c.Query("category_id"), 0)),
		Status:     strings.TrimSpace(c.Query("status")),
		Search:     strings.TrimSpace(c.Query("q")),
		Tag:        strings.TrimSpace(c.Query("tag")),
		Sort:       strings.TrimSpace(c.Query("sort")),
		Limit:      positiveInt(c.Query("limit"), 20),
		Offset:     positiveInt(c.Query("offset"), 0),
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items, "total": total})
}

func (h *Handler) getKnowledge(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	article, err := h.service.KnowledgeGet(c.Request.Context(), id)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": article})
}

func (h *Handler) saveKnowledge(c *gin.Context) {
	var request domain.AIKnowledge
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 文章内容无法解析。", domain.ErrInvalidInput))
		return
	}
	article, err := h.service.SaveKnowledge(c.Request.Context(), request, contextUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	c.Set(middleware.AuditDetailKey, "保存知识库文章 "+article.Title)
	c.JSON(http.StatusCreated, gin.H{"data": article})
}

func (h *Handler) updateKnowledge(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var request domain.AIKnowledge
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 文章内容无法解析。", domain.ErrInvalidInput))
		return
	}
	request.ID = id
	article, err := h.service.SaveKnowledge(c.Request.Context(), request, contextUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	c.Set(middleware.AuditDetailKey, "更新知识库文章 "+article.Title)
	c.JSON(http.StatusOK, gin.H{"data": article})
}

func (h *Handler) deleteKnowledge(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteKnowledge(c.Request.Context(), id); err != nil {
		respondError(c, err)
		return
	}
	c.Set(middleware.AuditDetailKey, "删除知识库文章 #"+strconv.FormatUint(uint64(id), 10))
	c.Status(http.StatusNoContent)
}

func (h *Handler) listKnowledgeCategories(c *gin.Context) {
	categories, err := h.service.KnowledgeCategories(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": categories})
}

func (h *Handler) saveKnowledgeCategory(c *gin.Context) {
	var request domain.AIKnowledgeCategory
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 分类内容无法解析。", domain.ErrInvalidInput))
		return
	}
	category, err := h.service.SaveKnowledgeCategory(c.Request.Context(), request)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": category})
}

func (h *Handler) deleteKnowledgeCategory(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteKnowledgeCategory(c.Request.Context(), id); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type importRequest struct {
	CategoryID uint   `json:"category_id"`
	Text       string `json:"text" binding:"required"`
}

func (h *Handler) importKnowledge(c *gin.Context) {
	var request importRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 导入内容无法解析。", domain.ErrInvalidInput))
		return
	}
	count, err := h.service.ImportKnowledge(c.Request.Context(), request.CategoryID, request.Text, contextUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"imported": count})
}

func (h *Handler) exportKnowledge(c *gin.Context) {
	text, err := h.service.ExportKnowledge(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=knowledge.txt")
	c.String(http.StatusOK, text)
}

type testKnowledgeRequest struct {
	Question string `json:"question"`
}

func (h *Handler) testKnowledge(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var request testKnowledgeRequest
	_ = c.ShouldBindJSON(&request)
	result, err := h.service.TestKnowledge(c.Request.Context(), id, request.Question)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// ── 快捷问题 ──

func (h *Handler) listQuickQuestions(c *gin.Context) {
	questions, err := h.service.QuickQuestions(c.Request.Context(), false, "")
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": questions})
}

func (h *Handler) saveQuickQuestion(c *gin.Context) {
	var request domain.AIQuickQuestion
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 快捷问题内容无法解析。", domain.ErrInvalidInput))
		return
	}
	question, err := h.service.SaveQuickQuestion(c.Request.Context(), request)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": question})
}

func (h *Handler) updateQuickQuestion(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var request domain.AIQuickQuestion
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, fmt.Errorf("%w: 快捷问题内容无法解析。", domain.ErrInvalidInput))
		return
	}
	request.ID = id
	question, err := h.service.SaveQuickQuestion(c.Request.Context(), request)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": question})
}

func (h *Handler) deleteQuickQuestion(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteQuickQuestion(c.Request.Context(), id); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ── 客服人员 ──

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
	case errors.Is(err, domain.ErrAIDisabled):
		return http.StatusServiceUnavailable, "ai_disabled", "智能客服当前未启用，你可以提交工单，客服会人工跟进。"
	case errors.Is(err, domain.ErrNotConfigured):
		return http.StatusServiceUnavailable, "not_configured", "AI 服务还没有配置好，请先联系管理员。"
	case errors.Is(err, domain.ErrProviderFailed):
		return http.StatusBadGateway, "provider_failed", "AI 服务暂时不可用，请稍后再试或转人工。"
	case errors.Is(err, domain.ErrToolDisabled), errors.Is(err, domain.ErrToolForbidden):
		return http.StatusForbidden, "tool_forbidden", "这个操作需要人工客服在后台处理。"
	case errors.Is(err, domain.ErrToolConfirm):
		return http.StatusConflict, "need_confirm", "这个操作需要你确认后才能继续。"
	case errors.Is(err, domain.ErrToolRateLimited), errors.Is(err, domain.ErrRateLimited):
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
