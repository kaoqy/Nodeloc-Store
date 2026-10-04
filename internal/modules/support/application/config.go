package application

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/domain"
)

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
		{Key: "ticket.created", Name: "工单创建通知", Event: "ticket_created", Category: "ticket", InApp: true, TitleTemplate: "工单 {{ticket_no}} 已创建", ContentTemplate: "你的问题「{{subject}}」已提交，客服会尽快跟进。", Variables: "ticket_no,subject,user_name"},
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
	rows, err := s.repo.ListSystemConfigs(ctx, group)
	if err != nil {
		return nil, err
	}
	// 老库里可能留着已经下线的配置行（例如只存在于代码里、从未生效的开关）。
	// 这里按内置清单过滤，而不是删数据：店主的旧值仍在库里，
	// 万一将来需要恢复某一项，数据还在。
	return filterActiveConfigs(rows), nil
}

// filterActiveConfigs 只保留仍在 defaultSystemConfigs 里的配置项。
func filterActiveConfigs(rows []domain.SystemConfig) []domain.SystemConfig {
	active := make(map[string]bool, len(rows))
	for _, config := range defaultSystemConfigs() {
		active[config.Group+"/"+config.Key] = true
	}
	out := make([]domain.SystemConfig, 0, len(rows))
	for _, config := range rows {
		if !active[config.Group+"/"+config.Key] {
			continue
		}
		// 补充说明文字：老库里没有 Description 列，用内置文案兜住。
		if strings.TrimSpace(config.Description) == "" {
			for _, preset := range defaultSystemConfigs() {
				if preset.Group == config.Group && preset.Key == config.Key {
					config.Description = preset.Description
					break
				}
			}
		}
		out = append(out, config)
	}
	return out
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
//
// 已下线的键（例如旧版 AI 客服的开关）同时从库里清掉：它们不在默认清单里，
// 留着只会让老店主的配置页出现改了也无效的幽灵项。
func (s *Service) seedSystemConfigs(ctx context.Context) error {
	existing, err := s.repo.ListSystemConfigs(ctx, "")
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(existing))
	for _, config := range existing {
		known[config.Group+"/"+config.Key] = true
	}
	active := make(map[string]bool)
	for _, config := range defaultSystemConfigs() {
		key := config.Group + "/" + config.Key
		active[key] = true
		if known[key] {
			continue
		}
		if err := s.repo.SaveSystemConfig(ctx, &config); err != nil {
			return err
		}
	}
	// 清理已经下线的配置行：默认清单是唯一事实来源。
	for _, config := range existing {
		if active[config.Group+"/"+config.Key] {
			continue
		}
		if err := s.repo.DeleteSystemConfig(ctx, config.Group, config.Key); err != nil {
			return err
		}
	}
	return nil
}

