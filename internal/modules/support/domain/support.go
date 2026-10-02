package domain

import (
	"errors"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
)

// 工单与 AI 模块复用 canonical persistence model。
type (
	Ticket                    = models.Ticket
	TicketMessage             = models.TicketMessage
	TicketLog                 = models.TicketLog
	TicketAttachment          = models.TicketAttachment
	TicketAISession           = models.TicketAISession
	AIConversation            = models.AIConversation
	AIMessage                 = models.AIMessage
	AIToolDefinition          = models.AIToolDefinition
	AIToolPermission          = models.AIToolPermission
	AIToolCall                = models.AIToolCall
	AIKnowledge               = models.AIKnowledge
	AIKnowledgeCategory       = models.AIKnowledgeCategory
	AIFeedback                = models.AIFeedback
	AIConfig                  = models.AIConfig
	AIWorkflowConfig          = models.AIWorkflowConfig
	AIQuickQuestion           = models.AIQuickQuestion
	CustomerServiceAgent      = models.CustomerServiceAgent
	CustomerServiceAssignment = models.CustomerServiceAssignment
	QuickReply                = models.QuickReply
	NotificationTemplate      = models.NotificationTemplate
	NotificationLog           = models.NotificationLog
	SystemConfig              = models.SystemConfig
	OperationLog              = models.OperationLog
)

var (
	ErrTicketNotFound   = errors.New("ticket not found")
	ErrAIDisabled       = errors.New("ai customer service is disabled")
	ErrToolDisabled     = errors.New("ai tool is disabled")
	ErrToolNotFound     = errors.New("ai tool not found")
	ErrToolForbidden    = errors.New("ai tool is not permitted")
	ErrToolConfirm      = errors.New("ai tool requires user confirmation")
	ErrToolRateLimited  = errors.New("ai tool rate limit reached")
	ErrInvalidInput     = errors.New("invalid input")
	ErrForbidden        = errors.New("forbidden")
	ErrRateLimited      = errors.New("rate limit reached")
	ErrProviderFailed   = errors.New("ai provider request failed")
	ErrNotConfigured    = errors.New("ai provider is not configured")
	ErrHumanUnavailable = errors.New("no human agent is available")
)

// 消息发送方。
const (
	SenderUser   = "user"
	SenderAI     = "ai"
	SenderAgent  = "agent"
	SenderSystem = "system"
)

// 会话渠道。
const (
	ChannelWidget = "widget"
	ChannelTicket = "ticket"
	ChannelAdmin  = "admin"
)

// 工具风险等级。
const (
	RiskLow      = "low"
	RiskMedium   = "medium"
	RiskHigh     = "high"
	RiskCritical = "critical"
)

var RiskLevels = []string{RiskLow, RiskMedium, RiskHigh, RiskCritical}

// TicketFilter 是后台工单列表的筛选条件。
type TicketFilter struct {
	Status    string
	Handler   string
	Priority  string
	Type      string
	Keyword   string
	AgentID   uint
	OnlyMine  bool
	Attention string
	Limit     int
	Offset    int
}

// TicketView 是工单列表行，带用户与客服的展示字段。
type TicketView struct {
	Ticket
	Username   string `json:"username,omitempty"`
	AgentName  string `json:"agent_name,omitempty"`
	Unread     bool   `json:"unread"`
	Overdue    bool   `json:"overdue"`
	LastSender string `json:"last_sender,omitempty"`
}

// TicketDetail 是工作台需要的完整上下文。
type TicketDetail struct {
	Ticket       `json:"ticket"`
	Messages     []TicketMessage             `json:"messages"`
	Logs         []TicketLog                 `json:"logs"`
	ToolCalls    []AIToolCall                `json:"tool_calls"`
	AISession    *TicketAISession            `json:"ai_session,omitempty"`
	Assignments  []CustomerServiceAssignment `json:"assignments,omitempty"`
	QuickReplies []QuickReply                `json:"quick_replies,omitempty"`
	Related      []Ticket                    `json:"related,omitempty"`
	Username     string                      `json:"username,omitempty"`
	AgentName    string                      `json:"agent_name,omitempty"`
}

