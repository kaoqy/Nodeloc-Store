package models

import (
	"time"
)

// ── 工单与 AI 客服 ────────────────────────────────────────────────────
//
// 工单默认先由 AI 接待：AI 读知识库、在授权范围内调用只读工具，能解决就
// 解决，解决不了或用户主动要求时转人工。所有对话、工具调用、转人工和状态
// 变化都留痕，客服接手时能看到完整的 AI 上下文。

// 工单状态。
const (
	TicketStatusAIProcessing   = "ai_processing"
	TicketStatusWaitingUser    = "waiting_user"
	TicketStatusAISolved       = "ai_solved"
	TicketStatusUserRequested  = "user_requested_human"
	TicketStatusPendingHuman   = "pending_human"
	TicketStatusHumanHandling  = "human_handling"
	TicketStatusWaitingConfirm = "waiting_confirm"
	TicketStatusResolved       = "resolved"
	TicketStatusClosed         = "closed"
	TicketStatusRejected       = "rejected"
	TicketStatusCancelled      = "cancelled"
)

var TicketStatusLabels = map[string]string{
	TicketStatusAIProcessing:   "AI 处理中",
	TicketStatusWaitingUser:    "等待用户回复",
	TicketStatusAISolved:       "AI 已解决",
	TicketStatusUserRequested:  "用户申请人工",
	TicketStatusPendingHuman:   "待人工处理",
	TicketStatusHumanHandling:  "人工处理中",
	TicketStatusWaitingConfirm: "等待用户确认",
	TicketStatusResolved:       "已解决",
	TicketStatusClosed:         "已关闭",
	TicketStatusRejected:       "已拒绝",
	TicketStatusCancelled:      "已撤销",
}

// 工单处理方：AI 或人工。
const (
	TicketHandlerAI     = "ai"
	TicketHandlerHuman  = "human"
	TicketHandlerSystem = "system"
)

// 优先级。
var TicketPriorities = []string{"low", "normal", "high", "urgent"}

var TicketPriorityLabels = map[string]string{
	"low": "低", "normal": "普通", "high": "高", "urgent": "紧急",
}

// Ticket 是一张工单。状态流转见 TicketStatusLabels，AI 接待是默认路径。
type Ticket struct {
	Base
	TicketNo string `gorm:"size:64;uniqueIndex;not null" json:"ticket_no"`
	UserID   uint   `gorm:"not null;index" json:"user_id"`
	// 关联订单与商品：AI 和客服都靠它定位问题，允许为空（咨询类工单）。
	OrderID     *uint  `gorm:"index" json:"order_id,omitempty"`
	OrderNo     string `gorm:"size:64;index" json:"order_no,omitempty"`
	ProductID   *uint  `gorm:"index" json:"product_id,omitempty"`
	ProductName string `gorm:"size:160" json:"product_name,omitempty"`

	Type     string `gorm:"size:48;not null;index" json:"type"`
	TypeName string `gorm:"size:80" json:"type_name,omitempty"`
	Subject  string `gorm:"size:200;not null" json:"subject"`
	Content  string `gorm:"type:text" json:"content,omitempty"`

	Status   string `gorm:"size:32;default:'ai_processing';not null;index" json:"status"`
	Priority string `gorm:"size:16;default:'normal';not null;index" json:"priority"`
	// Handler 说明当前处理方：ai / human / system。
	Handler string `gorm:"size:16;default:'ai';not null;index" json:"handler"`
	// AssignedAgentID 是人工客服负责人的用户 ID，未转人工时为空。
	AssignedAgentID *uint `gorm:"index" json:"assigned_agent_id,omitempty"`

	AIEnabled bool `gorm:"default:true;not null" json:"ai_enabled"`
	// TransferReason 记录为什么转人工，客服在摘要里直接看到。
	TransferReason string `gorm:"size:255" json:"transfer_reason,omitempty"`
	Summary        string `gorm:"type:text" json:"summary,omitempty"`
	SuggestedPlan  string `gorm:"type:text" json:"suggested_plan,omitempty"`

	Tags            string `gorm:"size:255" json:"tags,omitempty"`
	Source          string `gorm:"size:32;default:'web';not null" json:"source"`
	CustomerContact string `gorm:"size:255" json:"customer_contact,omitempty"`

	Satisfaction     int    `gorm:"default:0;not null" json:"satisfaction"`
	SatisfactionNote string `gorm:"size:500" json:"satisfaction_note,omitempty"`

	LastMessageAt   *time.Time `gorm:"index" json:"last_message_at,omitempty"`
	FirstResponseAt *time.Time `json:"first_response_at,omitempty"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`
	ClosedAt        *time.Time `json:"closed_at,omitempty"`
	DueAt           *time.Time `gorm:"index" json:"due_at,omitempty"`

	MessageCount    int  `gorm:"default:0;not null" json:"message_count"`
	AttachmentCount int  `gorm:"default:0;not null" json:"attachment_count"`
	UnreadForStaff  bool `gorm:"default:true;not null" json:"unread_for_staff"`
	UnreadForUser   bool `gorm:"default:false;not null" json:"unread_for_user"`
}

