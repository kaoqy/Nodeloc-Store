package domain

import (
	"errors"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
)

// 工单与客服模块复用 canonical persistence model。
type (
	Ticket                    = models.Ticket
	TicketMessage             = models.TicketMessage
	TicketLog                 = models.TicketLog
	TicketAttachment          = models.TicketAttachment
	TicketAISession           = models.TicketAISession
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
	ErrInvalidInput     = errors.New("invalid input")
	ErrForbidden        = errors.New("forbidden")
	ErrRateLimited      = errors.New("rate limit reached")
	ErrHumanUnavailable = errors.New("no human agent is available")
)

// 消息发送方。SenderAI 只用于展示历史行，新消息不再产生。
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
	Assignments  []CustomerServiceAssignment `json:"assignments,omitempty"`
	QuickReplies []QuickReply                `json:"quick_replies,omitempty"`
	Related      []Ticket                    `json:"related,omitempty"`
	Username     string                      `json:"username,omitempty"`
	AgentName    string                      `json:"agent_name,omitempty"`
}

// TicketStats 是工单中心顶部的统计卡。
type TicketStats struct {
	Total            int64   `json:"total"`
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
