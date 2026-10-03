package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/domain"
)

// ── 工单配置 ─────────────────────────────────────────────────────────

// Config 读取 AI 配置与工作流配置；两张表都没有记录时给出默认值，
// 保证第一次打开后台就能看到一份可编辑的配置。
func (s *Service) Config(ctx context.Context) (*domain.AIConfig, error) {
	config, err := s.repo.GetAIConfig(ctx)
	if err != nil {
		return nil, err
	}
	if config.ID == 0 {
		defaults := DefaultAIConfig()
		return &defaults, nil
	}
	return config, nil
}

func (s *Service) Workflow(ctx context.Context) (*domain.AIWorkflowConfig, error) {
	config, err := s.repo.GetAIWorkflow(ctx)
	if err != nil {
		return nil, err
	}
	if config.ID == 0 {
		defaults := DefaultAIWorkflow()
		return &defaults, nil
	}
	return config, nil
}

// SaveConfig 保存 AI 配置。API Key 传空表示保持原值，传 __clear__ 表示清除。
func (s *Service) SaveConfig(ctx context.Context, config domain.AIConfig) (*domain.AIConfig, error) {
	existing, err := s.repo.GetAIConfig(ctx)
	if err != nil {
		return nil, err
	}
	config.ID = existing.ID
	config.Base = existing.Base
	if config.APIKeyEnc == "" {
		config.APIKeyEnc = existing.APIKeyEnc
	} else if config.APIKeyEnc == "__clear__" {
		config.APIKeyEnc = ""
	} else {
		encrypted, err := s.EncryptSecret(config.APIKeyEnc)
		if err != nil {
			return nil, err
		}
		config.APIKeyEnc = encrypted
	}
	config.Provider = strings.TrimSpace(config.Provider)
	if config.Provider == "" {
		config.Provider = "openai-compatible"
	}
	if config.TimeoutMS < 1000 || config.TimeoutMS > 120000 {
		return nil, fmt.Errorf("%w: 请求超时要在 1 到 120 秒之间。", domain.ErrInvalidInput)
	}
	if config.MaxContext <= 0 || config.MaxContext > 60 {
		return nil, fmt.Errorf("%w: 最大上下文条数要在 1 到 60 之间。", domain.ErrInvalidInput)
	}
	if config.Temperature < 0 || config.Temperature > 2 {
		return nil, fmt.Errorf("%w: 温度参数要在 0 到 2 之间。", domain.ErrInvalidInput)
	}
	if config.TopP <= 0 || config.TopP > 1 {
		return nil, fmt.Errorf("%w: Top P 要在 0 到 1 之间。", domain.ErrInvalidInput)
	}
	if config.MaxMessageLen <= 0 {
		config.MaxMessageLen = 2000
	}
	if config.AgentName == "" {
		config.AgentName = "智能客服"
	}
	if config.IsEnabled {
		if strings.TrimSpace(config.BaseURL) == "" || strings.TrimSpace(config.Model) == "" || config.APIKeyEnc == "" {
			return nil, fmt.Errorf("%w: 启用 AI 客服前要填好 API 地址、模型名称和 API Key。", domain.ErrInvalidInput)
		}
	}
	if err := s.repo.SaveAIConfig(ctx, &config); err != nil {
		return nil, err
	}
	saved, err := s.repo.GetAIConfig(ctx)
	if err != nil {
		return nil, err
	}
	return s.maskConfig(saved), nil
}

// maskConfig 去掉密钥明文，只告诉前端「是否已配置」。
func (s *Service) maskConfig(config *domain.AIConfig) *domain.AIConfig {
	if config == nil {
		return nil
	}
	masked := *config
	hasKey := masked.APIKeyEnc != ""
	masked.APIKeyEnc = ""
	if hasKey {
		masked.APIKeyEnc = "__saved__"
	}
	return &masked
}

// ConfigForAdmin 返回给后台的配置视图（密钥只回 has_key 标记）。
func (s *Service) ConfigForAdmin(ctx context.Context) (map[string]any, error) {
	config, err := s.Config(ctx)
	if err != nil {
		return nil, err
	}
	workflow, err := s.Workflow(ctx)
	if err != nil {
		return nil, err
	}
	hasKey := config.APIKeyEnc != "" && config.APIKeyEnc != "__saved__"
	config.APIKeyEnc = ""
	return map[string]any{"config": config, "workflow": workflow, "has_key": hasKey}, nil
}

func (s *Service) SaveWorkflow(ctx context.Context, config domain.AIWorkflowConfig) (*domain.AIWorkflowConfig, error) {
	existing, err := s.repo.GetAIWorkflow(ctx)
	if err != nil {
		return nil, err
	}
	config.ID = existing.ID
	config.Base = existing.Base
	if config.DefaultHandleMinutes < 1 {
		config.DefaultHandleMinutes = 10
	}
	if config.MaxFailures < 1 {
		config.MaxFailures = 3
	}
	if config.TransferAfterFailures < 1 {
		config.TransferAfterFailures = 2
	}
	if config.TransferAfterDownvotes < 1 {
		config.TransferAfterDownvotes = 2
	}
	if config.HighAmountThreshold <= 0 {
		config.HighAmountThreshold = 500
	}
	if config.EstimateReplyMinutes <= 0 {
		config.EstimateReplyMinutes = 30
	}
	if err := s.repo.SaveAIWorkflow(ctx, &config); err != nil {
		return nil, err
	}
	return s.repo.GetAIWorkflow(ctx)
}

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