// TicketMessage 是工单消息流的一行，AI、用户、客服、系统共用一张表，
// 顺序即真实时间顺序，转人工不切表、不丢上下文。
type TicketMessage struct {
	Base
	TicketID uint `gorm:"not null;index" json:"ticket_id"`
	// SenderType: user / ai / agent / system。
	SenderType  string `gorm:"size:16;not null;index" json:"sender_type"`
	SenderID    *uint  `gorm:"index" json:"sender_id,omitempty"`
	SenderName  string `gorm:"size:80" json:"sender_name,omitempty"`
	Content     string `gorm:"type:text;not null" json:"content"`
	ContentType string `gorm:"size:16;default:'text';not null" json:"content_type"`
	// IsInternal 的内部备注只给客服看，不进入用户消息流。
	IsInternal bool   `gorm:"default:false;not null;index" json:"is_internal"`
	Meta       string `gorm:"type:text" json:"meta,omitempty"`
}

// TicketLog 是工单的每一次状态/负责人/优先级变化，含操作前后的快照。
type TicketLog struct {
	Base
	TicketID  uint   `gorm:"not null;index" json:"ticket_id"`
	ActorID   *uint  `gorm:"index" json:"actor_id,omitempty"`
	ActorType string `gorm:"size:16;default:'system';not null" json:"actor_type"`
	Action    string `gorm:"size:64;not null;index" json:"action"`
	Before    string `gorm:"type:text" json:"before,omitempty"`
	After     string `gorm:"type:text" json:"after,omitempty"`
	Detail    string `gorm:"type:text" json:"detail,omitempty"`
	IP        string `gorm:"size:64" json:"ip,omitempty"`
	UserAgent string `gorm:"size:255" json:"user_agent,omitempty"`
	Result    string `gorm:"size:24;default:'ok';not null" json:"result"`
	Error     string `gorm:"type:text" json:"error,omitempty"`
}

// TicketAttachment 是工单附件。类型与大小在配置里限制，落盘前校验。
type TicketAttachment struct {
	Base
	TicketID  uint   `gorm:"not null;index" json:"ticket_id"`
	MessageID *uint  `gorm:"index" json:"message_id,omitempty"`
	UserID    uint   `gorm:"not null;index" json:"user_id"`
	FileName  string `gorm:"size:255;not null" json:"file_name"`
	FilePath  string `gorm:"size:500;not null" json:"file_path"`
	MimeType  string `gorm:"size:120" json:"mime_type,omitempty"`
	Size      int64  `gorm:"default:0;not null" json:"size"`
	Kind      string `gorm:"size:24;default:'image';not null" json:"kind"`
	Status    string `gorm:"size:16;default:'ready';not null" json:"status"`
}

