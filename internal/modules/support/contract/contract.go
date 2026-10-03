package contract

import (
	"context"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/domain"
)

// Repository 是工单与客服模块的持久化端口。
type Repository interface {
	// ── 工单 ──
	ListTickets(ctx context.Context, filter domain.TicketFilter) ([]domain.TicketView, int64, error)
	GetTicket(ctx context.Context, id uint) (*domain.Ticket, error)
	GetTicketByNo(ctx context.Context, no string) (*domain.Ticket, error)
	CreateTicket(ctx context.Context, ticket *domain.Ticket) error
	UpdateTicket(ctx context.Context, ticket *domain.Ticket) error
	DeleteTicket(ctx context.Context, id uint) error
	NextTicketNo(ctx context.Context, prefix string) (string, error)

	ListMessages(ctx context.Context, ticketID uint, includeInternal bool, limit, offset int) ([]domain.TicketMessage, int64, error)
	CreateMessage(ctx context.Context, message *domain.TicketMessage) error
	UpdateMessage(ctx context.Context, message *domain.TicketMessage) error

	AppendTicketLog(ctx context.Context, entry *domain.TicketLog) error
	ListTicketLogs(ctx context.Context, ticketID uint, limit, offset int) ([]domain.TicketLog, int64, error)

	CreateAttachment(ctx context.Context, attachment *domain.TicketAttachment) error
	ListAttachments(ctx context.Context, ticketID uint) ([]domain.TicketAttachment, error)

	TicketStats(ctx context.Context, agentID uint) (*domain.TicketStats, error)
	RelatedTickets(ctx context.Context, userID, excludeID uint, limit int) ([]domain.Ticket, error)

	// ── 客服 ──
	ListAgents(ctx context.Context, activeOnly bool) ([]domain.CustomerServiceAgent, error)
	GetAgent(ctx context.Context, id uint) (*domain.CustomerServiceAgent, error)
	GetAgentByUser(ctx context.Context, userID uint) (*domain.CustomerServiceAgent, error)
	SaveAgent(ctx context.Context, agent *domain.CustomerServiceAgent) error
	DeleteAgent(ctx context.Context, id uint) error
	AgentLoad(ctx context.Context, agentID uint) (int64, error)
	CreateAssignment(ctx context.Context, assignment *domain.CustomerServiceAssignment) error
	ListAssignments(ctx context.Context, ticketID uint) ([]domain.CustomerServiceAssignment, error)

	// ── 通知模板与系统配置（统一配置中心）──
	CreateNotificationLog(ctx context.Context, entry *domain.NotificationLog) error
	ListNotificationLogs(ctx context.Context, status string, limit, offset int) ([]domain.NotificationLog, int64, error)
	ListNotificationTemplates(ctx context.Context, category string) ([]domain.NotificationTemplate, error)
	SaveNotificationTemplate(ctx context.Context, template *domain.NotificationTemplate) error
	DeleteNotificationTemplate(ctx context.Context, id uint) error
	ListSystemConfigs(ctx context.Context, group string) ([]domain.SystemConfig, error)
	SaveSystemConfig(ctx context.Context, config *domain.SystemConfig) error
	DeleteSystemConfig(ctx context.Context, group, key string) error

	// ── 快捷回复 ──
	ListQuickReplies(ctx context.Context, enabledOnly bool, ticketType, role string) ([]domain.QuickReply, error)
	SaveQuickReply(ctx context.Context, reply *domain.QuickReply) error
	DeleteQuickReply(ctx context.Context, id uint) error
	IncrementQuickReplyUse(ctx context.Context, id uint) error
}

// Notifier 是通知模块暴露给工单的发送能力：只能给工单关联人发站内通知。
type Notifier interface {
	NotifyUser(ctx context.Context, userID uint, notificationType, title, content, link string) error
}

// MailSender 是站外提醒能力，配置关闭时实现方直接返回 nil。
//
// SendToUser 发给通知的关联买家；SendToAddress 发给模板里写死的收件地址，
// 让「订单异常发到老板邮箱」这类提醒不必再为每个人单独建模板。
type MailSender interface {
	SendToUser(ctx context.Context, userID uint, subject, body string) error
	SendToAddress(ctx context.Context, address, subject, body string) error
}

// StaffRecipients 解析提醒事件里的 recipients=staff：管理端员工的站内账号
// 与邮箱。两者分开是因为站内通知按用户 ID 投递，邮件按地址投递。
type StaffRecipients interface {
	UserIDs(ctx context.Context) ([]uint, error)
	Emails(ctx context.Context) ([]string, error)
}
