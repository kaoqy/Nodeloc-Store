package application

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/domain"
)

// ── 工具同步 ─────────────────────────────────────────────────────────

// SyncToolDefinitions 把内置工具注册表同步到数据库：新版本新增的工具自动
// 出现，已存在的保留管理员改过的开关与限频。
func (s *Service) SyncToolDefinitions(ctx context.Context) error {
	s.registerBuiltins()
	existing, err := s.repo.ListTools(ctx, false)
	if err != nil {
		return err
	}
	byKey := map[string]domain.AIToolDefinition{}
	for _, tool := range existing {
		byKey[tool.Key] = tool
	}
	for i, spec := range s.tools.All() {
		record, found := byKey[spec.Key]
		if !found {
			record = domain.AIToolDefinition{
				Key: spec.Key, Name: spec.Name, Description: spec.Description,
				Category: spec.Category, Method: "INTERNAL", Kind: "internal",
				RequireLogin: spec.RequireLogin, OwnDataOnly: spec.OwnDataOnly,
				RequireConfirm: spec.RequireConfirm, AllowAuto: spec.AllowAuto,
				IsEnabled: true, RateLimit: spec.RateLimit,
				TimeoutMS: timeoutOrDefault(spec.TimeoutMS), FailureMode: "reply",
				RiskLevel: riskOrDefault(spec.RiskLevel), Builtin: true, SortOrder: i,
			}
			// 高风险工具默认关闭，必须管理员显式放开。
			if record.RiskLevel == domain.RiskHigh || record.RiskLevel == domain.RiskCritical {
				record.IsEnabled = false
			}
			if err := s.repo.SaveTool(ctx, &record); err != nil {
				return err
			}
			continue
		}
		// 只同步展示字段，不动管理员配置的开关与限频。
		changed := false
		if record.Name != spec.Name {
			record.Name, changed = spec.Name, true
		}
		if record.Description != spec.Description {
			record.Description, changed = spec.Description, true
		}
		if record.Category != spec.Category {
			record.Category, changed = spec.Category, true
		}
		if record.Builtin != true {
			record.Builtin, changed = true, true
		}
		if changed {
			if err := s.repo.SaveTool(ctx, &record); err != nil {
				return err
			}
		}
	}
	return nil
}

func timeoutOrDefault(value int) int {
	if value <= 0 {
		return 8000
	}
	return value
}

func riskOrDefault(value string) string {
	if value == "" {
		return domain.RiskLow
	}
	return value
}

// ToolList 是 AI 工具管理页的读接口。
func (s *Service) ToolList(ctx context.Context) ([]map[string]any, error) {
	s.registerBuiltins()
	if err := s.SyncToolDefinitions(ctx); err != nil {
		return nil, err
	}
	tools, err := s.repo.ListTools(ctx, false)
	if err != nil {
		return nil, err
	}
	permissions, err := s.repo.ListToolPermissions(ctx)
	if err != nil {
		return nil, err
	}
	byTool := map[uint][]domain.AIToolPermission{}
	for _, permission := range permissions {
		byTool[permission.ToolID] = append(byTool[permission.ToolID], permission)
	}
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		out = append(out, map[string]any{"tool": tool, "permissions": byTool[tool.ID]})
	}
	return out, nil
}