// TicketAISession 是工单侧 AI 会话的摘要信息：意图、订单、是否建议转人工，
// 以及给客服看的问题摘要和推荐处理方案。
type TicketAISession struct {
	Base
	TicketID       uint       `gorm:"not null;index" json:"ticket_id"`
	ConversationID uint       `gorm:"index" json:"conversation_id"`
	Status         string     `gorm:"size:24;default:'active';not null;index" json:"status"`
	Intent         string     `gorm:"size:64" json:"intent,omitempty"`
	OrderNo        string     `gorm:"size:64" json:"order_no,omitempty"`
	ProductName    string     `gorm:"size:160" json:"product_name,omitempty"`
	Summary        string     `gorm:"type:text" json:"summary,omitempty"`
	SuggestedPlan  string     `gorm:"type:text" json:"suggested_plan,omitempty"`
	NeedsHuman     bool       `gorm:"default:false;not null" json:"needs_human"`
	Reason         string     `gorm:"size:255" json:"reason,omitempty"`
	FailureCount   int        `gorm:"default:0;not null" json:"failure_count"`
	DownvoteCount  int        `gorm:"default:0;not null" json:"downvote_count"`
	TurnCount      int        `gorm:"default:0;not null" json:"turn_count"`
	LastActiveAt   *time.Time `json:"last_active_at,omitempty"`
}

// AIConversation 是 AI 客服的一场对话（网站悬浮客服或工单内对话）。
type AIConversation struct {
	Base
	// UserID 为空表示游客会话；游客受到更严格的频率限制。
	UserID        *uint      `gorm:"index" json:"user_id,omitempty"`
	TicketID      *uint      `gorm:"index" json:"ticket_id,omitempty"`
	Channel       string     `gorm:"size:24;default:'widget';not null;index" json:"channel"`
	Status        string     `gorm:"size:24;default:'active';not null;index" json:"status"`
	Title         string     `gorm:"size:200" json:"title,omitempty"`
	AgentName     string     `gorm:"size:80" json:"agent_name,omitempty"`
	PageContext   string     `gorm:"size:255" json:"page_context,omitempty"`
	IP            string     `gorm:"size:64;index" json:"ip,omitempty"`
	UserAgent     string     `gorm:"size:255" json:"user_agent,omitempty"`
	MessageCount  int        `gorm:"default:0;not null" json:"message_count"`
	HandedToHuman bool       `gorm:"default:false;not null" json:"handed_to_human"`
	Rating        int        `gorm:"default:0;not null" json:"rating"`
	RatingNote    string     `gorm:"size:500" json:"rating_note,omitempty"`
	LastMessageAt *time.Time `gorm:"index" json:"last_message_at,omitempty"`
}

// AIMessage 是 AI 对话的一条消息，role 取值 system/user/assistant/tool。
type AIMessage struct {
	Base
	ConversationID uint   `gorm:"not null;index" json:"conversation_id"`
	Role           string `gorm:"size:16;not null;index" json:"role"`
	Content        string `gorm:"type:text" json:"content,omitempty"`
	// ToolKey / ToolCallID 只在 role=tool 时填写。
	ToolKey    string `gorm:"size:64" json:"tool_key,omitempty"`
	ToolCallID string `gorm:"size:64" json:"tool_call_id,omitempty"`
	Model      string `gorm:"size:80" json:"model,omitempty"`
	Tokens     int    `gorm:"default:0;not null" json:"tokens"`
	Status     string `gorm:"size:16;default:'ok';not null" json:"status"`
	Error      string `gorm:"type:text" json:"error,omitempty"`
	Meta       string `gorm:"type:text" json:"meta,omitempty"`
}

