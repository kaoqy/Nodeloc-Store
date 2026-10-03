package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/domain"
)

// ── 工单 ─────────────────────────────────────────────────────────────

// CreateTicketInput 是创建工单的输入。
type CreateTicketInput struct {
	UserID      uint
	Type        string
	Subject     string
	Content     string
	OrderID     *uint
	OrderNo     string
	ProductID   *uint
	ProductName string
	Priority    string
	Source      string
	Contact     string
}

// CreateTicket 建单后进入人工待处理队列：写入买家首条消息、留下创建日志，
// 并给买家发一条「已提交」通知。AI 接待已下线，工单默认由人工跟进。
func (s *Service) CreateTicket(ctx context.Context, input CreateTicketInput) (*domain.Ticket, error) {
	subject := strings.TrimSpace(input.Subject)
	if subject == "" {
		return nil, fmt.Errorf("%w: 工单标题不能为空。", domain.ErrInvalidInput)
	}
	if len([]rune(subject)) > 120 {
		return nil, fmt.Errorf("%w: 工单标题最多 120 个字。", domain.ErrInvalidInput)
	}
	if input.UserID == 0 {
		return nil, fmt.Errorf("%w: 请先登录再提交工单。", domain.ErrInvalidInput)
	}
	ticketType := strings.TrimSpace(input.Type)
	if ticketType == "" {
		ticketType = "other"
	}
	priority := strings.TrimSpace(input.Priority)
	if priority == "" {
		priority = "normal"
	}
	if !validPriority(priority) {
		return nil, fmt.Errorf("%w: 工单优先级无效。", domain.ErrInvalidInput)
	}
	// 工单编号前缀由配置中心管理，店家可以改成自己的习惯（例如 SHOP）。
	prefix := s.SystemConfigValue(ctx, "ticket", "ticket_no_prefix", "TK")
	no, err := s.repo.NextTicketNo(ctx, prefix)
	if err != nil {
		return nil, err
	}
	due := s.now().UTC()
	estimate := s.SystemConfigInt(ctx, "ticket", "human_timeout_hours", 0)
	if estimate > 0 {
		due = due.Add(time.Duration(estimate) * time.Hour)
	}
	// 人工优先：新单一律进待人工队列，AI 不再自动接待。
	// AIEnabled 置 false，工单里的自动应答分支因此不会触发。
	ticket := &domain.Ticket{
		TicketNo: no, UserID: input.UserID, Type: ticketType, Subject: subject,
		Content: input.Content, Status: models.TicketStatusPendingHuman, Handler: models.TicketHandlerHuman,
		Priority: priority, AIEnabled: false, Source: input.Source, DueAt: &due,
		UnreadForStaff: true,
	}
	if ticket.Source == "" {
		ticket.Source = "web"
	}
	if input.OrderID != nil {
		ticket.OrderID = input.OrderID
	}
	if input.OrderNo != "" {
		ticket.OrderNo = strings.TrimSpace(input.OrderNo)
	}
	if input.ProductID != nil {
		ticket.ProductID = input.ProductID
		ticket.ProductName = input.ProductName
	}
	if input.Contact != "" {
		ticket.CustomerContact = input.Contact
	}
	if err := s.repo.CreateTicket(ctx, ticket); err != nil {
		return nil, err
	}
	initial := &domain.TicketMessage{
		TicketID: ticket.ID, SenderType: domain.SenderUser, SenderID: &input.UserID,
		Content: input.Content, ContentType: "text",
	}
	if initial.Content == "" {
		initial.Content = subject
	}
	if err := s.repo.CreateMessage(ctx, initial); err != nil {
		return nil, err
	}
	// 不再为新单创建 TicketAISession，也不再调用 AI 首答。
	// TicketAISession 表保留（工单表有外键关联），只停止读写。
	s.notify(ctx, "ticket.created", map[string]string{
		"ticket_no": ticket.TicketNo, "subject": ticket.Subject, "user_name": fmt.Sprintf("用户 #%d", input.UserID),
	}, Notification{
		UserID: input.UserID, TicketID: ticket.ID,
		Title:   "工单 " + ticket.TicketNo + " 已创建",
		Content: "客服会尽快跟进你的问题，你可以在「我的工单」里查看处理进度。",
	})
	return s.repo.GetTicket(ctx, ticket.ID)
}