// SaveTool 更新一个工具的配置。内置工具的 key 不允许改。
func (s *Service) SaveTool(ctx context.Context, tool domain.AIToolDefinition, actorID uint) (*domain.AIToolDefinition, error) {
	if tool.ID == 0 {
		return nil, fmt.Errorf("%w: 缺少工具编号。", domain.ErrInvalidInput)
	}
	existing, err := s.repo.GetTool(ctx, tool.Key)
	if err != nil {
		return nil, err
	}
	if existing.ID != tool.ID {
		return nil, domain.ErrToolNotFound
	}
	if tool.RiskLevel != "" && !validRisk(tool.RiskLevel) {
		return nil, fmt.Errorf("%w: 风险等级无效。", domain.ErrInvalidInput)
	}
	// 内置工具的元数据以代码为准，只允许改开关、限频与确认策略。
	existing.IsEnabled = tool.IsEnabled
	existing.AllowAuto = tool.AllowAuto
	existing.RequireConfirm = tool.RequireConfirm
	existing.RequireApproval = tool.RequireApproval
	existing.OwnDataOnly = tool.OwnDataOnly
	existing.RateLimit = tool.RateLimit
	if existing.RateLimit < 0 {
		existing.RateLimit = 0
	}
	existing.TimeoutMS = tool.TimeoutMS
	if m := strings.TrimSpace(tool.FailureMode); m != "" {
		existing.FailureMode = m
	}
	if r := strings.TrimSpace(tool.RiskLevel); r != "" {
		existing.RiskLevel = r
	}
	if desc := strings.TrimSpace(tool.Description); desc != "" && !existing.Builtin {
		existing.Description = desc
	}
	if err := s.repo.SaveTool(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

func validRisk(value string) bool {
	for _, candidate := range domain.RiskLevels {
		if candidate == value {
			return true
		}
	}
	return false
}

// SetToolPermission 更新某个角色对某个工具的使用权。
func (s *Service) SetToolPermission(ctx context.Context, toolID uint, role string, allowed bool, actorID uint) error {
	role = strings.TrimSpace(role)
	if role == "" {
		return fmt.Errorf("%w: 请选择角色。", domain.ErrInvalidInput)
	}
	permission := &domain.AIToolPermission{ToolID: toolID, Role: role, Allowed: allowed}
	if actorID > 0 {
		permission.UpdatedBy = &actorID
	}
	return s.repo.SetToolPermission(ctx, permission)
}

// ToolCalls 是工具调用日志。
func (s *Service) ToolCalls(ctx context.Context, filter ToolCallFilterInput) ([]domain.AIToolCall, int64, error) {
	return s.repo.ListToolCalls(ctx, filter.toContract())
}

// ToolCallFilterInput 是 HTTP 层的筛选参数。
type ToolCallFilterInput struct {
	ToolKey string
	Status  string
	UserID  uint
	Limit   int
	Offset  int
}

// ── 知识库 ───────────────────────────────────────────────────────────

func (s *Service) KnowledgeList(ctx context.Context, filter domain.KnowledgeFilter) ([]domain.AIKnowledge, int64, error) {
	return s.repo.ListKnowledge(ctx, filter)
}

func (s *Service) KnowledgeGet(ctx context.Context, id uint) (*domain.AIKnowledge, error) {
	return s.repo.GetKnowledge(ctx, id)
}

// SaveKnowledge 创建或更新一篇知识库文章，slug 唯一且必填。
func (s *Service) SaveKnowledge(ctx context.Context, article domain.AIKnowledge, actorID uint) (*domain.AIKnowledge, error) {
	article.Title = strings.TrimSpace(article.Title)
	article.Content = strings.TrimSpace(article.Content)
	article.Slug = strings.TrimSpace(article.Slug)
	if article.Title == "" || article.Content == "" {
		return nil, fmt.Errorf("%w: 文章标题和正文都要填。", domain.ErrInvalidInput)
	}
	if len([]rune(article.Title)) > 120 {
		return nil, fmt.Errorf("%w: 文章标题最多 120 个字。", domain.ErrInvalidInput)
	}
	if article.Slug == "" {
		article.Slug = slugify(article.Title)
	}
	if article.Status == "" {
		article.Status = "draft"
	}
	switch article.Status {
	case "draft", "published", "disabled":
	default:
		return nil, fmt.Errorf("%w: 文章状态无效。", domain.ErrInvalidInput)
	}
	if article.Priority < 0 {
		article.Priority = 0
	}
	if article.ID == 0 {
		article.Version = 1
		if actorID > 0 {
			article.AuthorID = &actorID
		}
		if err := s.repo.CreateKnowledge(ctx, &article); err != nil {
			return nil, err
		}
		return &article, nil
	}
	existing, err := s.repo.GetKnowledge(ctx, article.ID)
	if err != nil {
		return nil, err
	}
	article.Base = existing.Base
	article.Version = existing.Version + 1
	if article.AuthorID == nil {
		article.AuthorID = existing.AuthorID
	}
	if err := s.repo.UpdateKnowledge(ctx, &article); err != nil {
		return nil, err
	}
	return &article, nil
}

func (s *Service) DeleteKnowledge(ctx context.Context, id uint) error {
	article, err := s.repo.GetKnowledge(ctx, id)
	if err != nil {
		return err
	}
	if article.Builtin {
		return fmt.Errorf("%w: 内置说明文章不能删除，可以改为停用。", domain.ErrInvalidInput)
	}
	return s.repo.DeleteKnowledge(ctx, id)
}

// TestKnowledge 让管理员验证 AI 是否能正确引用某篇文章。
func (s *Service) TestKnowledge(ctx context.Context, id uint, question string) (map[string]any, error) {
	article, err := s.repo.GetKnowledge(ctx, id)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(question) == "" {
		question = article.Title
	}
	hits, err := s.repo.SearchKnowledge(ctx, question, 5)
	if err != nil {
		return nil, err
	}
	found := false
	for _, hit := range hits {
		if hit.ID == article.ID {
			found = true
			break
		}
	}
	return map[string]any{
		"question": question, "hits": hits, "matched": found,
		"message": func() string {
			if found {
				return "AI 检索能命中这篇文章。"
			}
			return "这次检索没有命中，可以给文章补上关键词，或提高优先级。"
		}(),
	}, nil
}

// ImportKnowledge 批量导入，每行支持「标题|正文」或仅标题。
func (s *Service) ImportKnowledge(ctx context.Context, categoryID uint, text string, actorID uint) (int, error) {
	lines := strings.Split(text, "\n")
	imported := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		title := strings.TrimSpace(parts[0])
		if title == "" {
			continue
		}
		content := title
		if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
			content = strings.TrimSpace(parts[1])
		}
		article := domain.AIKnowledge{
			CategoryID: categoryID, Title: title, Slug: slugify(title) + "-" + timestampSuffix(),
			Content: content, Status: "draft", Source: "import",
		}
		if _, err := s.SaveKnowledge(ctx, article, actorID); err != nil {
			continue
		}
		imported++
	}
	return imported, nil
}