// TicketStats 是工单中心顶部的统计卡。
type TicketStats struct {
	Total            int64   `json:"total"`
	AIProcessing     int64   `json:"ai_processing"`
	PendingHuman     int64   `json:"pending_human"`
	HumanHandling    int64   `json:"human_handling"`
	Resolved         int64   `json:"resolved"`
	Unread           int64   `json:"unread"`
	Overdue          int64   `json:"overdue"`
	Urgent           int64   `json:"urgent"`
	TodayCreated     int64   `json:"today_created"`
	SatisfactionAvg  float64 `json:"satisfaction_avg"`
	ResolveRate      float64 `json:"resolve_rate"`
	AvgHandleMinutes float64 `json:"avg_handle_minutes"`
}

// KnowledgeFilter 是知识库列表筛选。
type KnowledgeFilter struct {
	CategoryID uint
	Status     string
	Search     string
	Tag        string
	Limit      int
	Offset     int
	Sort       string
}

// ToolCallInput 是一次受控工具调用的请求。
type ToolCallInput struct {
	ToolKey        string
	UserID         uint
	UserRole       string
	ConversationID uint
	TicketID       uint
	Params         map[string]any
	Confirmed      bool
	IP             string
}

// ToolCallResult 是工具调用结果与审计摘要。
type ToolCallResult struct {
	ToolKey        string `json:"tool_key"`
	ToolName       string `json:"tool_name"`
	Status         string `json:"status"`
	Data           any    `json:"data,omitempty"`
	Summary        string `json:"summary,omitempty"`
	Error          string `json:"error,omitempty"`
	DurationMS     int64  `json:"duration_ms"`
	RequireConfirm bool   `json:"require_confirm"`
	RiskLevel      string `json:"risk_level"`
}

// KnowledgeHit 是一次知识库检索命中。
type KnowledgeHit struct {
	ID       uint    `json:"id"`
	Title    string  `json:"title"`
	Summary  string  `json:"summary,omitempty"`
	Content  string  `json:"content"`
	Priority int     `json:"priority"`
	Score    float64 `json:"score"`
}

// ChatInput 是一次 AI 对话请求。
type ChatInput struct {
	UserID      uint
	UserRole    string
	GuestKey    string
	Channel     string
	PageContext string
	Content     string
	TicketID    uint
	IP          string
	UserAgent   string
	Confirmed   bool
}

// ChatReply 是一次 AI 对话的回答。
type ChatReply struct {
	ConversationID  uint             `json:"conversation_id"`
	MessageID       uint             `json:"message_id"`
	Content         string           `json:"content"`
	ToolCalls       []ToolCallResult `json:"tool_calls,omitempty"`
	SuggestTransfer bool             `json:"suggest_transfer"`
	SuggestTicket   bool             `json:"suggest_ticket"`
	NeedConfirm     *ToolCallResult  `json:"need_confirm,omitempty"`
	TicketID        uint             `json:"ticket_id,omitempty"`
	TicketNo        string           `json:"ticket_no,omitempty"`
	KnowledgeHits   []KnowledgeHit   `json:"knowledge_hits,omitempty"`
	Fallback        bool             `json:"fallback"`
}

// TransferInput 是转人工的请求。
type TransferInput struct {
	TicketID uint
	UserID   uint
	Reason   string
	Channel  string
}

// TransferResult 是转人工的结果。
type TransferResult struct {
	Ticket          *Ticket `json:"ticket"`
	TicketNo        string  `json:"ticket_no"`
	Summary         string  `json:"summary"`
	SuggestedPlan   string  `json:"suggested_plan"`
	AgentName       string  `json:"agent_name,omitempty"`
	WorkingHours    string  `json:"working_hours,omitempty"`
	EstimateMinutes int     `json:"estimate_minutes"`
}