func validPriority(value string) bool {
	switch value {
	case "low", "normal", "high", "urgent":
		return true
	}
	return false
}

// ListTickets 是后台工单列表。
func (s *Service) ListTickets(ctx context.Context, filter domain.TicketFilter) ([]domain.TicketView, int64, error) {
	return s.repo.ListTickets(ctx, filter)
}

// TicketDetail 组装工作台需要的所有上下文。includeInternal 决定是否带内部备注。
func (s *Service) TicketDetail(ctx context.Context, id uint, includeInternal bool) (*domain.TicketDetail, error) {
	ticket, err := s.repo.GetTicket(ctx, id)
	if err != nil {
		return nil, err
	}
	detail := &domain.TicketDetail{Ticket: *ticket}
	if messages, _, err := s.repo.ListMessages(ctx, id, includeInternal, 300, 0); err == nil {
		detail.Messages = messages
	} else {
		return nil, err
	}
	if logs, _, err := s.repo.ListTicketLogs(ctx, id, 200, 0); err == nil {
		detail.Logs = logs
	}
	if assignments, err := s.repo.ListAssignments(ctx, id); err == nil {
		detail.Assignments = assignments
	}
	if tickets, err := s.repo.RelatedTickets(ctx, ticket.UserID, id, 5); err == nil {
		detail.Related = tickets
	}
	if replies, err := s.repo.ListQuickReplies(ctx, true, ticket.Type, ""); err == nil {
		detail.QuickReplies = replies
	}
	if detail.Username == "" {
		detail.Username = fmt.Sprintf("用户 #%d", ticket.UserID)
	}
	return detail, nil
}

// SetTicketStatus 修改工单状态并写日志。状态白名单在前，避免任意字符串入库。
func (s *Service) SetTicketStatus(ctx context.Context, id uint, status string, actorID uint, actorType, detail string) (*domain.Ticket, error) {
	status = strings.TrimSpace(status)
	if !validStatus(status) {
		return nil, fmt.Errorf("%w: 工单状态无效。", domain.ErrInvalidInput)
	}
	ticket, err := s.repo.GetTicket(ctx, id)
	if err != nil {
		return nil, err
	}
	before := jsonString(map[string]any{"status": ticket.Status, "handler": ticket.Handler}, 2000)
	ticket.Status = status
	switch status {
	case models.TicketStatusPendingHuman, models.TicketStatusUserRequested:
		ticket.Handler = models.TicketHandlerHuman
	case models.TicketStatusHumanHandling, models.TicketStatusWaitingConfirm:
		ticket.Handler = models.TicketHandlerHuman
	case models.TicketStatusAIProcessing, models.TicketStatusWaitingUser, models.TicketStatusAISolved:
		ticket.Handler = models.TicketHandlerAI
	}
	if status == models.TicketStatusResolved {
		now := s.now().UTC()
		ticket.ResolvedAt = &now
	}
	if status == models.TicketStatusClosed {
		now := s.now().UTC()
		ticket.ClosedAt = &now
	}
	if err := s.repo.UpdateTicket(ctx, ticket); err != nil {
		return nil, err
	}
	entry := &domain.TicketLog{
		TicketID: id, ActorType: actorType, Action: "ticket.status",
		Before: before, After: jsonString(map[string]any{"status": ticket.Status, "handler": ticket.Handler}, 2000),
		Detail: detail, Result: "ok",
	}
	if actorID > 0 {
		entry.ActorID = &actorID
	}
	if err := s.repo.AppendTicketLog(ctx, entry); err != nil {
		logf("ticket %d: status log failed: %v", id, err)
	}
	return ticket, nil
}

func validStatus(value string) bool {
	_, ok := ticketStatusLabels[value]
	return ok
}

