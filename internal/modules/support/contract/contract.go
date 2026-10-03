package contract

import (
	"context"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/domain"
)

// Repository 是工单与 AI 模块的持久化端口。
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

	// ── AI 会话与消息 ──
	CreateConversation(ctx context.Context, conversation *domain.AIConversation) error
	UpdateConversation(ctx context.Context, conversation *domain.AIConversation) error
	GetConversation(ctx context.Context, id uint) (*domain.AIConversation, error)
	ListConversations(ctx context.Context, userID uint, limit, offset int) ([]domain.AIConversation, int64, error)
	ListMessagesByConversation(ctx context.Context, conversationID uint, limit int) ([]domain.AIMessage, error)
	CreateAIMessage(ctx context.Context, message *domain.AIMessage) error

	// ── 工具 ──
	ListTools(ctx context.Context, enabledOnly bool) ([]domain.AIToolDefinition, error)
	GetTool(ctx context.Context, key string) (*domain.AIToolDefinition, error)
	SaveTool(ctx context.Context, tool *domain.AIToolDefinition) error
	DeleteTool(ctx context.Context, id uint) error
	ListToolPermissions(ctx context.Context) ([]domain.AIToolPermission, error)
	SetToolPermission(ctx context.Context, permission *domain.AIToolPermission) error

	CreateToolCall(ctx context.Context, call *domain.AIToolCall) error
	ListToolCalls(ctx context.Context, filter ToolCallFilter) ([]domain.AIToolCall, int64, error)
	CountToolCalls(ctx context.Context, toolKey string, userID uint, since time.Time) (int64, error)

	// ── 知识库 ──
	ListKnowledge(ctx context.Context, filter domain.KnowledgeFilter) ([]domain.AIKnowledge, int64, error)
	GetKnowledge(ctx context.Context, id uint) (*domain.AIKnowledge, error)
	GetKnowledgeBySlug(ctx context.Context, slug string) (*domain.AIKnowledge, error)
	CreateKnowledge(ctx context.Context, article *domain.AIKnowledge) error
	UpdateKnowledge(ctx context.Context, article *domain.AIKnowledge) error
	DeleteKnowledge(ctx context.Context, id uint) error
	SearchKnowledge(ctx context.Context, query string, limit int) ([]domain.KnowledgeHit, error)
	ListKnowledgeCategories(ctx context.Context) ([]domain.AIKnowledgeCategory, error)
	CreateKnowledgeCategory(ctx context.Context, category *domain.AIKnowledgeCategory) error
	UpdateKnowledgeCategory(ctx context.Context, category *domain.AIKnowledgeCategory) error
	DeleteKnowledgeCategory(ctx context.Context, id uint) error

	// ── 配置 ──
	GetAIConfig(ctx context.Context) (*domain.AIConfig, error)
	SaveAIConfig(ctx context.Context, config *domain.AIConfig) error
	GetAIWorkflow(ctx context.Context) (*domain.AIWorkflowConfig, error)
	SaveAIWorkflow(ctx context.Context, config *domain.AIWorkflowConfig) error

	ListQuickQuestions(ctx context.Context, enabledOnly bool, position string) ([]domain.AIQuickQuestion, error)
	SaveQuickQuestion(ctx context.Context, question *domain.AIQuickQuestion) error
	DeleteQuickQuestion(ctx context.Context, id uint) error

	CreateFeedback(ctx context.Context, feedback *domain.AIFeedback) error
	ListFeedback(ctx context.Context, limit int) ([]domain.AIFeedback, error)
	FeedbackCount(ctx context.Context, conversationID uint, rating int, since time.Time) (int64, error)

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

	// ── 快捷回复 ──
	ListQuickReplies(ctx context.Context, enabledOnly bool, ticketType, role string) ([]domain.QuickReply, error)
	SaveQuickReply(ctx context.Context, reply *domain.QuickReply) error
	DeleteQuickReply(ctx context.Context, id uint) error
	IncrementQuickReplyUse(ctx context.Context, id uint) error
}

// ToolCallFilter 是工具调用日志的筛选条件。
type ToolCallFilter struct {
	ToolKey string
	Status  string
	UserID  uint
	Limit   int
	Offset  int
}

// OrderContext 是 AI 查询订单时需要的订单信息。数据由支付模块提供，
// 保证 AI 只看到被授权的字段，不直接读数据库。
type OrderContext struct {
	OrderNo           string     `json:"order_no"`
	ProductName       string     `json:"product_name"`
	Quantity          int        `json:"quantity"`
	TotalAmount       int        `json:"total_amount"`
	Status            string     `json:"status"`
	FulfillmentStatus string     `json:"fulfillment_status"`
	PaidAt            *time.Time `json:"paid_at,omitempty"`
	DeliveredAt       *time.Time `json:"delivered_at,omitempty"`
	// DeliverySummary 只给出交付内容的摘要，完整卡密不会交给 AI。
	DeliverySummary string `json:"delivery_summary,omitempty"`
}

