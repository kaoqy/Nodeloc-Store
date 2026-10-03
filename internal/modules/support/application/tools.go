package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/domain"
)

// ToolHandler 是一个受控工具的实现。参数已经在调用前做过白名单校验，
// 返回的数据也必须是可安全展示给当前调用者的字段。
type ToolHandler func(ctx context.Context, access contract.Access, params map[string]any) (any, error)

// ToolSpec 是注册表里的一个工具定义：声明式元数据 + 实际执行函数。
type ToolSpec struct {
	Key            string
	Name           string
	Description    string
	Category       string
	RiskLevel      string
	RequireLogin   bool
	OwnDataOnly    bool
	RequireConfirm bool
	AllowAuto      bool
	RateLimit      int
	TimeoutMS      int
	AllowedParams  []string
	RequiredParams []string
	ParamTypes     map[string]string
	Handler        ToolHandler
}

// ToolRegistry 保存所有允许 AI 调用的工具。没有注册的工具一律拒绝，
// 这正是「AI 不能调用未授权接口」的实现方式。
type ToolRegistry interface {
	Lookup(key string) (*ToolSpec, bool)
	All() []*ToolSpec
}

type registry struct {
	specs map[string]*ToolSpec
	order []*ToolSpec
}

// NewRegistry 建立空注册表；工具在 Wire 里按内置清单注册。
func NewRegistry() ToolRegistry {
	return &registry{specs: map[string]*ToolSpec{}}
}

func (r *registry) Register(spec *ToolSpec) {
	if spec == nil || spec.Key == "" {
		return
	}
	if _, exists := r.specs[spec.Key]; exists {
		return
	}
	r.specs[spec.Key] = spec
	r.order = append(r.order, spec)
}

func (r *registry) Lookup(key string) (*ToolSpec, bool) {
	spec, ok := r.specs[strings.TrimSpace(key)]
	return spec, ok
}

func (r *registry) All() []*ToolSpec {
	out := make([]*ToolSpec, len(r.order))
	copy(out, r.order)
	return out
}

// MutableRegistry 给 Wire 使用，对外仍然只暴露只读接口。
type MutableRegistry interface {
	ToolRegistry
	Register(spec *ToolSpec)
}

// AsMutable 把只读注册表还原成可注册版本。
func AsMutable(registry ToolRegistry) (MutableRegistry, bool) {
	mutable, ok := registry.(MutableRegistry)
	return mutable, ok
}

// ── 内置工具实现 ─────────────────────────────────────────────────────