// AssignAgent 把工单交给某个客服，并在同一事务语义下写分配记录。
func (s *Service) AssignAgent(ctx context.Context, ticketID, agentID, actorID uint, strategy string) (*domain.Ticket, error) {
	ticket, err := s.repo.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	agent, err := s.repo.GetAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if !agent.IsActive {
		return nil, fmt.Errorf("%w: 这个客服账号已停用。", domain.ErrInvalidInput)
	}
	before := jsonString(map[string]any{"assigned_agent_id": ticket.AssignedAgentID}, 2000)
	ticket.AssignedAgentID = &agentID
	if ticket.Status == models.TicketStatusPendingHuman || ticket.Status == models.TicketStatusUserRequested {
		ticket.Status = models.TicketStatusHumanHandling
	}
	ticket.Handler = models.TicketHandlerHuman
	if err := s.repo.UpdateTicket(ctx, ticket); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	assignment := &domain.CustomerServiceAssignment{
		TicketID: ticketID, AgentID: agentID, Strategy: strategy, Status: "assigned", AssignedAt: now,
	}
	if actorID > 0 {
		assignment.AssignedBy = &actorID
	}
	if err := s.repo.CreateAssignment(ctx, assignment); err != nil {
		logf("ticket %d: assignment record failed: %v", ticketID, err)
	}
	if err := s.repo.AppendTicketLog(ctx, &domain.TicketLog{
		TicketID: ticketID, ActorID: &actorID, ActorType: "agent", Action: "ticket.assign",
		Before: before, After: jsonString(map[string]any{"assigned_agent_id": agentID}, 2000), Result: "ok",
	}); err != nil {
		logf("ticket %d: assignment log failed: %v", ticketID, err)
	}
	return ticket, nil
}

// AutoAssign 按客服在线状态与并发量挑选一个客服，满了就留在待人工队列。
func (s *Service) AutoAssign(ctx context.Context, ticketID uint, actorID uint) (*domain.Ticket, error) {
	agents, err := s.repo.ListAgents(ctx, true)
	if err != nil {
		return nil, err
	}
	best := uint(0)
	bestLoad := int64(1 << 30)
	for _, agent := range agents {
		if !agent.AcceptManual || agent.Status == "offline" {
			continue
		}
		load, err := s.repo.AgentLoad(ctx, agent.ID)
		if err != nil {
			continue
		}
		if int(load) >= agent.MaxConcurrent && agent.MaxConcurrent > 0 {
			continue
		}
		if load < bestLoad {
			bestLoad = load
			best = agent.ID
		}
	}
	if best == 0 {
		if _, err := s.SetTicketStatus(ctx, ticketID, models.TicketStatusPendingHuman, actorID, "system", "暂无空闲客服，留在待人工队列"); err != nil {
			return nil, err
		}
		return nil, domain.ErrHumanUnavailable
	}
	return s.AssignAgent(ctx, ticketID, best, actorID, "auto")
}

// TransferToHuman 把 AI 会话或工单转人工，保留完整上下文并生成摘要。
func (s *Service) TransferToHuman(ctx context.Context, input domain.TransferInput) (*domain.TransferResult, error) {
	if input.UserID == 0 {
		return nil, fmt.Errorf("%w: 请先登录再转人工。", domain.ErrInvalidInput)
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		reason = "用户请求人工客服"
	}
	var ticket *domain.Ticket
	if input.TicketID > 0 {
		existing, err := s.repo.GetTicket(ctx, input.TicketID)
		if err != nil {
			return nil, err
		}
		if existing.UserID != input.UserID {
			return nil, domain.ErrForbidden
		}
		ticket = existing
	} else {
		created, err := s.CreateTicket(ctx, CreateTicketInput{
			UserID: input.UserID, Type: "other", Subject: "转人工客服",
			Content: "用户请求转人工客服。", Source: input.Channel,
		})
		if err != nil {
			return nil, err
		}
		ticket = created
	}
	summary, plan := s.summarize(ctx, ticket)
	before := jsonString(map[string]any{"status": ticket.Status}, 2000)
	ticket.Status = models.TicketStatusPendingHuman
	ticket.Handler = models.TicketHandlerHuman
	ticket.TransferReason = reason
	ticket.Summary = summary
	ticket.SuggestedPlan = plan
	if err := s.repo.UpdateTicket(ctx, ticket); err != nil {
		return nil, err
	}
	message := &domain.TicketMessage{
		TicketID: ticket.ID, SenderType: domain.SenderSystem,
		Content: "用户申请人工跟进。原因：" + reason, ContentType: "text",
	}
	_ = s.repo.CreateMessage(ctx, message)
	_ = s.repo.AppendTicketLog(ctx, &domain.TicketLog{
		TicketID: ticket.ID, ActorType: "user", Action: "ticket.transfer",
		Before: before, After: jsonString(map[string]any{"status": ticket.Status}, 2000),
		Detail: reason, Result: "ok",
	})
	// 转人工后把工单交给在线客服；没有空闲客服就留在待人工队列。
	if _, err := s.AutoAssign(ctx, ticket.ID, 0); err != nil && !errors.Is(err, domain.ErrHumanUnavailable) {
		logf("ticket %s: auto assign failed: %v", ticket.TicketNo, err)
	}
	result := &domain.TransferResult{
		TicketNo: ticket.TicketNo, Summary: summary, SuggestedPlan: plan,
		EstimateMinutes: s.SystemConfigInt(ctx, "ticket", "estimate_reply_minutes", 30),
		WorkingHours:    s.SystemConfigValue(ctx, "ticket", "working_hours", "每天 09:00 - 21:00"),
	}
	if result.EstimateMinutes <= 0 {
		result.EstimateMinutes = 30
	}
	s.notify(ctx, "ticket.transferred", map[string]string{
		"ticket_no": ticket.TicketNo, "agent_name": "人工客服",
	}, Notification{
		UserID: input.UserID, TicketID: ticket.ID,
		Title:   "工单 " + ticket.TicketNo + " 已转人工",
		Content: "客服会尽快跟进，你的历史留言已经一并转过去了。",
	})
	fresh, err := s.repo.GetTicket(ctx, ticket.ID)
	if err == nil {
		ticket = fresh
	}
	result.Ticket = ticket
	return result, nil
}