// AIToolDefinition 是一个受控工具：AI 只能调用这里登记且启用的工具。
type AIToolDefinition struct {
	Base
	Key         string `gorm:"size:64;uniqueIndex;not null" json:"key"`
	Name        string `gorm:"size:120;not null" json:"name"`
	Description string `gorm:"type:text" json:"description,omitempty"`
	Category    string `gorm:"size:48;default:'query';not null;index" json:"category"`
	// Method 是展示用的请求方式；内部工具统一为 INTERNAL。
	Method   string `gorm:"size:16;default:'INTERNAL';not null" json:"method"`
	Endpoint string `gorm:"size:255" json:"endpoint,omitempty"`
	Kind     string `gorm:"size:16;default:'internal';not null" json:"kind"`
	// ParamsSchema / ResultSchema 是 JSON Schema 子集，用于前后端校验和脱敏描述。
	ParamsSchema string `gorm:"type:text" json:"params_schema,omitempty"`
	ResultSchema string `gorm:"type:text" json:"result_schema,omitempty"`

	RequireLogin    bool `gorm:"default:true;not null" json:"require_login"`
	OwnDataOnly     bool `gorm:"default:true;not null" json:"own_data_only"`
	RequireApproval bool `gorm:"default:false;not null" json:"require_approval"`
	AllowAuto       bool `gorm:"default:true;not null" json:"allow_auto"`
	RequireConfirm  bool `gorm:"default:false;not null" json:"require_confirm"`
	IsEnabled       bool `gorm:"default:true;not null;index" json:"is_enabled"`
	// EnabledByDefault 记录「这个工具出厂就该启用」。升级时用它做一次性放开，
	// 之后管理员手动关掉的开关不会被再次打开。
	EnabledByDefault bool `gorm:"default:false;not null" json:"enabled_by_default"`

	RateLimit   int    `gorm:"default:30;not null" json:"rate_limit"`
	TimeoutMS   int    `gorm:"default:8000;not null" json:"timeout_ms"`
	FailureMode string `gorm:"size:24;default:'reply';not null" json:"failure_mode"`
	RiskLevel   string `gorm:"size:16;default:'low';not null;index" json:"risk_level"`
	// Builtin 是内置工具标记：内置工具不能被删除，只能停用或调整权限。
	Builtin   bool `gorm:"default:false;not null" json:"builtin"`
	SortOrder int  `gorm:"default:0;not null" json:"sort_order"`
}

// AIToolPermission 让某个角色是否可以使用某个工具。
type AIToolPermission struct {
	Base
	ToolID    uint   `gorm:"not null;index:idx_ai_tool_role" json:"tool_id"`
	Role      string `gorm:"size:32;not null;index:idx_ai_tool_role" json:"role"`
	Allowed   bool   `gorm:"default:true;not null" json:"allowed"`
	UpdatedBy *uint  `gorm:"index" json:"updated_by,omitempty"`
}

// AIToolCall 是每一次工具调用的审计记录。Params/Result 都是脱敏后的文本。
type AIToolCall struct {
	Base
	ConversationID *uint  `gorm:"index" json:"conversation_id,omitempty"`
	MessageID      *uint  `gorm:"index" json:"message_id,omitempty"`
	TicketID       *uint  `gorm:"index" json:"ticket_id,omitempty"`
	UserID         *uint  `gorm:"index" json:"user_id,omitempty"`
	ToolKey        string `gorm:"size:64;not null;index" json:"tool_key"`
	ToolName       string `gorm:"size:120" json:"tool_name,omitempty"`
	Params         string `gorm:"type:text" json:"params,omitempty"`
	Result         string `gorm:"type:text" json:"result,omitempty"`
	Status         string `gorm:"size:24;default:'ok';not null;index" json:"status"`
	Error          string `gorm:"type:text" json:"error,omitempty"`
	DurationMS     int64  `gorm:"default:0;not null" json:"duration_ms"`
	RequireConfirm bool   `gorm:"default:false;not null" json:"require_confirm"`
	ConfirmedBy    *uint  `gorm:"index" json:"confirmed_by,omitempty"`
	RiskLevel      string `gorm:"size:16;default:'low';not null" json:"risk_level"`
	IP             string `gorm:"size:64" json:"ip,omitempty"`
}

// AIKnowledgeCategory 是知识库分类。
type AIKnowledgeCategory struct {
	Base
	Name        string `gorm:"size:120;not null" json:"name"`
	Slug        string `gorm:"size:120;uniqueIndex;not null" json:"slug"`
	Description string `gorm:"type:text" json:"description,omitempty"`
	ParentID    *uint  `gorm:"index" json:"parent_id,omitempty"`
	SortOrder   int    `gorm:"default:0;not null" json:"sort_order"`
	IsEnabled   bool   `gorm:"default:true;not null" json:"is_enabled"`
}