// ExportKnowledge 导出全部文章为可再导入的文本格式。
func (s *Service) ExportKnowledge(ctx context.Context) (string, error) {
	articles, _, err := s.repo.ListKnowledge(ctx, domain.KnowledgeFilter{Limit: 200})
	if err != nil {
		return "", err
	}
	var builder strings.Builder
	for _, article := range articles {
		builder.WriteString(article.Title)
		builder.WriteString("|")
		builder.WriteString(strings.ReplaceAll(article.Content, "\n", " "))
		builder.WriteString("\n")
	}
	return builder.String(), nil
}

// ── 知识库分类 ───────────────────────────────────────────────────────

func (s *Service) KnowledgeCategories(ctx context.Context) ([]domain.AIKnowledgeCategory, error) {
	return s.repo.ListKnowledgeCategories(ctx)
}

func (s *Service) SaveKnowledgeCategory(ctx context.Context, category domain.AIKnowledgeCategory) (*domain.AIKnowledgeCategory, error) {
	category.Name = strings.TrimSpace(category.Name)
	if category.Name == "" {
		return nil, fmt.Errorf("%w: 分类名称要填。", domain.ErrInvalidInput)
	}
	if category.Slug == "" {
		category.Slug = slugify(category.Name)
	}
	if category.ID == 0 {
		if err := s.repo.CreateKnowledgeCategory(ctx, &category); err != nil {
			return nil, err
		}
		return &category, nil
	}
	existing, err := s.repo.ListKnowledgeCategories(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range existing {
		if item.ID == category.ID {
			category.Base = item.Base
			break
		}
	}
	if err := s.repo.UpdateKnowledgeCategory(ctx, &category); err != nil {
		return nil, err
	}
	return &category, nil
}

func (s *Service) DeleteKnowledgeCategory(ctx context.Context, id uint) error {
	return s.repo.DeleteKnowledgeCategory(ctx, id)
}

// ── 快捷问题 ─────────────────────────────────────────────────────────

func (s *Service) QuickQuestions(ctx context.Context, enabledOnly bool, position string) ([]domain.AIQuickQuestion, error) {
	return s.repo.ListQuickQuestions(ctx, enabledOnly, position)
}

func (s *Service) SaveQuickQuestion(ctx context.Context, question domain.AIQuickQuestion) (*domain.AIQuickQuestion, error) {
	question.Title = strings.TrimSpace(question.Title)
	if question.Title == "" {
		return nil, fmt.Errorf("%w: 快捷问题标题要填。", domain.ErrInvalidInput)
	}
	if question.Position == "" {
		question.Position = "widget"
	}
	if err := s.repo.SaveQuickQuestion(ctx, &question); err != nil {
		return nil, err
	}
	return &question, nil
}

func (s *Service) DeleteQuickQuestion(ctx context.Context, id uint) error {
	return s.repo.DeleteQuickQuestion(ctx, id)
}

// ── 客服人员 ─────────────────────────────────────────────────────────

func (s *Service) Agents(ctx context.Context) ([]map[string]any, error) {
	agents, err := s.repo.ListAgents(ctx, false)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(agents))
	for _, agent := range agents {
		load, _ := s.repo.AgentLoad(ctx, agent.ID)
		out = append(out, map[string]any{"agent": agent, "current_load": load})
	}
	return out, nil
}