// summarize 生成问题摘要与推荐处理方案，给接手的人工客服看。
func (s *Service) summarize(ctx context.Context, ticket *domain.Ticket) (string, string) {
	parts := []string{fmt.Sprintf("工单 %s（%s）", ticket.TicketNo, ticket.Subject)}
	if ticket.OrderNo != "" {
		parts = append(parts, "关联订单 "+ticket.OrderNo)
	}
	if ticket.ProductName != "" {
		parts = append(parts, "商品 "+ticket.ProductName)
	}
	if ticket.Content != "" {
		parts = append(parts, "用户描述："+truncate(ticket.Content, 200))
	}
	if messages, _, err := s.repo.ListMessages(ctx, ticket.ID, false, 20, 0); err == nil {
		count := 0
		for _, message := range messages {
			if message.SenderType == domain.SenderUser && message.Content != "" {
				count++
			}
		}
		if count > 0 {
			parts = append(parts, fmt.Sprintf("用户共发送 %d 条消息", count))
		}
	}
	plan := "先核对订单与发货状态，再按知识库售后规则给出处理方案；涉及退款请走后台退款流程。"
	switch ticket.Type {
	case "refund":
		plan = "核对订单支付与发货状态，按退款规则确认是否可退；退款需要在后台订单页由有权限的账号执行。"
	case "card":
		plan = "核对卡密交付记录，确认是否为重复卡或无效卡；不向用户展示完整卡密，必要时重新发货。"
	case "payment":
		plan = "核对支付流水与订单状态，区分未支付、支付未回调与掉单，再决定重试查单或人工补单。"
	}
	return strings.Join(parts, "；"), plan
}

func truncate(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}

// TicketStats 是工单中心的统计卡。
func (s *Service) TicketStats(ctx context.Context, agentID uint) (*domain.TicketStats, error) {
	return s.repo.TicketStats(ctx, agentID)
}

// AddMessage 是客服或系统在工单里发消息，内部备注只给后台看。
func (s *Service) AddMessage(ctx context.Context, ticketID uint, senderType string, senderID uint, senderName, content string, internal bool) (*domain.TicketMessage, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("%w: 消息内容不能为空。", domain.ErrInvalidInput)
	}
	if len([]rune(content)) > 4000 {
		return nil, fmt.Errorf("%w: 消息太长了。", domain.ErrInvalidInput)
	}
	ticket, err := s.repo.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	message := &domain.TicketMessage{
		TicketID: ticketID, SenderType: senderType, SenderName: senderName,
		Content: content, ContentType: "text", IsInternal: internal,
	}
	if senderID > 0 {
		message.SenderID = &senderID
	}
	if err := s.repo.CreateMessage(ctx, message); err != nil {
		return nil, err
	}
	if !internal {
		// 客服回复后把状态推进到「人工处理中」，用户回复则等客服再看。
		switch senderType {
		case domain.SenderAgent:
			if ticket.Status == models.TicketStatusPendingHuman || ticket.Status == models.TicketStatusUserRequested {
				_, _ = s.SetTicketStatus(ctx, ticketID, models.TicketStatusHumanHandling, senderID, "agent", "客服首次回复")
			}
			now := s.now().UTC()
			if ticket.FirstResponseAt == nil {
				ticket.FirstResponseAt = &now
				_ = s.repo.UpdateTicket(ctx, ticket)
			}
		case domain.SenderUser:
			if ticket.Status == models.TicketStatusResolved || ticket.Status == models.TicketStatusClosed {
				_, _ = s.SetTicketStatus(ctx, ticketID, models.TicketStatusHumanHandling, senderID, "user", "用户重新打开工单")
			} else if ticket.Status == models.TicketStatusWaitingConfirm {
				_, _ = s.SetTicketStatus(ctx, ticketID, models.TicketStatusHumanHandling, senderID, "user", "用户回复工单")
			}
		}
	}
	_ = s.repo.AppendTicketLog(ctx, &domain.TicketLog{
		TicketID: ticketID, ActorType: senderType, Action: "ticket.message",
		Detail: truncate(content, 120), Result: "ok",
	})
	return message, nil
}