// AIKnowledge 是一篇知识库文章。Keywords 参与检索，Content 直接进提示词。
//
// TableName 固定为 ai_knowledge：GORM 会把 AIKnowledge 复数化成
// ai_knowledges，而迁移脚本、运维查询与后台约定都使用单数表名。
type AIKnowledge struct {
	Base
	CategoryID uint   `gorm:"index" json:"category_id"`
	Title      string `gorm:"size:200;not null" json:"title"`
	Slug       string `gorm:"size:200;uniqueIndex;not null" json:"slug"`
	Summary    string `gorm:"size:500" json:"summary,omitempty"`
	Content    string `gorm:"type:text;not null" json:"content"`
	Keywords   string `gorm:"size:500" json:"keywords,omitempty"`
	Tags       string `gorm:"size:255" json:"tags,omitempty"`
	Priority   int    `gorm:"default:0;not null;index" json:"priority"`
	// Status: draft / published / disabled。
	Status      string     `gorm:"size:16;default:'draft';not null;index" json:"status"`
	PublishAt   *time.Time `gorm:"index" json:"publish_at,omitempty"`
	ExpireAt    *time.Time `gorm:"index" json:"expire_at,omitempty"`
	Version     int        `gorm:"default:1;not null" json:"version"`
	AuthorID    *uint      `gorm:"index" json:"author_id,omitempty"`
	Source      string     `gorm:"size:32;default:'manual';not null" json:"source"`
	ViewCount   int        `gorm:"default:0;not null" json:"view_count"`
	UsefulCount int        `gorm:"default:0;not null" json:"useful_count"`
	// Builtin 的官方说明文章不允许删除，只能下线。
	Builtin bool `gorm:"default:false;not null" json:"builtin"`
}

// AIFeedback 是用户对 AI 回复的点赞/点踩，点踩累计到阈值会触发转人工建议。
type AIFeedback struct {
	Base
	ConversationID uint  `gorm:"not null;index" json:"conversation_id"`
	MessageID      *uint `gorm:"index" json:"message_id,omitempty"`
	TicketID       *uint `gorm:"index" json:"ticket_id,omitempty"`
	UserID         *uint `gorm:"index" json:"user_id,omitempty"`
	// Rating: 1 点赞，-1 点踩。
	Rating  int    `gorm:"default:0;not null;index" json:"rating"`
	Reason  string `gorm:"size:120" json:"reason,omitempty"`
	Comment string `gorm:"size:500" json:"comment,omitempty"`
	Handled bool   `gorm:"default:false;not null" json:"handled"`
}

// AIConfig 是 AI 客服的总配置（单行）。APIKeyEnc 是加密后的密钥，
// 任何读接口都不会返回明文，只回 has_key。
type AIConfig struct {
	Base
	Provider    string  `gorm:"size:32;default:'openai-compatible';not null" json:"provider"`
	BaseURL     string  `gorm:"size:255" json:"base_url,omitempty"`
	APIKeyEnc   string  `gorm:"type:text" json:"-"`
	Model       string  `gorm:"size:120" json:"model,omitempty"`
	TimeoutMS   int     `gorm:"default:30000;not null" json:"timeout_ms"`
	MaxContext  int     `gorm:"default:12;not null" json:"max_context"`
	MaxReplyLen int     `gorm:"default:2000;not null" json:"max_reply_len"`
	Temperature float64 `gorm:"default:0.3;not null" json:"temperature"`
	TopP        float64 `gorm:"default:1;not null" json:"top_p"`
	IsEnabled   bool    `gorm:"default:false;not null" json:"is_enabled"`

	GuestAllowed    bool `gorm:"default:true;not null" json:"guest_allowed"`
	GuestDailyLimit int  `gorm:"default:10;not null" json:"guest_daily_limit"`
	UserDailyLimit  int  `gorm:"default:100;not null" json:"user_daily_limit"`
	IPRateLimit     int  `gorm:"default:30;not null" json:"ip_rate_limit"`
	MaxMessageLen   int  `gorm:"default:2000;not null" json:"max_message_len"`

	SystemPrompt  string `gorm:"type:text" json:"system_prompt,omitempty"`
	Greeting      string `gorm:"type:text" json:"greeting,omitempty"`
	FallbackReply string `gorm:"type:text" json:"fallback_reply,omitempty"`
	TransferTip   string `gorm:"type:text" json:"transfer_tip,omitempty"`
	TicketTip     string `gorm:"type:text" json:"ticket_tip,omitempty"`
	SensitiveTip  string `gorm:"type:text" json:"sensitive_tip,omitempty"`
	Avatar        string `gorm:"size:500" json:"avatar,omitempty"`
	AgentName     string `gorm:"size:80;default:'智能客服';not null" json:"agent_name"`

	RatingEnabled bool `gorm:"default:true;not null" json:"rating_enabled"`
	LogEnabled    bool `gorm:"default:true;not null" json:"log_enabled"`
	MailEnabled   bool `gorm:"default:false;not null" json:"mail_enabled"`
}