// OrderReader 是支付模块暴露给 AI 的只读订单能力。
type OrderReader interface {
	// OrdersForUser 只返回该用户自己的订单。
	OrdersForUser(ctx context.Context, userID uint, limit int) ([]OrderContext, error)
	// OrderForUser 在订单不属于该用户时返回 nil，越权查询因此拿不到数据。
	OrderForUser(ctx context.Context, userID uint, orderNo string) (*OrderContext, error)
}

// UserContext 是 AI 查询用户资料时的安全字段。
type UserContext struct {
	ID         uint      `json:"id"`
	Username   string    `json:"username"`
	Nickname   string    `json:"nickname,omitempty"`
	Role       string    `json:"role"`
	IsActive   bool      `json:"is_active"`
	HasEmail   bool      `json:"has_email"`
	OAuthBound bool      `json:"oauth_bound"`
	Points     int       `json:"points"`
	CreatedAt  time.Time `json:"created_at"`
}

// UserReader 是身份模块暴露给 AI 的只读用户能力。
type UserReader interface {
	Context(ctx context.Context, userID uint) (*UserContext, error)
}

// CatalogReader 是商品模块暴露给 AI 的只读商品能力。
type CatalogReader interface {
	ProductFacts(ctx context.Context, limit int) ([]ProductFact, error)
	ProductFact(ctx context.Context, productID uint) (*ProductFact, error)
}

// ProductFact 是 AI 可以介绍的公开商品信息，不含任何内部字段。
type ProductFact struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Summary   string `json:"summary,omitempty"`
	Price     int    `json:"price"`
	Stock     int    `json:"stock"`
	Category  string `json:"category,omitempty"`
	Published bool   `json:"published"`
}

// ActivityFact 是 AI 可以介绍的活动信息。
type ActivityFact struct {
	ID       uint       `json:"id"`
	Name     string     `json:"name"`
	Subtitle string     `json:"subtitle,omitempty"`
	Type     string     `json:"type"`
	TypeName string     `json:"type_name,omitempty"`
	StartAt  *time.Time `json:"start_at,omitempty"`
	EndAt    *time.Time `json:"end_at,omitempty"`
}

// ActivityReader 是活动模块暴露给 AI 的只读活动能力。
type ActivityReader interface {
	ActiveActivities(ctx context.Context, limit int) ([]ActivityFact, error)
}

// Refunder 是支付模块暴露给 AI 的退款能力。
//
// 只有「已支付且属于当前用户」的订单会被接受：实现方自己校验归属与状态，
// 并把退款走 NodeLoc 原路退回。AI 不接触金额，只传订单号。
type Refunder interface {
	// RefundOrderForUser 退款一张属于该用户的订单。订单不属于他时返回错误，
	// 越权退款因此不可能发生。
	RefundOrderForUser(ctx context.Context, userID uint, orderNo string) (amount int, status string, err error)
	// RefundableOrders 列出该用户当前可退款的订单，供 AI 先核对再操作。
	RefundableOrders(ctx context.Context, userID uint, limit int) ([]OrderContext, error)
}

// Notifier 是通知模块暴露给 AI 的发送能力。AI 只能发送站内通知，
// 且必须落在当前用户或工单关联人身上。
type Notifier interface {
	NotifyUser(ctx context.Context, userID uint, notificationType, title, content, link string) error
}

// MailSender 是站外提醒能力，配置关闭时实现方直接返回 nil。
type MailSender interface {
	SendToUser(ctx context.Context, userID uint, subject, body string) error
}

// KnowledgeProvider 让 AI 服务读取知识库全文。
type KnowledgeProvider interface {
	Search(ctx context.Context, query string, limit int) ([]domain.KnowledgeHit, error)
}

// ModelMessage 是一条发给大模型的对话消息。
type ModelMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ModelRequest 是一次大模型请求。
type ModelRequest struct {
	SystemPrompt string
	Messages     []ModelMessage
	MaxTokens    int
	Temperature  float64
	TopP         float64
	TimeoutMS    int
}

// ModelReply 是一次大模型回答。
type ModelReply struct {
	Content string
	Tokens  int
	Raw     []byte
}

// ModelClient 是大模型调用的抽象，真实实现走 OpenAI 兼容协议，
// 测试里可以替换成假实现，AI 的受控逻辑不依赖外部服务。
type ModelClient interface {
	Complete(ctx context.Context, config *domain.AIConfig, request ModelRequest) (*ModelReply, error)
}

// Access 描述一次 AI 请求的调用者身份。
type Access struct {
	UserID   uint
	UserRole string
	GuestKey string
	IsStaff  bool
}