// defaultSystemConfigs 是配置中心的可编辑项清单，覆盖工单、订单商品、营销、站点、风控与数据保留。
func defaultSystemConfigs() []domain.SystemConfig {
	return []domain.SystemConfig{
		// ── 工单规则 ──────────────────────────────────────────────
		{Group: "ticket", Key: "ticket_no_prefix", Value: "TK", ValueType: "string",
			Label: "工单编号前缀", Description: "买家看到的工单号前缀，例如 TK 生成 TK0001。", SortOrder: 0},
		{Group: "ticket", Key: "human_timeout_hours", Value: "24", ValueType: "int",
			Label: "人工响应超时（小时）", Description: "转人工后超过这个时间未处理，工单会标记为即将超时。", SortOrder: 1},
		{Group: "ticket", Key: "allow_reopen", Value: "true", ValueType: "bool",
			Label: "允许买家重新打开", Description: "关闭或已解决的工单，买家可以重新打开继续追问。", SortOrder: 2},
		{Group: "ticket", Key: "allow_cancel", Value: "true", ValueType: "bool",
			Label: "允许买家撤销工单", Description: "买家可以自己撤销还没解决的工单。", SortOrder: 3},
		{Group: "ticket", Key: "enable_rating", Value: "true", ValueType: "bool",
			Label: "启用工单评价", Description: "工单结束后邀请买家打分，分数会进满意度统计。", SortOrder: 4},

		// ── 兼容设置（保留已有数据，不在配置页面展示）────────────
		// 这些键在过去版本里可能已经被店家修改过，但当前版本没有对应的
		// 执行路径。保留在 active 清单里可避免 seedSystemConfigs 删除旧值，
		// 前端则不再渲染这些控件，避免出现「改了但不生效」的假配置。
		{Group: "order", Key: "unpaid_cancel_hours", Value: "2", ValueType: "int",
			Label: "待支付订单保留时长（小时）", Description: "历史兼容配置，当前版本未接入自动关单任务。", SortOrder: 0},
		{Group: "order", Key: "auto_deliver_retry", Value: "true", ValueType: "bool",
			Label: "交付失败自动重试", Description: "历史兼容配置，当前版本未接入自动重试任务。", SortOrder: 1},

		// ── 商品规则 ──────────────────────────────────────────────
		{Group: "product", Key: "default_stock_alert", Value: "5", ValueType: "int",
			Label: "库存预警阈值", Description: "历史兼容配置；当前预警阈值由系统设置中的功能开关读取。", SortOrder: 0},
		{Group: "product", Key: "show_sold_count", Value: "true", ValueType: "bool",
			Label: "前台显示销量", Description: "在商品卡片上显示已售数量。", SortOrder: 1},

		// ── 营销规则 ──────────────────────────────────────────────
		{Group: "activity", Key: "default_per_user_limit", Value: "1", ValueType: "int",
			Label: "活动默认每人限次", Description: "新建活动时默认的每人参与次数；0 表示不限次。", SortOrder: 0},
		{Group: "activity", Key: "block_activity_stacking", Value: "true", ValueType: "bool",
			Label: "默认禁止活动叠加", Description: "一条订单只应用优惠最大的一项活动，避免折上折算出异常低价。", SortOrder: 1},

		// ── 站点展示 ──────────────────────────────────────────────
		{Group: "site", Key: "footer_show_version", Value: "true", ValueType: "bool",
			Label: "页脚显示版本号", Description: "在店铺页脚展示当前版本，方便你核对线上是否是最新构建。", SortOrder: 0},

		{Group: "risk", Key: "coupon_max_attempts", Value: "10", ValueType: "int",
			Label: "优惠码每分钟尝试上限", Description: "历史兼容配置，当前版本未接入限流计数器。", SortOrder: 0},
		{Group: "risk", Key: "require_second_confirm", Value: "true", ValueType: "bool",
			Label: "高风险操作二次确认", Description: "历史兼容配置，当前版本未接入统一二次确认中间件。", SortOrder: 1},
		{Group: "retention", Key: "audit_log_days", Value: "180", ValueType: "int",
			Label: "操作日志保留天数", Description: "历史兼容配置，当前版本未接入日志清理任务。", SortOrder: 0},
		{Group: "retention", Key: "ticket_days", Value: "365", ValueType: "int",
			Label: "工单保留天数", Description: "历史兼容配置，当前版本未接入工单清理任务。", SortOrder: 1},
		{Group: "upload", Key: "max_image_mb", Value: "8", ValueType: "int",
			Label: "图片上传上限（MB）", Description: "历史兼容配置；当前上传上限固定为 2 MB。", SortOrder: 0},
	}
}

// SortedRoleList 是客服配置里可选的角色。
func SortedRoleList() []string {
	roles := []string{
		"guest", "user", "support", "support_agent", "support_lead",
		"operator", "ops_manager", "product_manager", "order_manager",
		"finance", "data_viewer", "admin", "super_admin",
	}
	sort.Strings(roles)
	return roles
}