// AIWorkflowConfig 是 AI 工作流与转人工策略（单行）。
type AIWorkflowConfig struct {
	Base
	DefaultHandleMinutes   int  `gorm:"default:10;not null" json:"default_handle_minutes"`
	MaxFailures            int  `gorm:"default:3;not null" json:"max_failures"`
	TransferAfterFailures  int  `gorm:"default:2;not null" json:"transfer_after_failures"`
	TransferAfterDownvotes int  `gorm:"default:2;not null" json:"transfer_after_downvotes"`
	TransferOnExplicit     bool `gorm:"default:true;not null" json:"transfer_on_explicit"`
	TransferHighAmount     bool `gorm:"default:true;not null" json:"transfer_high_amount"`
	HighAmountThreshold    int  `gorm:"default:500;not null" json:"high_amount_threshold"`
	TransferRefund         bool `gorm:"default:true;not null" json:"transfer_refund"`
	TransferCardDispute    bool `gorm:"default:true;not null" json:"transfer_card_dispute"`
	TransferPaymentIssue   bool `gorm:"default:true;not null" json:"transfer_payment_issue"`
	TransferAbuse          bool `gorm:"default:true;not null" json:"transfer_abuse"`

	CanCreateTicket      bool `gorm:"default:true;not null" json:"can_create_ticket"`
	CanUpdateTicket      bool `gorm:"default:false;not null" json:"can_update_ticket"`
	CanNotify            bool `gorm:"default:false;not null" json:"can_notify"`
	CanQueryOrder        bool `gorm:"default:true;not null" json:"can_query_order"`
	CanQueryShipping     bool `gorm:"default:true;not null" json:"can_query_shipping"`
	CanRecommendActivity bool `gorm:"default:true;not null" json:"can_recommend_activity"`
	CanGrantCoupon       bool `gorm:"default:false;not null" json:"can_grant_coupon"`
	// CanRefund 允许 AI 直接对当前用户自己的已支付订单发起退款。
	CanRefund bool `gorm:"default:true;not null" json:"can_refund"`
	// RequireHumanRefund 为 true 时，提到退款就转人工；默认 false，由 AI 直接处理。
	RequireHumanRefund   bool   `gorm:"default:false;not null" json:"require_human_refund"`
	TransferNotice       string `gorm:"type:text" json:"transfer_notice,omitempty"`
	WorkingHours         string `gorm:"size:120" json:"working_hours,omitempty"`
	EstimateReplyMinutes int    `gorm:"default:30;not null" json:"estimate_reply_minutes"`
}

// AIQuickQuestion 是前台快捷问题，可绑定知识库文章或工具。
type AIQuickQuestion struct {
	Base
	Title        string `gorm:"size:160;not null" json:"title"`
	Content      string `gorm:"type:text" json:"content,omitempty"`
	Position     string `gorm:"size:32;default:'widget';not null" json:"position"`
	Pages        string `gorm:"size:255" json:"pages,omitempty"`
	SortOrder    int    `gorm:"default:0;not null" json:"sort_order"`
	IsEnabled    bool   `gorm:"default:true;not null;index" json:"is_enabled"`
	RequireLogin bool   `gorm:"default:false;not null" json:"require_login"`
	KnowledgeID  *uint  `gorm:"index" json:"knowledge_id,omitempty"`
	ToolKey      string `gorm:"size:64" json:"tool_key,omitempty"`
}