// registerBuiltins 注册默认工具集。工具本身只读或是有限写操作：写操作
// 默认关闭，需要管理员在 AI 工具页显式放开。
func (s *Service) registerBuiltins() {
	mutable, ok := AsMutable(s.tools)
	if !ok {
		return
	}
	register := func(spec *ToolSpec) { mutable.Register(spec) }

	register(&ToolSpec{
		Key: "user.profile", Name: "查询当前用户资料", Category: "user", RiskLevel: domain.RiskLow,
		Description:  "查询当前登录用户自己的账号资料与绑定状态。",
		RequireLogin: true, OwnDataOnly: true, AllowAuto: true, RateLimit: 30,
		Handler: s.toolUserProfile,
	})
	register(&ToolSpec{
		Key: "order.list", Name: "查询当前用户订单", Category: "order", RiskLevel: domain.RiskLow,
		Description:  "查询当前登录用户自己的订单列表，只返回摘要。",
		RequireLogin: true, OwnDataOnly: true, AllowAuto: true, RateLimit: 30,
		AllowedParams: []string{"limit"}, ParamTypes: map[string]string{"limit": "int"},
		Handler: s.toolOrderList,
	})
	register(&ToolSpec{
		Key: "order.detail", Name: "查询订单详情", Category: "order", RiskLevel: domain.RiskLow,
		Description:  "按订单号查询当前用户自己的一笔订单，含支付与发货状态。",
		RequireLogin: true, OwnDataOnly: true, AllowAuto: true, RateLimit: 30,
		AllowedParams: []string{"order_no"}, RequiredParams: []string{"order_no"},
		ParamTypes: map[string]string{"order_no": "string"},
		Handler:    s.toolOrderDetail,
	})
	register(&ToolSpec{
		Key: "order.payment_status", Name: "查询订单支付状态", Category: "order", RiskLevel: domain.RiskLow,
		Description:  "查询当前用户一笔订单的支付状态。",
		RequireLogin: true, OwnDataOnly: true, AllowAuto: true, RateLimit: 30,
		AllowedParams: []string{"order_no"}, RequiredParams: []string{"order_no"},
		ParamTypes: map[string]string{"order_no": "string"},
		Handler:    s.toolPaymentStatus,
	})
	register(&ToolSpec{
		Key: "order.delivery_status", Name: "查询订单发货状态", Category: "order", RiskLevel: domain.RiskLow,
		Description:  "查询当前用户一笔订单的发货状态与交付摘要（不含完整卡密）。",
		RequireLogin: true, OwnDataOnly: true, AllowAuto: true, RateLimit: 30,
		AllowedParams: []string{"order_no"}, RequiredParams: []string{"order_no"},
		ParamTypes: map[string]string{"order_no": "string"},
		Handler:    s.toolDeliveryStatus,
	})
	register(&ToolSpec{
		Key: "product.detail", Name: "查询商品详情", Category: "product", RiskLevel: domain.RiskLow,
		Description:  "查询在售商品的公开信息：名称、价格、库存、分类。",
		RequireLogin: false, OwnDataOnly: false, AllowAuto: true, RateLimit: 60,
		AllowedParams: []string{"product_id"}, RequiredParams: []string{"product_id"},
		ParamTypes: map[string]string{"product_id": "int"},
		Handler:    s.toolProductDetail,
	})
	register(&ToolSpec{
		Key: "product.list", Name: "查询在售商品", Category: "product", RiskLevel: domain.RiskLow,
		Description:  "列出当前在售商品的公开信息，用于推荐或对比。",
		RequireLogin: false, OwnDataOnly: false, AllowAuto: true, RateLimit: 30,
		AllowedParams: []string{"limit"}, ParamTypes: map[string]string{"limit": "int"},
		Handler: s.toolProductList,
	})
	register(&ToolSpec{
		Key: "activity.list", Name: "查询当前活动", Category: "activity", RiskLevel: domain.RiskLow,
		Description:  "列出当前正在进行中的促销活动与规则摘要。",
		RequireLogin: false, OwnDataOnly: false, AllowAuto: true, RateLimit: 30,
		Handler: s.toolActivityList,
	})
	register(&ToolSpec{
		Key: "knowledge.search", Name: "获取帮助文档", Category: "knowledge", RiskLevel: domain.RiskLow,
		Description:  "在知识库里检索帮助文档与售后规则。",
		RequireLogin: false, OwnDataOnly: false, AllowAuto: true, RateLimit: 60,
		AllowedParams: []string{"query"}, RequiredParams: []string{"query"},
		ParamTypes: map[string]string{"query": "string"},
		Handler:    s.toolKnowledgeSearch,
	})
	register(&ToolSpec{
		Key: "ticket.list", Name: "查询当前用户工单", Category: "ticket", RiskLevel: domain.RiskLow,
		Description:  "查询当前用户自己提交过的工单与处理状态。",
		RequireLogin: true, OwnDataOnly: true, AllowAuto: true, RateLimit: 30,
		Handler: s.toolTicketList,
	})
	register(&ToolSpec{
		Key: "ticket.create", Name: "创建工单", Category: "ticket", RiskLevel: domain.RiskMedium,
		Description:  "为当前用户创建一张工单，把问题摘要交给人工客服。",
		RequireLogin: true, OwnDataOnly: true, AllowAuto: true, RateLimit: 10,
		AllowedParams:  []string{"subject", "content", "type", "order_no"},
		RequiredParams: []string{"subject"},
		ParamTypes:     map[string]string{"subject": "string", "content": "string", "type": "string", "order_no": "string"},
		Handler:        s.toolTicketCreate,
	})
	register(&ToolSpec{
		Key: "ticket.transfer", Name: "转人工", Category: "ticket", RiskLevel: domain.RiskMedium,
		Description:  "把当前会话或工单转给人工客服，并保留完整 AI 上下文。",
		RequireLogin: true, OwnDataOnly: true, AllowAuto: true, RateLimit: 10,
		AllowedParams: []string{"ticket_id", "reason"},
		ParamTypes:    map[string]string{"ticket_id": "int", "reason": "string"},
		Handler:       s.toolTicketTransfer,
	})
	register(&ToolSpec{
		Key: "ticket.add_message", Name: "添加工单消息", Category: "ticket", RiskLevel: domain.RiskMedium,
		Description:  "在当前用户自己的工单里追加一条消息。",
		RequireLogin: true, OwnDataOnly: true, AllowAuto: false, RequireConfirm: true, RateLimit: 20,
		AllowedParams: []string{"ticket_id", "content"}, RequiredParams: []string{"ticket_id", "content"},
		ParamTypes: map[string]string{"ticket_id": "int", "content": "string"},
		Handler:    s.toolTicketAddMessage,
	})
	register(&ToolSpec{
		Key: "ticket.update_status", Name: "更新工单状态", Category: "ticket", RiskLevel: domain.RiskHigh,
		Description:  "更新工单状态。高风险操作，需要管理员在后台显式放开并二次确认。",
		RequireLogin: true, OwnDataOnly: true, RequireConfirm: true, AllowAuto: false, RateLimit: 10,
		AllowedParams: []string{"ticket_id", "status"}, RequiredParams: []string{"ticket_id", "status"},
		ParamTypes: map[string]string{"ticket_id": "int", "status": "string"},
		Handler:    s.toolTicketUpdateStatus,
	})
	register(&ToolSpec{
		Key: "notification.send", Name: "发送站内通知", Category: "notification", RiskLevel: domain.RiskHigh,
		Description:  "向当前用户发送一条站内通知。高风险写操作，默认需要管理员放开。",
		RequireLogin: true, OwnDataOnly: true, RequireConfirm: true, AllowAuto: false, RateLimit: 10,
		AllowedParams: []string{"title", "content"}, RequiredParams: []string{"title", "content"},
		ParamTypes: map[string]string{"title": "string", "content": "string"},
		Handler:    s.toolNotify,
	})
	register(&ToolSpec{
		Key: "refund.list", Name: "查询可退款订单", Category: "refund", RiskLevel: domain.RiskLow,
		Description:  "列出当前用户自己已支付且尚未退款的订单，用于核对是否可以退。",
		RequireLogin: true, OwnDataOnly: true, AllowAuto: true, RateLimit: 20,
		Handler: s.toolRefundableOrders,
	})
	register(&ToolSpec{
		Key: "refund.order", Name: "发起订单退款", Category: "refund", RiskLevel: domain.RiskHigh,
		Description:  "对当前用户自己的一张已支付订单发起退款，原路退回其 NodeLoc 账户。高风险写操作，需要用户确认。",
		RequireLogin: true, OwnDataOnly: true, RequireConfirm: true, AllowAuto: true, RateLimit: 5,
		AllowedParams: []string{"order_no", "reason"}, RequiredParams: []string{"order_no"},
		ParamTypes: map[string]string{"order_no": "string", "reason": "string"},
		Handler:    s.toolRefundOrder,
	})
	register(&ToolSpec{
		Key: "coupon.grant", Name: "发放优惠券", Category: "coupon", RiskLevel: domain.RiskCritical,
		Description:  "向当前用户发放一张优惠券。高风险，默认关闭，需要二次确认与管理员审批。",
		RequireLogin: true, OwnDataOnly: true, RequireConfirm: true, AllowAuto: false, RateLimit: 5,
		AllowedParams: []string{"coupon_id"}, RequiredParams: []string{"coupon_id"},
		ParamTypes: map[string]string{"coupon_id": "int"},
		Handler:    s.toolGrantCoupon,
	})
}