func (s *Service) SaveAgent(ctx context.Context, agent domain.CustomerServiceAgent) (*domain.CustomerServiceAgent, error) {
	if agent.UserID == 0 {
		return nil, fmt.Errorf("%w: 请选择要设为客服的账号。", domain.ErrInvalidInput)
	}
	if agent.MaxConcurrent <= 0 {
		agent.MaxConcurrent = 5
	}
	switch agent.Status {
	case "", "online", "busy", "offline":
		if agent.Status == "" {
			agent.Status = "offline"
		}
	default:
		return nil, fmt.Errorf("%w: 在线状态取值无效。", domain.ErrInvalidInput)
	}
	if agent.AssignStrategy == "" {
		agent.AssignStrategy = "balanced"
	}
	if err := s.repo.SaveAgent(ctx, &agent); err != nil {
		return nil, err
	}
	return &agent, nil
}

func (s *Service) DeleteAgent(ctx context.Context, id uint) error {
	return s.repo.DeleteAgent(ctx, id)
}

// ── 快捷回复 ─────────────────────────────────────────────────────────

func (s *Service) QuickReplies(ctx context.Context, enabledOnly bool, ticketType, role string) ([]domain.QuickReply, error) {
	return s.repo.ListQuickReplies(ctx, enabledOnly, ticketType, role)
}

func (s *Service) SaveQuickReply(ctx context.Context, reply domain.QuickReply) (*domain.QuickReply, error) {
	reply.Title = strings.TrimSpace(reply.Title)
	reply.Content = strings.TrimSpace(reply.Content)
	if reply.Title == "" || reply.Content == "" {
		return nil, fmt.Errorf("%w: 快捷回复标题和内容都要填。", domain.ErrInvalidInput)
	}
	if err := s.repo.SaveQuickReply(ctx, &reply); err != nil {
		return nil, err
	}
	return &reply, nil
}

func (s *Service) DeleteQuickReply(ctx context.Context, id uint) error {
	return s.repo.DeleteQuickReply(ctx, id)
}

// RenderQuickReply 用变量渲染一条快捷回复，变量缺失时保留空串而不是报错。
func (s *Service) RenderQuickReply(ctx context.Context, id uint, variables map[string]string) (string, error) {
	replies, err := s.repo.ListQuickReplies(ctx, false, "", "")
	if err != nil {
		return "", err
	}
	for _, reply := range replies {
		if reply.ID != id {
			continue
		}
		content := reply.Content
		for key, value := range variables {
			content = strings.ReplaceAll(content, "{"+key+"}", value)
		}
		_ = s.repo.IncrementQuickReplyUse(ctx, id)
		return content, nil
	}
	return "", fmt.Errorf("%w: 没有找到这条快捷回复。", domain.ErrInvalidInput)
}

// ── 买家与后台共用的补充查询 ────────────────────────────────────────

// AgentByUser 返回某个后台账号对应的客服坐席，没配置时返回 nil。
func (s *Service) AgentByUser(ctx context.Context, userID uint) (*domain.CustomerServiceAgent, error) {
	if userID == 0 {
		return nil, nil
	}
	return s.repo.GetAgentByUser(ctx, userID)
}

// Conversations 是后台的 AI 会话列表。
func (s *Service) Conversations(ctx context.Context, userID uint, limit, offset int) ([]domain.AIConversation, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	return s.repo.ListConversations(ctx, userID, limit, offset)
}

// FeedbackList 是后台的评价列表。
func (s *Service) FeedbackList(ctx context.Context, limit int) ([]domain.AIFeedback, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	return s.repo.ListFeedback(ctx, limit)
}

// RateTicket 记录买家对工单的满意度评分。
func (s *Service) RateTicket(ctx context.Context, ticketID uint, satisfaction int, comment string) error {
	if satisfaction < 1 || satisfaction > 5 {
		return fmt.Errorf("%w: 满意度请在 1 到 5 分之间。", domain.ErrInvalidInput)
	}
	ticket, err := s.repo.GetTicket(ctx, ticketID)
	if err != nil {
		return err
	}
	ticket.Satisfaction = satisfaction
	ticket.SatisfactionNote = strings.TrimSpace(comment)
	if err := s.repo.UpdateTicket(ctx, ticket); err != nil {
		return err
	}
	return s.repo.AppendTicketLog(ctx, &domain.TicketLog{
		TicketID: ticketID, ActorType: "user", Action: "ticket.rate",
		Detail: fmt.Sprintf("满意度 %d 分", satisfaction), Result: "ok",
	})
}

// SystemConfigValue 读取配置中心的一项设置；没有配置时返回兜底值。
// 配置中心写的是同一张 system_configs 表，所以后台改完立刻生效，不用重启。
func (s *Service) SystemConfigValue(ctx context.Context, group, key, fallback string) string {
	configs, err := s.SystemConfigs(ctx, group)
	if err != nil {
		return fallback
	}
	for _, config := range configs {
		if config.Key == key {
			if strings.TrimSpace(config.Value) == "" {
				return fallback
			}
			return config.Value
		}
	}
	return fallback
}