// CustomerServiceAgent 是人工客服坐席配置。
type CustomerServiceAgent struct {
	Base
	UserID   uint   `gorm:"not null;uniqueIndex" json:"user_id"`
	Nickname string `gorm:"size:80" json:"nickname,omitempty"`
	Avatar   string `gorm:"size:500" json:"avatar,omitempty"`
	// Status: online / busy / offline。
	Status         string  `gorm:"size:16;default:'offline';not null;index" json:"status"`
	AcceptManual   bool    `gorm:"default:true;not null" json:"accept_manual"`
	TicketTypes    string  `gorm:"size:255" json:"ticket_types,omitempty"`
	MaxConcurrent  int     `gorm:"default:5;not null" json:"max_concurrent"`
	WorkStart      string  `gorm:"size:8;default:'09:00'" json:"work_start,omitempty"`
	WorkEnd        string  `gorm:"size:8;default:'21:00'" json:"work_end,omitempty"`
	AssignStrategy string  `gorm:"size:24;default:'balanced';not null" json:"assign_strategy"`
	Role           string  `gorm:"size:32;default:'support';not null" json:"role"`
	Score          float64 `gorm:"default:0;not null" json:"score"`
	AvgResponseSec int     `gorm:"default:0;not null" json:"avg_response_sec"`
	HandledCount   int     `gorm:"default:0;not null" json:"handled_count"`
	IsActive       bool    `gorm:"default:true;not null;index" json:"is_active"`
}