// ToolSpecs 返回注册表里的所有工具，供后台同步到数据库。
func (s *Service) ToolSpecs() []*ToolSpec {
	return s.tools.All()
}

// ── 工具实现 ─────────────────────────────────────────────────────────

func (s *Service) toolUserProfile(ctx context.Context, access contract.Access, _ map[string]any) (any, error) {
	if s.users == nil || access.UserID == 0 {
		return nil, domain.ErrForbidden
	}
	profile, err := s.users.Context(ctx, access.UserID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"username": profile.Username, "nickname": profile.Nickname, "role": profile.Role,
		"is_active": profile.IsActive, "has_email": profile.HasEmail,
		"oauth_bound": profile.OAuthBound, "points": profile.Points,
	}, nil
}

func (s *Service) toolOrderList(ctx context.Context, access contract.Access, params map[string]any) (any, error) {
	if s.orders == nil || access.UserID == 0 {
		return nil, domain.ErrForbidden
	}
	limit := intParam(params, "limit", 5, 1, 20)
	orders, err := s.orders.OrdersForUser(ctx, access.UserID, limit)
	if err != nil {
		return nil, err
	}
	summaries := make([]map[string]any, 0, len(orders))
	for _, order := range orders {
		summaries = append(summaries, orderSummary(order))
	}
	return map[string]any{"orders": summaries}, nil
}