// SystemConfigInt 读取一个整数配置，解析失败时退回兜底值。
func (s *Service) SystemConfigInt(ctx context.Context, group, key string, fallback int) int {
	value := s.SystemConfigValue(ctx, group, key, "")
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

// SystemConfigBool 读取一个开关配置。
func (s *Service) SystemConfigBool(ctx context.Context, group, key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(s.SystemConfigValue(ctx, group, key, "")))
	switch value {
	case "":
		return fallback
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

// ── 统一配置中心 ─────────────────────────────────────────────────────

// NotificationTemplates 返回通知模板；为空时写入内置模板，保证后台有可编辑项。
func (s *Service) NotificationTemplates(ctx context.Context, category string) ([]domain.NotificationTemplate, error) {
	if err := s.seedNotificationTemplates(ctx); err != nil {
		logf("seed notification templates: %v", err)
	}
	return s.repo.ListNotificationTemplates(ctx, category)
}

func (s *Service) SaveNotificationTemplate(ctx context.Context, template domain.NotificationTemplate) (*domain.NotificationTemplate, error) {
	template.Key = strings.TrimSpace(template.Key)
	template.Name = strings.TrimSpace(template.Name)
	template.Event = strings.TrimSpace(template.Event)
	if template.Key == "" || template.Name == "" {
		return nil, fmt.Errorf("%w: 模板标识和名称都要填。", domain.ErrInvalidInput)
	}
	if template.TitleTemplate == "" && template.ContentTemplate == "" {
		return nil, fmt.Errorf("%w: 标题模板和内容模板至少要填一个。", domain.ErrInvalidInput)
	}
	if template.Category == "" {
		template.Category = "ticket"
	}
	if template.RetryLimit < 0 {
		template.RetryLimit = 3
	}
	if err := s.repo.SaveNotificationTemplate(ctx, &template); err != nil {
		return nil, err
	}
	return &template, nil
}

func (s *Service) DeleteNotificationTemplate(ctx context.Context, id uint) error {
	return s.repo.DeleteNotificationTemplate(ctx, id)
}

// seedNotificationTemplates 补齐缺失的默认模板，但不覆盖店家改过的模板。
func (s *Service) seedNotificationTemplates(ctx context.Context) error {
	existing, err := s.repo.ListNotificationTemplates(ctx, "")
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(existing))
	for _, template := range existing {
		known[template.Key] = true
	}
	for i, template := range defaultNotificationTemplates() {
		if known[template.Key] {
			continue
		}
		template.SortOrder = i
		if err := s.repo.SaveNotificationTemplate(ctx, &template); err != nil {
			return err
		}
	}
	return nil
}

func defaultNotificationTemplates() []domain.NotificationTemplate {
	return []domain.NotificationTemplate{
		{Key: "ticket.created", Name: "工单创建通知", Event: "ticket_created", Category: "ticket", InApp: true, TitleTemplate: "工单 {{ticket_no}} 已创建", ContentTemplate: "你的问题「{{subject}}」已提交，智能客服会先接待。", Variables: "ticket_no,subject,user_name"},
		{Key: "ticket.ai_started", Name: "AI 开始处理通知", Event: "ai_started", Category: "ticket", InApp: true, TitleTemplate: "智能客服已开始处理 {{ticket_no}}", ContentTemplate: "AI 正在核对你的问题，需要人工时会自动转接。", Variables: "ticket_no"},
		{Key: "ticket.ai_done", Name: "AI 处理完成通知", Event: "ai_done", Category: "ticket", InApp: true, TitleTemplate: "{{ticket_no}} 有新回复", ContentTemplate: "智能客服已给出处理建议，请查看并确认是否解决。", Variables: "ticket_no"},
		{Key: "ticket.transferred", Name: "转人工成功通知", Event: "ticket_transfer", Category: "ticket", InApp: true, Mail: true, TitleTemplate: "工单 {{ticket_no}} 已转人工", ContentTemplate: "人工客服会尽快跟进，你的对话记录已一并转交。", Variables: "ticket_no,agent_name"},
		{Key: "ticket.agent_reply", Name: "人工客服回复通知", Event: "agent_reply", Category: "ticket", InApp: true, Mail: true, TitleTemplate: "客服回复了 {{ticket_no}}", ContentTemplate: "{{agent_name}} 回复了你的工单，请查看。", Variables: "ticket_no,agent_name"},
		{Key: "ticket.status_changed", Name: "工单状态变化通知", Event: "ticket_status", Category: "ticket", InApp: true, TitleTemplate: "{{ticket_no}} 状态更新为 {{status}}", ContentTemplate: "工单状态已更新，点击查看详情。", Variables: "ticket_no,status"},
		{Key: "ticket.timeout_warning", Name: "工单即将超时通知", Event: "ticket_timeout", Category: "ticket", InApp: true, TitleTemplate: "工单 {{ticket_no}} 即将超时", ContentTemplate: "请尽快处理这张工单，当前优先级 {{priority}}。", Variables: "ticket_no,priority", Recipients: "staff"},
		{Key: "activity.started", Name: "活动开始通知", Event: "activity_started", Category: "activity", InApp: true, TitleTemplate: "活动「{{activity_name}}」开始了", ContentTemplate: "{{activity_name}}：{{subtitle}}", Variables: "activity_name,subtitle"},
		{Key: "coupon.claimed", Name: "优惠券领取通知", Event: "coupon_claimed", Category: "coupon", InApp: true, TitleTemplate: "优惠券已领取", ContentTemplate: "优惠券已放入你的账户，下单时填写对应优惠码即可抵扣。", Variables: "coupon_name"},
		{Key: "coupon.expiring", Name: "优惠券过期通知", Event: "coupon_expiring", Category: "coupon", InApp: true, Mail: true, TitleTemplate: "优惠券即将过期", ContentTemplate: "你的优惠券将在 {{expire_at}} 到期，请尽快使用。", Variables: "expire_at"},
		{Key: "order.exception", Name: "订单异常通知", Event: "order_exception", Category: "order", InApp: true, Mail: true, TitleTemplate: "订单 {{order_no}} 需要关注", ContentTemplate: "{{detail}}", Variables: "order_no,detail", Recipients: "staff"},
	}
}

// SystemConfigs 返回配置中心的分组配置；为空时写入内置默认值。
func (s *Service) SystemConfigs(ctx context.Context, group string) ([]domain.SystemConfig, error) {
	if err := s.seedSystemConfigs(ctx); err != nil {
		logf("seed system configs: %v", err)
	}
	return s.repo.ListSystemConfigs(ctx, group)
}

func (s *Service) SaveSystemConfig(ctx context.Context, config domain.SystemConfig, actorID uint) (*domain.SystemConfig, error) {
	config.Group = strings.TrimSpace(config.Group)
	config.Key = strings.TrimSpace(config.Key)
	if config.Group == "" || config.Key == "" {
		return nil, fmt.Errorf("%w: 配置分组和键都要填。", domain.ErrInvalidInput)
	}
	switch config.ValueType {
	case "", "string", "int", "bool", "json", "text":
	default:
		return nil, fmt.Errorf("%w: 配置值类型无效。", domain.ErrInvalidInput)
	}
	if config.ValueType == "" {
		config.ValueType = "string"
	}
	if config.ValueType == "int" {
		if _, err := strconv.Atoi(strings.TrimSpace(config.Value)); err != nil {
			return nil, fmt.Errorf("%w: 这一项需要填写整数。", domain.ErrInvalidInput)
		}
	}
	if config.ValueType == "bool" {
		switch strings.ToLower(strings.TrimSpace(config.Value)) {
		case "1", "0", "true", "false", "on", "off", "yes", "no":
		default:
			return nil, fmt.Errorf("%w: 这一项需要填写是/否。", domain.ErrInvalidInput)
		}
	}
	if config.ValueType == "json" && strings.TrimSpace(config.Value) != "" {
		var probe any
		if err := json.Unmarshal([]byte(config.Value), &probe); err != nil {
			return nil, fmt.Errorf("%w: 这一项需要填写合法 JSON。", domain.ErrInvalidInput)
		}
	}
	if actorID > 0 {
		config.UpdatedBy = &actorID
	}
	if err := s.repo.SaveSystemConfig(ctx, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// seedSystemConfigs 保证默认配置项都存在：升级后新增的键会被补上，
// 店家已经改过的值不会被覆盖。所以这里不能只在表为空时执行一次。
func (s *Service) seedSystemConfigs(ctx context.Context) error {
	existing, err := s.repo.ListSystemConfigs(ctx, "")
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(existing))
	for _, config := range existing {
		known[config.Group+"/"+config.Key] = true
	}
	for _, config := range defaultSystemConfigs() {
		if known[config.Group+"/"+config.Key] {
			continue
		}
		if err := s.repo.SaveSystemConfig(ctx, &config); err != nil {
			return err
		}
	}
	return nil
}

// defaultSystemConfigs 是配置中心的首批可编辑项，覆盖工单、风控与数据保留。
func defaultSystemConfigs() []domain.SystemConfig {
	return []domain.SystemConfig{
		{Group: "ticket", Key: "ticket_no_prefix", Value: "TK", ValueType: "string", Label: "工单编号前缀", SortOrder: 0},
		{Group: "ticket", Key: "auto_close_days", Value: "7", ValueType: "int", Label: "自动关闭时间（天）", SortOrder: 1},
		{Group: "ticket", Key: "human_timeout_hours", Value: "24", ValueType: "int", Label: "人工响应超时（小时）", SortOrder: 2},
		{Group: "ticket", Key: "allow_reopen", Value: "true", ValueType: "bool", Label: "允许用户重新打开", SortOrder: 3},
		{Group: "ticket", Key: "allow_cancel", Value: "true", ValueType: "bool", Label: "允许用户撤销工单", SortOrder: 4},
		{Group: "ticket", Key: "enable_rating", Value: "true", ValueType: "bool", Label: "启用工单评价", SortOrder: 5},
		{Group: "ticket", Key: "attachment_types", Value: "jpg,png,webp,pdf", ValueType: "string", Label: "允许的附件类型", SortOrder: 6},
		{Group: "ticket", Key: "attachment_max_mb", Value: "8", ValueType: "int", Label: "附件大小上限（MB）", SortOrder: 7},
		{Group: "risk", Key: "coupon_max_attempts", Value: "10", ValueType: "int", Label: "优惠码每分钟尝试上限", SortOrder: 0},
		{Group: "risk", Key: "ai_message_rate", Value: "30", ValueType: "int", Label: "AI 消息每分钟上限", SortOrder: 1},
		{Group: "risk", Key: "require_second_confirm", Value: "true", ValueType: "bool", Label: "高风险操作二次确认", SortOrder: 2},
		{Group: "retention", Key: "audit_log_days", Value: "180", ValueType: "int", Label: "操作日志保留天数", SortOrder: 0},
		{Group: "retention", Key: "ai_conversation_days", Value: "90", ValueType: "int", Label: "AI 对话保留天数", SortOrder: 1},
		{Group: "retention", Key: "ticket_days", Value: "365", ValueType: "int", Label: "工单保留天数", SortOrder: 2},
		{Group: "upload", Key: "max_image_mb", Value: "8", ValueType: "int", Label: "图片上传上限（MB）", SortOrder: 0},
		// 订单与商品：这些开关影响买家下单与库存提醒，不再只存在于代码里。
		{Group: "order", Key: "unpaid_cancel_hours", Value: "2", ValueType: "int", Label: "待支付订单保留小时数", SortOrder: 0},
		{Group: "order", Key: "allow_guest_checkout", Value: "false", ValueType: "bool", Label: "允许未登录下单", SortOrder: 1},
		{Group: "order", Key: "auto_deliver_retry", Value: "true", ValueType: "bool", Label: "交付失败自动重试", SortOrder: 2},
		{Group: "product", Key: "default_stock_alert", Value: "5", ValueType: "int", Label: "默认库存预警阈值", SortOrder: 0},
		{Group: "product", Key: "show_sold_count", Value: "true", ValueType: "bool", Label: "前台显示销量", SortOrder: 1},
		{Group: "product", Key: "allow_restock_notify", Value: "true", ValueType: "bool", Label: "缺货时通知补货人", SortOrder: 2},
		{Group: "activity", Key: "default_per_user_limit", Value: "1", ValueType: "int", Label: "活动默认每人限次", SortOrder: 0},
		{Group: "activity", Key: "block_activity_stacking", Value: "true", ValueType: "bool", Label: "默认禁止活动叠加", SortOrder: 1},
		{Group: "site", Key: "announcement_position", Value: "home", ValueType: "string", Label: "公告展示位置", SortOrder: 0},
		{Group: "site", Key: "footer_show_version", Value: "true", ValueType: "bool", Label: "页脚显示版本号", SortOrder: 1},
	}
}

// ── 帮助函数 ─────────────────────────────────────────────────────────

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastDash = false
		case r > 127:
			builder.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && builder.Len() > 0 {
				builder.WriteString("-")
				lastDash = true
			}
		}
	}
	out := strings.Trim(builder.String(), "-")
	if out == "" {
		out = fmt.Sprintf("article-%d", time.Now().Unix())
	}
	return out
}