// CustomerServiceAssignment 是工单的人工分配记录，支持转交与重新分配。
type CustomerServiceAssignment struct {
	Base
	TicketID   uint       `gorm:"not null;index" json:"ticket_id"`
	AgentID    uint       `gorm:"not null;index" json:"agent_id"`
	AssignedBy *uint      `gorm:"index" json:"assigned_by,omitempty"`
	Strategy   string     `gorm:"size:24;default:'manual';not null" json:"strategy"`
	Status     string     `gorm:"size:24;default:'assigned';not null;index" json:"status"`
	AssignedAt time.Time  `json:"assigned_at"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// QuickReply 是客服的快捷回复，支持变量占位符（{ticket_no}、{user_name} 等）。
type QuickReply struct {
	Base
	Title       string `gorm:"size:160;not null" json:"title"`
	Content     string `gorm:"type:text;not null" json:"content"`
	TicketTypes string `gorm:"size:255" json:"ticket_types,omitempty"`
	Scene       string `gorm:"size:48" json:"scene,omitempty"`
	Roles       string `gorm:"size:255" json:"roles,omitempty"`
	SortOrder   int    `gorm:"default:0;not null" json:"sort_order"`
	IsEnabled   bool   `gorm:"default:true;not null;index" json:"is_enabled"`
	Variables   string `gorm:"size:500" json:"variables,omitempty"`
	UseCount    int    `gorm:"default:0;not null" json:"use_count"`
}

// NotificationTemplate 让每一种通知的标题、正文、渠道都可以在后台配置。
type NotificationTemplate struct {
	Base
	Key             string `gorm:"size:64;uniqueIndex;not null" json:"key"`
	Name            string `gorm:"size:120;not null" json:"name"`
	Event           string `gorm:"size:64;not null;index" json:"event"`
	Category        string `gorm:"size:32;default:'ticket';not null;index" json:"category"`
	IsEnabled       bool   `gorm:"default:true;not null" json:"is_enabled"`
	InApp           bool   `gorm:"default:true;not null" json:"in_app"`
	Mail            bool   `gorm:"default:false;not null" json:"mail"`
	TitleTemplate   string `gorm:"type:text" json:"title_template,omitempty"`
	ContentTemplate string `gorm:"type:text" json:"content_template,omitempty"`
	Variables       string `gorm:"size:500" json:"variables,omitempty"`
	Recipients      string `gorm:"size:120" json:"recipients,omitempty"`
	RetryLimit      int    `gorm:"default:3;not null" json:"retry_limit"`
	SortOrder       int    `gorm:"default:0;not null" json:"sort_order"`
}

// NotificationLog 记录一次发送尝试，含失败原因，便于排查与重试。
type NotificationLog struct {
	Base
	TemplateKey string     `gorm:"size:64;index" json:"template_key"`
	Channel     string     `gorm:"size:16;default:'in_app';not null;index" json:"channel"`
	UserID      *uint      `gorm:"index" json:"user_id,omitempty"`
	TicketID    *uint      `gorm:"index" json:"ticket_id,omitempty"`
	OrderID     *uint      `gorm:"index" json:"order_id,omitempty"`
	Title       string     `gorm:"size:255" json:"title,omitempty"`
	Content     string     `gorm:"type:text" json:"content,omitempty"`
	Status      string     `gorm:"size:16;default:'sent';not null;index" json:"status"`
	Error       string     `gorm:"type:text" json:"error,omitempty"`
	Attempts    int        `gorm:"default:1;not null" json:"attempts"`
	SentAt      *time.Time `json:"sent_at,omitempty"`
}

// SystemConfig 是统一配置中心的通用键值表，按分组组织。
// 重要开关与模板都放这里，避免把配置写死在代码里。
type SystemConfig struct {
	Base
	Group       string `gorm:"size:64;not null;uniqueIndex:uniq_system_config;index" json:"group"`
	Key         string `gorm:"size:64;not null;uniqueIndex:uniq_system_config" json:"key"`
	Value       string `gorm:"type:text" json:"value,omitempty"`
	ValueType   string `gorm:"size:16;default:'string';not null" json:"value_type"`
	Label       string `gorm:"size:120" json:"label,omitempty"`
	Description string `gorm:"size:500" json:"description,omitempty"`
	// IsSecret 的配置只写不回读，读接口返回 has_value 而不是明文。
	IsSecret  bool  `gorm:"default:false;not null" json:"is_secret"`
	SortOrder int   `gorm:"default:0;not null" json:"sort_order"`
	UpdatedBy *uint `gorm:"index" json:"updated_by,omitempty"`
}

// OperationLog 是跨模块的统一操作日志，比 AuditLog 多出工单/订单/角色字段，
// 用于「重要操作全量留痕」这一条要求。
type OperationLog struct {
	Base
	ActorID    *uint  `gorm:"index" json:"actor_id,omitempty"`
	ActorRole  string `gorm:"size:32;index" json:"actor_role,omitempty"`
	ActorName  string `gorm:"size:64" json:"actor_name,omitempty"`
	UserID     *uint  `gorm:"index" json:"user_id,omitempty"`
	TicketID   *uint  `gorm:"index" json:"ticket_id,omitempty"`
	OrderID    *uint  `gorm:"index" json:"order_id,omitempty"`
	Resource   string `gorm:"size:64;not null;index" json:"resource"`
	Action     string `gorm:"size:64;not null;index" json:"action"`
	Before     string `gorm:"type:text" json:"before,omitempty"`
	After      string `gorm:"type:text" json:"after,omitempty"`
	Detail     string `gorm:"type:text" json:"detail,omitempty"`
	IP         string `gorm:"size:64" json:"ip,omitempty"`
	UserAgent  string `gorm:"size:255" json:"user_agent,omitempty"`
	Result     string `gorm:"size:24;default:'ok';not null;index" json:"result"`
	Error      string `gorm:"type:text" json:"error,omitempty"`
	DurationMS int64  `gorm:"default:0;not null" json:"duration_ms"`
}

// TableName 固定知识库表名，避免 GORM 复数化规则将来再次改名。
func (AIKnowledge) TableName() string { return "ai_knowledge" }

// TableName 固定知识库分类表名，与迁移脚本保持一致。
func (AIKnowledgeCategory) TableName() string { return "ai_knowledge_categories" }