func (s *Service) toolOrderDetail(ctx context.Context, access contract.Access, params map[string]any) (any, error) {
	order, err := s.orderForUser(ctx, access, params)
	if err != nil {
		return nil, err
	}
	return orderDetail(*order), nil
}

func (s *Service) toolPaymentStatus(ctx context.Context, access contract.Access, params map[string]any) (any, error) {
	order, err := s.orderForUser(ctx, access, params)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"order_no": order.OrderNo, "status": order.Status,
		"paid_at": order.PaidAt, "total_amount": order.TotalAmount,
	}, nil
}

func (s *Service) toolDeliveryStatus(ctx context.Context, access contract.Access, params map[string]any) (any, error) {
	order, err := s.orderForUser(ctx, access, params)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"order_no": order.OrderNo, "fulfillment_status": order.FulfillmentStatus,
		"delivered_at": order.DeliveredAt, "delivery_summary": order.DeliverySummary,
	}, nil
}

// orderForUser 是越权查询的关口：订单不属于调用者时直接判为不可见。
func (s *Service) orderForUser(ctx context.Context, access contract.Access, params map[string]any) (*contract.OrderContext, error) {
	if s.orders == nil || access.UserID == 0 {
		return nil, domain.ErrForbidden
	}
	orderNo := stringParam(params, "order_no")
	if orderNo == "" {
		return nil, fmt.Errorf("%w: 请提供订单号。", domain.ErrInvalidInput)
	}
	order, err := s.orders.OrderForUser(ctx, access.UserID, orderNo)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, fmt.Errorf("%w: 没有找到属于你的这笔订单。", domain.ErrForbidden)
	}
	return order, nil
}

func (s *Service) toolProductDetail(ctx context.Context, access contract.Access, params map[string]any) (any, error) {
	if s.catalog == nil {
		return nil, domain.ErrNotConfigured
	}
	id := uint(intParam(params, "product_id", 0, 0, 1<<30))
	if id == 0 {
		return nil, fmt.Errorf("%w: 请提供商品编号。", domain.ErrInvalidInput)
	}
	fact, err := s.catalog.ProductFact(ctx, id)
	if err != nil {
		return nil, err
	}
	if fact == nil {
		return nil, fmt.Errorf("%w: 这个商品不存在。", domain.ErrInvalidInput)
	}
	return fact, nil
}

func (s *Service) toolProductList(ctx context.Context, access contract.Access, params map[string]any) (any, error) {
	if s.catalog == nil {
		return nil, domain.ErrNotConfigured
	}
	limit := intParam(params, "limit", 10, 1, 30)
	facts, err := s.catalog.ProductFacts(ctx, limit)
	if err != nil {
		return nil, err
	}
	return map[string]any{"products": facts}, nil
}

func (s *Service) toolActivityList(ctx context.Context, access contract.Access, _ map[string]any) (any, error) {
	if s.activities == nil {
		return nil, domain.ErrNotConfigured
	}
	facts, err := s.activities.ActiveActivities(ctx, 10)
	if err != nil {
		return nil, err
	}
	return map[string]any{"activities": facts}, nil
}

func (s *Service) toolKnowledgeSearch(ctx context.Context, access contract.Access, params map[string]any) (any, error) {
	query := stringParam(params, "query")
	if query == "" {
		return nil, fmt.Errorf("%w: 请提供要查询的问题。", domain.ErrInvalidInput)
	}
	hits, err := s.repo.SearchKnowledge(ctx, query, 5)
	if err != nil {
		return nil, err
	}
	trimmed := make([]map[string]any, 0, len(hits))
	for _, hit := range hits {
		content := hit.Content
		if len(content) > 600 {
			content = content[:600] + "…"
		}
		trimmed = append(trimmed, map[string]any{"title": hit.Title, "summary": hit.Summary, "content": content})
	}
	return map[string]any{"documents": trimmed}, nil
}