func timestampSuffix() string {
	return time.Now().UTC().Format("150405")
}

// SeedKnowledge 在知识库为空时写入官方说明文章，保证 AI 第一次就有依据。
func (s *Service) SeedKnowledge(ctx context.Context, actorID uint) error {
	existing, total, err := s.repo.ListKnowledge(ctx, domain.KnowledgeFilter{Limit: 1})
	if err != nil {
		return err
	}
	if total > 0 || len(existing) > 0 {
		return nil
	}
	articles := defaultKnowledge()
	for i := range articles {
		article := articles[i]
		article.Builtin = true
		article.Status = "published"
		article.Source = "builtin"
		article.Priority = 100 - i
		if actorID > 0 {
			article.AuthorID = &actorID
		}
		if err := s.repo.CreateKnowledge(ctx, &article); err != nil {
			return err
		}
	}
	return nil
}

func defaultKnowledge() []domain.AIKnowledge {
	type seed struct {
		title    string
		keywords string
		content  string
	}
	seeds := []seed{
		{"购买流程", "购买,下单,怎么买,流程", "1. 使用 NodeLoc 账号登录或注册本站账号。\n2. 在首页或分类页选择商品，进入商品详情页。\n3. 填写购买表单需要的资料，确认数量与金额。\n4. 点击「立即购买」创建订单，再点击「去支付」跳转 NodeLoc Payments 完成付款。\n5. 付款完成后回到订单页查看交付结果。"},
		{"支付说明", "支付,付款,NodeLoc Payments,积分", "本站只使用 NodeLoc Payments 收款，不提供其他支付方式。\n付款时订单金额由服务端计算并锁定，优惠码与活动折扣都会在付款前生效。\n如果付款后订单仍显示待支付，请在订单页点击「重新核对支付结果」，系统会向 NodeLoc 查单。"},
		{"自动发货说明", "自动发货,卡密,秒发", "卡密类商品付款成功后由系统自动发放。\n如果库存不足，订单会进入「等待补货」，补货后系统会立刻自动发货并通知你。\n卡密只会出现在你自己的订单详情页，请勿转发或截图给他人。"},
		{"人工发货说明", "人工发货,手动发货,多久发货", "人工交付的商品在付款后进入商家的发货队列。\n商家会按付款顺序处理，通常会在工作时间内完成。\n发货后订单详情页会显示交付内容，并给你发送站内通知。"},
		{"卡密使用教程", "卡密,兑换,使用教程", "1. 打开「我的订单」，找到对应订单。\n2. 在交付内容中复制卡密。\n3. 按商品说明到对应的平台兑换或激活。\n如果提示卡密无效或已被使用，请带着订单号创建工单，客服会核对交付记录。"},
		{"退款规则", "退款,退钱,售后", "数字商品具有一次性交付属性，已交付且可正常使用的卡密原则上不支持退款。\n如果卡密无效、重复或未交付，请在订单页创建工单并选择「退款」，客服核实后按规则处理。\n退款会退回你的 NodeLoc 账户，需要由有权限的管理员在后台执行。"},
		{"售后规则", "售后,处理时效,客服", "提交工单后由智能客服先接待，需要人工时会转给客服同事。\n请提供订单号、商品名与具体现象，便于快速定位。\n工作时间内会尽快响应，复杂问题需要向 NodeLoc 核实，时间可能更长。"},
		{"活动规则", "活动,折扣,满减,优惠码", "活动规则由商家在后台配置，下单金额一律由服务端按活动规则计算。\n同一订单默认只应用优惠力度最大的一条活动，是否可叠加以活动页说明为准。\n优惠码有生效时间、使用范围与每人限用次数，过期或超限会给出具体原因。"},
		{"常见问题", "常见问题,FAQ,登录,绑定", "Q：登录失败怎么办？\nA：确认使用的是 NodeLoc OAuth2 登录，并检查浏览器是否拦截了跳转。\nQ：邮箱不见了？\nA：只有申请到 email scope 后 NodeLoc 才会返回邮箱。\nQ：怎么绑定账号？\nA：在个人中心点击「绑定 NodeLoc」，按提示完成授权。"},
		{"联系客服说明", "联系客服,人工,工作时间", "你可以在任意页面右下角打开智能客服窗口。\n需要人工时，点击「转人工客服」，系统会把完整对话、订单与问题摘要一起交给客服同事。\n也可以直接在「我的工单」里提交新工单。"},
	}
	out := make([]domain.AIKnowledge, 0, len(seeds))
	for _, item := range seeds {
		out = append(out, domain.AIKnowledge{
			Title: item.title, Slug: slugify(item.title), Content: item.content,
			Keywords: item.keywords, Summary: item.content[:minInt(len(item.content), 120)],
			Tags: item.keywords, Status: "published",
		})
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SortedRoleList 是 AI 权限矩阵里可选的角色。
func SortedRoleList() []string {
	roles := []string{
		"guest", "user", "support", "support_agent", "support_lead",
		"operator", "ops_manager", "product_manager", "order_manager",
		"finance", "ai_admin", "data_viewer", "admin", "super_admin",
	}
	sort.Strings(roles)
	return roles
}