// CreateTicket 建单后默认交给 AI 接待：写入首条消息、建 AI 会话、把状态
// 推进到「AI 处理中」，并把处理过程写进消息流。
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
	workflow, _ := s.Workflow(ctx)
	due := s.now().UTC()
	estimate := s.SystemConfigInt(ctx, "ticket", "human_timeout_hours", 0)
	if estimate > 0 {
		due = due.Add(time.Duration(estimate) * time.Hour)
	} else if workflow != nil && workflow.EstimateReplyMinutes > 0 {
		due = due.Add(time.Duration(workflow.EstimateReplyMinutes) * time.Minute)
	}
	ticket := &domain.Ticket{
		TicketNo: no, UserID: input.UserID, Type: ticketType, Subject: subject,
		Content: input.Content, Status: models.TicketStatusAIProcessing, Handler: models.TicketHandlerAI,
		Priority: priority, AIEnabled: true, Source: input.Source, DueAt: &due,
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
	session := &domain.TicketAISession{TicketID: ticket.ID, Status: "active", TurnCount: 1}
	now := s.now().UTC()
	session.LastActiveAt = &now
	if err := s.repo.AppendTicketLog(ctx, &domain.TicketLog{
		TicketID: ticket.ID, ActorType: "system", Action: "ticket.create",
		After: jsonString(map[string]any{"status": ticket.Status, "type": ticket.Type}, 2000), Result: "ok",
	}); err != nil {
		logf("ticket %s: initial log failed: %v", ticket.TicketNo, err)
	}
	// AI 默认接待：这里先让 AI 回一次，用户打开工单就能看到处理结果。
	// AnswerTicket 会读取工单消息并走受控工具循环，能查订单、退款状态等真实数据。
	if s.aiReady(ctx) {
		reply, toolCalls, err := s.AnswerTicket(ctx, ticket, session)
		if err != nil {
			logf("ticket %s: ai first answer failed: %v", ticket.TicketNo, err)
		} else if reply != "" {
			_ = s.repo.CreateMessage(ctx, &domain.TicketMessage{
				TicketID: ticket.ID, SenderType: domain.SenderAI, Content: reply, ContentType: "markdown",
			})
			ticket.Status = models.TicketStatusWaitingUser
			ticket.Handler = models.TicketHandlerAI
			ticket.AIEnabled = true
			if err := s.repo.UpdateTicket(ctx, ticket); err != nil {
				logf("ticket %s: update status failed: %v", ticket.TicketNo, err)
			}
			_ = s.repo.AppendTicketLog(ctx, &domain.TicketLog{
				TicketID: ticket.ID, ActorType: "ai", Action: "ticket.ai_reply",
				Detail: truncate(reply, 200), Result: "ok",
				After: jsonString(map[string]any{"tools": toolCallKeys(toolCalls)}, 2000),
			})
		}
	}
	s.notify(ctx, "ticket.created", map[string]string{
		"ticket_no": ticket.TicketNo, "subject": ticket.Subject, "user_name": fmt.Sprintf("用户 #%d", input.UserID),
	}, Notification{
		UserID: input.UserID, TicketID: ticket.ID,
		Title:   "工单 " + ticket.TicketNo + " 已创建",
		Content: "智能客服已经开始处理你的问题，你也可以随时点「转人工客服」。",
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

// aiReady 判断是否真的配置好了 AI：开关打开且有模型客户端。
func (s *Service) aiReady(ctx context.Context) bool {
	config, err := s.Config(ctx)
	if err != nil || config == nil || !config.IsEnabled {
		return false
	}
	return s.model != nil
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
	// 只取这张工单的调用记录：不带 ticket_id 会把别人的调用一并读出来。
	if calls, _, err := s.repo.ListToolCalls(ctx, contract.ToolCallFilter{TicketID: id, Limit: 200}); err == nil {
		detail.ToolCalls = calls
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
		Content: "用户申请转人工客服，AI 对话上下文已保留。原因：" + reason, ContentType: "text",
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
	workflow, _ := s.Workflow(ctx)
	result := &domain.TransferResult{
		TicketNo: ticket.TicketNo, Summary: summary, SuggestedPlan: plan,
		EstimateMinutes: 30, WorkingHours: "每天 09:00 - 21:00",
	}
	if workflow != nil {
		result.EstimateMinutes = workflow.EstimateReplyMinutes
		if workflow.WorkingHours != "" {
			result.WorkingHours = workflow.WorkingHours
		}
	}
	s.notify(ctx, "ticket.transferred", map[string]string{
		"ticket_no": ticket.TicketNo, "agent_name": "人工客服",
	}, Notification{
		UserID: input.UserID, TicketID: ticket.ID,
		Title:   "工单 " + ticket.TicketNo + " 已转人工",
		Content: "客服会尽快跟进，你的 AI 对话记录已经一并转过去了。",
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

// toolCallKeys 只取工具标识，写进工单日志时不必记录完整结果。
func toolCallKeys(calls []domain.ToolCallResult) []string {
	keys := make([]string, 0, len(calls))
	for _, call := range calls {
		keys = append(keys, call.ToolKey)
	}
	return keys
}