// RequireTicketAccess 校验一个用户能否看到这张工单。买家只能看自己的；
// 客服需要工单分配给自己或是主管/管理员。
func (s *Service) RequireTicketAccess(ctx context.Context, ticketID, userID uint, role string) (*domain.Ticket, error) {
	ticket, err := s.repo.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if ticket.UserID == userID {
		return ticket, nil
	}
	switch role {
	case "super_admin", "admin", "support_lead":
		return ticket, nil
	case "support", "support_agent", "operator", "ops_manager":
		if ticket.AssignedAgentID != nil {
			if agent, err := s.repo.GetAgentByUser(ctx, userID); err == nil && agent != nil && agent.ID == *ticket.AssignedAgentID {
				return ticket, nil
			}
		}
		// 未分配的待人工工单对所有客服可见，便于抢单。
		if ticket.AssignedAgentID == nil {
			return ticket, nil
		}
		return nil, domain.ErrForbidden
	}
	return nil, domain.ErrForbidden
}

// MarkTicketRead 清掉后台未读标记，动的是列而不是整行，避免把别的字段写坏。
func (s *Service) MarkTicketRead(ctx context.Context, ticketID uint) error {
	ticket, err := s.repo.GetTicket(ctx, ticketID)
	if err != nil {
		return err
	}
	ticket.UnreadForStaff = false
	return s.repo.UpdateTicket(ctx, ticket)
}

// MarkUserRead 清掉买家侧的未读标记。
func (s *Service) MarkUserRead(ctx context.Context, ticketID uint) error {
	ticket, err := s.repo.GetTicket(ctx, ticketID)
	if err != nil {
		return err
	}
	ticket.UnreadForUser = false
	return s.repo.UpdateTicket(ctx, ticket)
}

// HandleTicketMessage 是买家在工单里继续追问时的入口。AI 客服已下线，
// 消息写入后直接留给人工跟进，不再触发自动应答。
func (s *Service) HandleTicketMessage(ctx context.Context, ticketID, userID uint, content string) (*domain.TicketMessage, bool, error) {
	message, err := s.AddMessage(ctx, ticketID, domain.SenderUser, userID, "", content, false)
	if err != nil {
		return nil, false, err
	}
	return message, false, nil
}

// SummaryForTicket 给客服看的问题摘要，转人工时调用。
func (s *Service) SummaryForTicket(ctx context.Context, ticketID uint) (string, string, error) {
	ticket, err := s.repo.GetTicket(ctx, ticketID)
	if err != nil {
		return "", "", err
	}
	summary, plan := s.summarize(ctx, ticket)
	return summary, plan, nil
}

// ticketStatusLabel 给工单返回补上中文状态名。
func ticketStatusLabel(status string) string {
	if label, ok := ticketStatusLabels[status]; ok {
		return label
	}
	return status
}

var ticketStatusLabels = map[string]string{
	"ai_processing": "处理中（历史）", "waiting_user": "等待用户回复", "ai_solved": "已解决（历史）",
	"user_requested_human": "用户申请人工", "pending_human": "待人工处理",
	"human_handling": "人工处理中", "waiting_confirm": "等待用户确认",
	"resolved": "已解决", "closed": "已关闭", "rejected": "已拒绝", "cancelled": "已撤销",
}