func (s *Service) toolTicketList(ctx context.Context, access contract.Access, _ map[string]any) (any, error) {
	if access.UserID == 0 {
		return nil, domain.ErrForbidden
	}
	views, _, err := s.repo.ListTickets(ctx, domain.TicketFilter{Limit: 10, Offset: 0})
	if err != nil {
		return nil, err
	}
	own := make([]map[string]any, 0, len(views))
	for _, view := range views {
		if view.UserID != access.UserID {
			continue
		}
		own = append(own, map[string]any{
			"ticket_no": view.TicketNo, "subject": view.Subject, "status": view.Status,
			"status_label": ticketStatusLabel(view.Status), "priority": view.Priority,
			"created_at": view.CreatedAt,
		})
		if len(own) >= 10 {
			break
		}
	}
	return map[string]any{"tickets": own}, nil
}

func (s *Service) toolTicketCreate(ctx context.Context, access contract.Access, params map[string]any) (any, error) {
	if access.UserID == 0 {
		return nil, domain.ErrForbidden
	}
	workflow, _ := s.Workflow(ctx)
	if workflow != nil && !workflow.CanCreateTicket {
		return nil, fmt.Errorf("%w: 当前配置不允许 AI 自动建单，请先转人工。", domain.ErrToolForbidden)
	}
	ticket, err := s.CreateTicket(ctx, CreateTicketInput{
		UserID:  access.UserID,
		Type:    stringParam(params, "type"),
		Subject: stringParam(params, "subject"),
		Content: stringParam(params, "content"),
		OrderNo: stringParam(params, "order_no"),
		Source:  "ai",
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"ticket_no": ticket.TicketNo, "ticket_id": ticket.ID, "status": ticket.Status}, nil
}

func (s *Service) toolTicketTransfer(ctx context.Context, access contract.Access, params map[string]any) (any, error) {
	if access.UserID == 0 {
		return nil, domain.ErrForbidden
	}
	ticketID := uint(intParam(params, "ticket_id", 0, 0, 1<<30))
	result, err := s.TransferToHuman(ctx, domain.TransferInput{
		TicketID: ticketID, UserID: access.UserID, Reason: stringParam(params, "reason"), Channel: domain.ChannelWidget,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"ticket_no": result.TicketNo, "summary": result.Summary}, nil
}

func (s *Service) toolTicketAddMessage(ctx context.Context, access contract.Access, params map[string]any) (any, error) {
	ticketID := uint(intParam(params, "ticket_id", 0, 0, 1<<30))
	content := stringParam(params, "content")
	ticket, err := s.repo.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if ticket.UserID != access.UserID {
		return nil, domain.ErrForbidden
	}
	if content == "" {
		return nil, fmt.Errorf("%w: 消息内容不能为空。", domain.ErrInvalidInput)
	}
	message := &domain.TicketMessage{
		TicketID: ticketID, SenderType: domain.SenderAI, Content: content, ContentType: "text",
	}
	if err := s.repo.CreateMessage(ctx, message); err != nil {
		return nil, err
	}
	return map[string]any{"message_id": message.ID}, nil
}

func (s *Service) toolTicketUpdateStatus(ctx context.Context, access contract.Access, params map[string]any) (any, error) {
	workflow, _ := s.Workflow(ctx)
	if workflow != nil && !workflow.CanUpdateTicket {
		return nil, fmt.Errorf("%w: 配置里没有允许 AI 修改工单状态。", domain.ErrToolForbidden)
	}
	ticketID := uint(intParam(params, "ticket_id", 0, 0, 1<<30))
	status := stringParam(params, "status")
	ticket, err := s.repo.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if ticket.UserID != access.UserID && !access.IsStaff {
		return nil, domain.ErrForbidden
	}
	if _, err := s.SetTicketStatus(ctx, ticketID, status, access.UserID, "ai", "AI 更新状态"); err != nil {
		return nil, err
	}
	return map[string]any{"ticket_no": ticket.TicketNo, "status": status}, nil
}

func (s *Service) toolNotify(ctx context.Context, access contract.Access, params map[string]any) (any, error) {
	workflow, _ := s.Workflow(ctx)
	if workflow != nil && !workflow.CanNotify {
		return nil, fmt.Errorf("%w: 配置里没有允许 AI 发送通知。", domain.ErrToolForbidden)
	}
	if s.notifier == nil || access.UserID == 0 {
		return nil, domain.ErrNotConfigured
	}
	title := stringParam(params, "title")
	content := stringParam(params, "content")
	if title == "" {
		return nil, fmt.Errorf("%w: 通知标题不能为空。", domain.ErrInvalidInput)
	}
	if err := s.notifier.NotifyUser(ctx, access.UserID, "ai_notice", title, content, ""); err != nil {
		return nil, err
	}
	return map[string]any{"sent": true}, nil
}

func (s *Service) toolGrantCoupon(ctx context.Context, access contract.Access, params map[string]any) (any, error) {
	workflow, _ := s.Workflow(ctx)
	if workflow != nil && !workflow.CanGrantCoupon {
		return nil, fmt.Errorf("%w: 配置里没有允许 AI 发放优惠券。", domain.ErrToolForbidden)
	}
	return nil, fmt.Errorf("%w: 优惠券发放需要人工在后台确认。", domain.ErrToolForbidden)
}

// toolRefundableOrders 列出该用户可退款的订单。只读，风险低。
func (s *Service) toolRefundableOrders(ctx context.Context, access contract.Access, _ map[string]any) (any, error) {
	if s.refunder == nil || access.UserID == 0 {
		return nil, domain.ErrForbidden
	}
	orders, err := s.refunder.RefundableOrders(ctx, access.UserID, 10)
	if err != nil {
		return nil, err
	}
	rows := make([]map[string]any, 0, len(orders))
	for _, order := range orders {
		rows = append(rows, map[string]any{
			"order_no": order.OrderNo, "product_name": order.ProductName,
			"total_amount": order.TotalAmount, "status": order.Status, "paid_at": order.PaidAt,
		})
	}
	return map[string]any{"orders": rows}, nil
}

// toolRefundOrder 发起退款。归属校验在 Refunder 内完成；这里只负责参数与结果形状。
// 退款成功后给买家发一条站内通知，让「钱去哪了」有据可查。
func (s *Service) toolRefundOrder(ctx context.Context, access contract.Access, params map[string]any) (any, error) {
	workflow, _ := s.Workflow(ctx)
	if workflow != nil && !workflow.CanRefund {
		return nil, fmt.Errorf("%w: 当前配置不允许 AI 直接退款，请转人工处理。", domain.ErrToolForbidden)
	}
	if s.refunder == nil || access.UserID == 0 {
		return nil, domain.ErrForbidden
	}
	orderNo := stringParam(params, "order_no")
	if orderNo == "" {
		return nil, fmt.Errorf("%w: 请提供要退款的订单号。", domain.ErrInvalidInput)
	}
	amount, status, err := s.refunder.RefundOrderForUser(ctx, access.UserID, orderNo)
	if err != nil {
		return nil, err
	}
	if s.notifier != nil {
		_ = s.notifier.NotifyUser(ctx, access.UserID, "order_refund",
			"订单 "+orderNo+" 已退款",
			fmt.Sprintf("退款 %d 已原路退回你的 NodeLoc 账户。", amount),
			"/orders/"+orderNo)
	}
	return map[string]any{"order_no": orderNo, "amount": amount, "status": status}, nil
}

// ── 参数校验 ─────────────────────────────────────────────────────────

// ValidateParams 做参数白名单与类型校验。未知参数会被拒绝而不是被忽略，
// 避免 AI（或被注入的提示）偷偷塞进越权字段。
func ValidateParams(spec *ToolSpec, params map[string]any) (map[string]any, error) {
	if spec == nil {
		return nil, domain.ErrToolNotFound
	}
	allowed := map[string]bool{}
	for _, key := range spec.AllowedParams {
		allowed[key] = true
	}
	clean := make(map[string]any, len(params))
	for key, value := range params {
		if len(spec.AllowedParams) > 0 && !allowed[key] {
			return nil, fmt.Errorf("%w: 工具 %s 不接受参数 %s", domain.ErrInvalidInput, spec.Key, key)
		}
		expected := spec.ParamTypes[key]
		switch expected {
		case "int":
			number, ok := toInt(value)
			if !ok {
				return nil, fmt.Errorf("%w: 参数 %s 需要是整数。", domain.ErrInvalidInput, key)
			}
			clean[key] = number
		case "string":
			text, ok := value.(string)
			if !ok {
				text = fmt.Sprintf("%v", value)
			}
			text = strings.TrimSpace(text)
			if len(text) > 2000 {
				return nil, fmt.Errorf("%w: 参数 %s 太长。", domain.ErrInvalidInput, key)
			}
			clean[key] = text
		default:
			clean[key] = value
		}
	}
	for _, key := range spec.RequiredParams {
		value, ok := clean[key]
		if !ok {
			return nil, fmt.Errorf("%w: 缺少参数 %s", domain.ErrInvalidInput, key)
		}
		if text, isString := value.(string); isString && text == "" {
			return nil, fmt.Errorf("%w: 缺少参数 %s", domain.ErrInvalidInput, key)
		}
	}
	return clean, nil
}

func toInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func intParam(params map[string]any, key string, fallback, min, max int) int {
	value, ok := params[key]
	if !ok {
		return fallback
	}
	number, ok := toInt(value)
	if !ok {
		return fallback
	}
	if number < min {
		return min
	}
	if max > 0 && number > max {
		return max
	}
	return number
}

func stringParam(params map[string]any, key string) string {
	value, ok := params[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		text = fmt.Sprintf("%v", value)
	}
	return strings.TrimSpace(text)
}

// ── 订单字段摘要 ─────────────────────────────────────────────────────

func orderSummary(order contract.OrderContext) map[string]any {
	return map[string]any{
		"order_no": order.OrderNo, "product_name": order.ProductName,
		"quantity": order.Quantity, "total_amount": order.TotalAmount,
		"status": order.Status, "fulfillment_status": order.FulfillmentStatus,
	}
}

func orderDetail(order contract.OrderContext) map[string]any {
	detail := orderSummary(order)
	detail["paid_at"] = order.PaidAt
	detail["delivered_at"] = order.DeliveredAt
	detail["delivery_summary"] = order.DeliverySummary
	return detail
}

// ── 错误分类 ─────────────────────────────────────────────────────────

// toolErrorCode 把工具错误翻成机器码，写进审计日志。
func toolErrorCode(err error) string {
	switch {
	case errors.Is(err, domain.ErrToolNotFound):
		return "tool_not_found"
	case errors.Is(err, domain.ErrToolDisabled):
		return "tool_disabled"
	case errors.Is(err, domain.ErrToolForbidden):
		return "tool_forbidden"
	case errors.Is(err, domain.ErrToolConfirm):
		return "need_confirm"
	case errors.Is(err, domain.ErrToolRateLimited):
		return "rate_limited"
	case errors.Is(err, domain.ErrInvalidInput):
		return "invalid_params"
	case errors.Is(err, domain.ErrForbidden):
		return "forbidden"
	case errors.Is(err, domain.ErrNotConfigured):
		return "not_configured"
	default:
		return "failed"
	}
}

// CallTool 是唯一的工具执行入口，所有安全校验都在这里。
func (s *Service) CallTool(ctx context.Context, input domain.ToolCallInput) (*domain.ToolCallResult, error) {
	started := time.Now()
	result := &domain.ToolCallResult{
		ToolKey: input.ToolKey, Status: "ok", RiskLevel: domain.RiskLow,
	}
	call := &domain.AIToolCall{
		ToolKey: input.ToolKey,
		Params:  redact(jsonString(input.Params, 2000), 2000),
		Status:  "ok",
	}
	if input.UserID > 0 {
		call.UserID = &input.UserID
	}
	if input.ConversationID > 0 {
		call.ConversationID = &input.ConversationID
	}
	if input.TicketID > 0 {
		call.TicketID = &input.TicketID
	}
	call.IP = input.IP

	record := func(err error) {
		result.DurationMS = time.Since(started).Milliseconds()
		call.DurationMS = result.DurationMS
		call.RiskLevel = result.RiskLevel
		if err != nil {
			call.Status = "failed"
			call.Error = redact(err.Error(), 500)
			result.Status = "failed"
			result.Error = toolErrorCode(err)
		}
		if persistErr := s.repo.CreateToolCall(ctx, call); persistErr != nil {
			logf("tool call audit write failed: %v", persistErr)
		}
	}

	spec, ok := s.tools.Lookup(input.ToolKey)
	if !ok {
		record(domain.ErrToolNotFound)
		return result, domain.ErrToolNotFound
	}
	result.ToolName = spec.Name
	result.RiskLevel = spec.RiskLevel

	// 数据库里的开关与管理员的角色授权是两道独立门槛，必须都通过。
	stored, err := s.repo.GetTool(ctx, spec.Key)
	if err != nil {
		record(err)
		return result, err
	}
	if !stored.IsEnabled {
		record(domain.ErrToolDisabled)
		return result, domain.ErrToolDisabled
	}
	if spec.RequireLogin && input.UserID == 0 {
		record(domain.ErrForbidden)
		return result, domain.ErrForbidden
	}
	if !spec.AllowAuto {
		// 管理员可以在工具页显式放开自动调用；否则必须二次确认。
		if !stored.AllowAuto {
			if !input.Confirmed {
				result.RequireConfirm = true
				record(domain.ErrToolConfirm)
				return result, domain.ErrToolConfirm
			}
		}
	} else if stored.RequireConfirm && !input.Confirmed {
		result.RequireConfirm = true
		record(domain.ErrToolConfirm)
		return result, domain.ErrToolConfirm
	}
	if ok, err := s.toolAllowedForRole(ctx, stored, input.UserRole); err != nil {
		record(err)
		return result, err
	} else if !ok {
		record(domain.ErrToolForbidden)
		return result, domain.ErrToolForbidden
	}
	if spec.RateLimit > 0 {
		since := time.Now().Add(-time.Minute)
		count, err := s.repo.CountToolCalls(ctx, spec.Key, input.UserID, since)
		if err != nil {
			record(err)
			return result, err
		}
		if count >= int64(spec.RateLimit) {
			record(domain.ErrToolRateLimited)
			return result, domain.ErrToolRateLimited
		}
	}

	params, err := ValidateParams(spec, input.Params)
	if err != nil {
		record(err)
		return result, err
	}

	// 每个工具都有自己的超时，避免一次慢查询拖住整段对话。
	timeout := spec.TimeoutMS
	if timeout <= 0 {
		timeout = 8000
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	access := contract.Access{UserID: input.UserID, UserRole: input.UserRole, IsStaff: isStaffRole(input.UserRole)}
	data, err := spec.Handler(runCtx, access, params)
	if err != nil {
		result.DurationMS = time.Since(started).Milliseconds()
		call.DurationMS = result.DurationMS
		call.Status = "failed"
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			call.Error = "timeout"
			result.Error = "timeout"
		default:
			call.Error = redact(err.Error(), 500)
			result.Error = toolErrorCode(err)
		}
		result.Status = "failed"
		call.RiskLevel = result.RiskLevel
		if persistErr := s.repo.CreateToolCall(ctx, call); persistErr != nil {
			logf("tool call audit write failed: %v", persistErr)
		}
		return result, err
	}

	result.Data = data
	result.DurationMS = time.Since(started).Milliseconds()
	call.Result = redact(jsonString(data, 2000), 2000)
	call.DurationMS = result.DurationMS
	call.RiskLevel = result.RiskLevel
	if persistErr := s.repo.CreateToolCall(ctx, call); persistErr != nil {
		logf("tool call audit write failed: %v", persistErr)
	}
	return result, nil
}

// toolAllowedForRole 读取 AI 工具权限表；没有配置时按工具自身默认放行规则处理。
func (s *Service) toolAllowedForRole(ctx context.Context, tool *domain.AIToolDefinition, role string) (bool, error) {
	permissions, err := s.repo.ListToolPermissions(ctx)
	if err != nil {
		return false, err
	}
	if role == "" {
		role = "guest"
	}
	for _, permission := range permissions {
		if permission.ToolID == tool.ID && permission.Role == role {
			return permission.Allowed, nil
		}
	}
	// 高风险工具在没有显式授权时默认拒绝。
	if tool.RiskLevel == domain.RiskHigh || tool.RiskLevel == domain.RiskCritical {
		return false, nil
	}
	return true, nil
}

func isStaffRole(role string) bool {
	switch role {
	case "super_admin", "admin", "operator", "support", "ops_manager", "product_manager",
		"order_manager", "finance", "support_lead", "support_agent", "ai_admin", "data_viewer":
		return true
	}
	return false
}

// ticketStatusLabel 给 AI 的返回补上中文状态名。
func ticketStatusLabel(status string) string {
	if label, ok := ticketStatusLabels[status]; ok {
		return label
	}
	return status
}

var ticketStatusLabels = map[string]string{
	"ai_processing": "AI 处理中", "waiting_user": "等待用户回复", "ai_solved": "AI 已解决",
	"user_requested_human": "用户申请人工", "pending_human": "待人工处理",
	"human_handling": "人工处理中", "waiting_confirm": "等待用户确认",
	"resolved": "已解决", "closed": "已关闭", "rejected": "已拒绝", "cancelled": "已撤销",
}

// toContract 把 HTTP 层筛选参数转成仓储层结构。
func (f ToolCallFilterInput) toContract() contract.ToolCallFilter {
	return contract.ToolCallFilter{
		ToolKey: strings.TrimSpace(f.ToolKey),
		Status:  strings.TrimSpace(f.Status),
		UserID:  f.UserID,
		Limit:   f.Limit,
		Offset:  f.Offset,
	}
}
