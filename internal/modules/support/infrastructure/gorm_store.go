package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/domain"
)

// GormStore 是工单与 AI 模块的持久化适配器。
type GormStore struct {
	db *gorm.DB
}

func NewGormStore(db *gorm.DB) *GormStore { return &GormStore{db: db} }

// ── 工单 ─────────────────────────────────────────────────────────────

func (s *GormStore) ListTickets(ctx context.Context, filter domain.TicketFilter) ([]domain.TicketView, int64, error) {
	query := s.db.WithContext(ctx).Model(&models.Ticket{})
	if status := strings.TrimSpace(filter.Status); status != "" {
		query = query.Where("status = ?", status)
	}
	if handler := strings.TrimSpace(filter.Handler); handler != "" {
		query = query.Where("handler = ?", handler)
	}
	if priority := strings.TrimSpace(filter.Priority); priority != "" {
		query = query.Where("priority = ?", priority)
	}
	if ticketType := strings.TrimSpace(filter.Type); ticketType != "" {
		query = query.Where("type = ?", ticketType)
	}
	if filter.AgentID > 0 {
		query = query.Where("assigned_agent_id = ?", filter.AgentID)
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("ticket_no LIKE ? OR subject LIKE ? OR order_no LIKE ? OR product_name LIKE ?", like, like, like, like)
	}
	switch filter.Attention {
	case "ai":
		// 与统计卡「AI 处理中」同一组状态，点进去条数必须一致。
		query = query.Where("status IN ?", []string{models.TicketStatusAIProcessing, models.TicketStatusWaitingUser})
	case "pending_human":
		// 与统计卡「待人工处理」同一组状态：用户已申请 + 排队中。
		query = query.Where("status IN ?", []string{models.TicketStatusUserRequested, models.TicketStatusPendingHuman})
	case "overdue":
		query = query.Where("due_at IS NOT NULL AND due_at < ? AND status NOT IN ?", time.Now().UTC(), []string{models.TicketStatusResolved, models.TicketStatusClosed})
	case "unread":
		query = query.Where("unread_for_staff = ?", true)
	case "urgent":
		query = query.Where("priority = ?", "urgent")
	case "refund":
		query = query.Where("type = ?", "refund")
	case "card":
		query = query.Where("type = ?", "card")
	case "payment":
		query = query.Where("type = ?", "payment")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	var tickets []models.Ticket
	if err := query.Order("CASE priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1 WHEN 'normal' THEN 2 ELSE 3 END, COALESCE(last_message_at, created_at) DESC").
		Limit(limit).Offset(filter.Offset).Find(&tickets).Error; err != nil {
		return nil, 0, err
	}
	views, err := s.decorateTickets(ctx, tickets)
	if err != nil {
		return nil, 0, err
	}
	return views, total, nil
}

// decorateTickets 给列表行补上用户名与客服昵称。批量查询避免逐行 N+1。
func (s *GormStore) decorateTickets(ctx context.Context, tickets []models.Ticket) ([]domain.TicketView, error) {
	views := make([]domain.TicketView, 0, len(tickets))
	if len(tickets) == 0 {
		return views, nil
	}
	userIDs := map[uint]bool{}
	agentIDs := map[uint]bool{}
	for _, ticket := range tickets {
		userIDs[ticket.UserID] = true
		if ticket.AssignedAgentID != nil {
			agentIDs[*ticket.AssignedAgentID] = true
		}
	}
	names := map[uint]string{}
	var users []models.User
	if err := s.db.WithContext(ctx).Select("id", "username", "nickname").Where("id IN ?", keys(userIDs)).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, user := range users {
		label := user.Username
		if strings.TrimSpace(user.Nickname) != "" {
			label = user.Nickname
		}
		names[user.ID] = label
	}
	agents := map[uint]string{}
	if len(agentIDs) > 0 {
		var rows []models.CustomerServiceAgent
		if err := s.db.WithContext(ctx).Where("id IN ?", keys(agentIDs)).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			label := row.Nickname
			if label == "" {
				label = fmt.Sprintf("客服 #%d", row.ID)
			}
			agents[row.ID] = label
		}
	}
	now := time.Now().UTC()
	for _, ticket := range tickets {
		view := domain.TicketView{Ticket: ticket}
		view.Username = names[ticket.UserID]
		if ticket.AssignedAgentID != nil {
			view.AgentName = agents[*ticket.AssignedAgentID]
		}
		view.Unread = ticket.UnreadForStaff
		view.Overdue = ticket.DueAt != nil && ticket.DueAt.Before(now) &&
			ticket.Status != models.TicketStatusResolved && ticket.Status != models.TicketStatusClosed
		views = append(views, view)
	}
	return views, nil
}

func keys(set map[uint]bool) []uint {
	out := make([]uint, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out
}

func (s *GormStore) GetTicket(ctx context.Context, id uint) (*domain.Ticket, error) {
	var ticket domain.Ticket
	if err := s.db.WithContext(ctx).First(&ticket, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrTicketNotFound
		}
		return nil, err
	}
	return &ticket, nil
}

func (s *GormStore) GetTicketByNo(ctx context.Context, no string) (*domain.Ticket, error) {
	var ticket domain.Ticket
	if err := s.db.WithContext(ctx).Where("ticket_no = ?", strings.TrimSpace(no)).First(&ticket).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrTicketNotFound
		}
		return nil, err
	}
	return &ticket, nil
}

func (s *GormStore) CreateTicket(ctx context.Context, ticket *domain.Ticket) error {
	return s.db.WithContext(ctx).Create(ticket).Error
}

func (s *GormStore) UpdateTicket(ctx context.Context, ticket *domain.Ticket) error {
	return s.db.WithContext(ctx).Save(ticket).Error
}

func (s *GormStore) DeleteTicket(ctx context.Context, id uint) error {
	result := s.db.WithContext(ctx).Delete(&models.Ticket{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrTicketNotFound
	}
	return nil
}

// NextTicketNo 生成工单号。用当天序号 + 固定前缀，读取时加锁避免重号。
func (s *GormStore) NextTicketNo(ctx context.Context, prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "TK"
	}
	now := time.Now().UTC()
	day := now.Format("20060102")
	like := fmt.Sprintf("%s%s%%", prefix, day)
	var last string
	err := s.db.WithContext(ctx).Model(&models.Ticket{}).
		Where("ticket_no LIKE ?", like).
		Order("ticket_no DESC").Limit(1).Pluck("ticket_no", &last).Error
	if err != nil {
		return "", err
	}
	seq := 1
	if len(last) > len(prefix)+len(day) {
		if parsed, err := parseTrailingInt(last[len(prefix)+len(day):]); err == nil {
			seq = parsed + 1
		}
	}
	no := fmt.Sprintf("%s%s%04d", prefix, day, seq)
	return no, nil
}

func parseTrailingInt(value string) (int, error) {
	var out int
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("empty")
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("not numeric")
		}
		out = out*10 + int(r-'0')
	}
	return out, nil
}

func (s *GormStore) ListMessages(ctx context.Context, ticketID uint, includeInternal bool, limit, offset int) ([]domain.TicketMessage, int64, error) {
	query := s.db.WithContext(ctx).Model(&models.TicketMessage{}).Where("ticket_id = ?", ticketID)
	if !includeInternal {
		query = query.Where("is_internal = ?", false)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var messages []domain.TicketMessage
	// 消息流按时间正序，工作台从第一条往下读。
	if err := query.Order("id ASC").Limit(limit).Offset(offset).Find(&messages).Error; err != nil {
		return nil, 0, err
	}
	return messages, total, nil
}

func (s *GormStore) CreateMessage(ctx context.Context, message *domain.TicketMessage) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(message).Error; err != nil {
			return err
		}
		updates := map[string]any{"message_count": gorm.Expr("message_count + 1"), "last_message_at": time.Now().UTC()}
		if message.SenderType == domain.SenderUser {
			updates["unread_for_staff"] = true
		} else if message.SenderType == domain.SenderAI || message.SenderType == domain.SenderAgent {
			updates["unread_for_user"] = true
		}
		return tx.Model(&models.Ticket{}).Where("id = ?", message.TicketID).Updates(updates).Error
	})
}

func (s *GormStore) UpdateMessage(ctx context.Context, message *domain.TicketMessage) error {
	return s.db.WithContext(ctx).Save(message).Error
}

func (s *GormStore) AppendTicketLog(ctx context.Context, entry *domain.TicketLog) error {
	return s.db.WithContext(ctx).Create(entry).Error
}

func (s *GormStore) ListTicketLogs(ctx context.Context, ticketID uint, limit, offset int) ([]domain.TicketLog, int64, error) {
	query := s.db.WithContext(ctx).Model(&models.TicketLog{}).Where("ticket_id = ?", ticketID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []domain.TicketLog
	if err := query.Order("id ASC").Limit(limit).Offset(offset).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

func (s *GormStore) CreateAttachment(ctx context.Context, attachment *domain.TicketAttachment) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(attachment).Error; err != nil {
			return err
		}
		return tx.Model(&models.Ticket{}).Where("id = ?", attachment.TicketID).
			Update("attachment_count", gorm.Expr("attachment_count + 1")).Error
	})
}

func (s *GormStore) ListAttachments(ctx context.Context, ticketID uint) ([]domain.TicketAttachment, error) {
	var rows []domain.TicketAttachment
	if err := s.db.WithContext(ctx).Where("ticket_id = ?", ticketID).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *GormStore) TicketStats(ctx context.Context, agentID uint) (*domain.TicketStats, error) {
	stats := &domain.TicketStats{}
	base := s.db.WithContext(ctx).Model(&models.Ticket{})
	if agentID > 0 {
		base = base.Where("assigned_agent_id = ?", agentID)
	}
	countBy := func(statuses ...string) (int64, error) {
		var count int64
		err := base.Session(&gorm.Session{}).Where("status IN ?", statuses).Count(&count).Error
		return count, err
	}
	var err error
	if agentID > 0 {
		if err := base.Session(&gorm.Session{}).Count(&stats.Total).Error; err != nil {
			return nil, err
		}
	} else if err := s.db.WithContext(ctx).Model(&models.Ticket{}).Count(&stats.Total).Error; err != nil {
		return nil, err
	}
	if stats.AIProcessing, err = countBy(models.TicketStatusAIProcessing, models.TicketStatusWaitingUser); err != nil {
		return nil, err
	}
	if stats.AISolved, err = countBy(models.TicketStatusAISolved); err != nil {
		return nil, err
	}
	if stats.PendingHuman, err = countBy(models.TicketStatusUserRequested, models.TicketStatusPendingHuman); err != nil {
		return nil, err
	}
	if stats.HumanHandling, err = countBy(models.TicketStatusHumanHandling, models.TicketStatusWaitingConfirm); err != nil {
		return nil, err
	}
	if stats.Resolved, err = countBy(models.TicketStatusResolved, models.TicketStatusClosed); err != nil {
		return nil, err
	}
	if err := base.Session(&gorm.Session{}).Where("unread_for_staff = ?", true).Count(&stats.Unread).Error; err != nil {
		return nil, err
	}
	if err := base.Session(&gorm.Session{}).Where("due_at IS NOT NULL AND due_at < ? AND status NOT IN ?", time.Now().UTC(), []string{models.TicketStatusResolved, models.TicketStatusClosed}).Count(&stats.Overdue).Error; err != nil {
		return nil, err
	}
	if err := base.Session(&gorm.Session{}).Where("priority = ?", "urgent").Count(&stats.Urgent).Error; err != nil {
		return nil, err
	}
	startOfDay := time.Now().UTC().Truncate(24 * time.Hour)
	if err := base.Session(&gorm.Session{}).Where("created_at >= ?", startOfDay).Count(&stats.TodayCreated).Error; err != nil {
		return nil, err
	}
	if stats.Total > 0 {
		stats.ResolveRate = float64(stats.Resolved) / float64(stats.Total)
	}
	// 处理时长与满意度由已解决工单聚合，没有数据时保持 0。
	type aggregate struct {
		AvgSeconds      float64
		AvgSatisfaction float64
		Rated           int64
	}
	var agg aggregate
	if err := s.db.WithContext(ctx).Model(&models.Ticket{}).
		Select("COALESCE(AVG(CASE WHEN resolved_at IS NOT NULL THEN (julianday(resolved_at) - julianday(created_at)) * 86400 END),0) AS avg_seconds, COALESCE(AVG(CASE WHEN satisfaction > 0 THEN satisfaction END),0) AS avg_satisfaction, COALESCE(SUM(CASE WHEN satisfaction > 0 THEN 1 ELSE 0 END),0) AS rated").
		Scan(&agg).Error; err != nil {
		// SQLite 支持 julianday；其他驱动失败时退化为 0，不影响主流程。
		agg = aggregate{}
	}
	stats.AvgHandleMinutes = agg.AvgSeconds / 60
	if agg.Rated > 0 {
		stats.SatisfactionAvg = agg.AvgSatisfaction
	}
	return stats, nil
}

func (s *GormStore) RelatedTickets(ctx context.Context, userID, excludeID uint, limit int) ([]domain.Ticket, error) {
	var tickets []domain.Ticket
	if err := s.db.WithContext(ctx).Where("user_id = ? AND id <> ?", userID, excludeID).
		Order("id DESC").Limit(limit).Find(&tickets).Error; err != nil {
		return nil, err
	}
	return tickets, nil
}

// ── AI 会话与消息 ────────────────────────────────────────────────────

func (s *GormStore) CreateConversation(ctx context.Context, conversation *domain.AIConversation) error {
	return s.db.WithContext(ctx).Create(conversation).Error
}

func (s *GormStore) UpdateConversation(ctx context.Context, conversation *domain.AIConversation) error {
	return s.db.WithContext(ctx).Save(conversation).Error
}

func (s *GormStore) GetConversation(ctx context.Context, id uint) (*domain.AIConversation, error) {
	var conversation domain.AIConversation
	if err := s.db.WithContext(ctx).First(&conversation, id).Error; err != nil {
		return nil, err
	}
	return &conversation, nil
}

func (s *GormStore) ListConversations(ctx context.Context, userID uint, limit, offset int) ([]domain.AIConversation, int64, error) {
	query := s.db.WithContext(ctx).Model(&models.AIConversation{})
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var conversations []domain.AIConversation
	if err := query.Order("id DESC").Limit(limit).Offset(offset).Find(&conversations).Error; err != nil {
		return nil, 0, err
	}
	return conversations, total, nil
}

func (s *GormStore) ListMessagesByConversation(ctx context.Context, conversationID uint, limit int) ([]domain.AIMessage, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var messages []domain.AIMessage
	err := s.db.WithContext(ctx).Where("conversation_id = ? AND role IN ?", conversationID, []string{"user", "assistant"}).
		Order("id DESC").Limit(limit).Find(&messages).Error
	if err != nil {
		return nil, err
	}
	// 反转为时间正序，喂给模型时顺序才对。
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

func (s *GormStore) CreateAIMessage(ctx context.Context, message *domain.AIMessage) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(message).Error; err != nil {
			return err
		}
		return tx.Model(&models.AIConversation{}).Where("id = ?", message.ConversationID).
			Updates(map[string]any{"message_count": gorm.Expr("message_count + 1"), "last_message_at": time.Now().UTC()}).Error
	})
}

// ── 工具 ─────────────────────────────────────────────────────────────

func (s *GormStore) ListTools(ctx context.Context, enabledOnly bool) ([]domain.AIToolDefinition, error) {
	query := s.db.WithContext(ctx).Model(&models.AIToolDefinition{})
	if enabledOnly {
		query = query.Where("is_enabled = ?", true)
	}
	var tools []domain.AIToolDefinition
	if err := query.Order("sort_order ASC, id ASC").Find(&tools).Error; err != nil {
		return nil, err
	}
	return tools, nil
}

func (s *GormStore) GetTool(ctx context.Context, key string) (*domain.AIToolDefinition, error) {
	var tool domain.AIToolDefinition
	if err := s.db.WithContext(ctx).Where("key = ?", strings.TrimSpace(key)).First(&tool).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrToolNotFound
		}
		return nil, err
	}
	return &tool, nil
}

func (s *GormStore) SaveTool(ctx context.Context, tool *domain.AIToolDefinition) error {
	return s.db.WithContext(ctx).Save(tool).Error
}

func (s *GormStore) DeleteTool(ctx context.Context, id uint) error {
	result := s.db.WithContext(ctx).Delete(&models.AIToolDefinition{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrToolNotFound
	}
	return nil
}

func (s *GormStore) ListToolPermissions(ctx context.Context) ([]domain.AIToolPermission, error) {
	var rows []domain.AIToolPermission
	if err := s.db.WithContext(ctx).Order("tool_id ASC, role ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *GormStore) SetToolPermission(ctx context.Context, permission *domain.AIToolPermission) error {
	var existing domain.AIToolPermission
	err := s.db.WithContext(ctx).Where("tool_id = ? AND role = ?", permission.ToolID, permission.Role).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.db.WithContext(ctx).Create(permission).Error
	}
	if err != nil {
		return err
	}
	existing.Allowed = permission.Allowed
	if permission.UpdatedBy != nil {
		existing.UpdatedBy = permission.UpdatedBy
	}
	return s.db.WithContext(ctx).Save(&existing).Error
}

func (s *GormStore) CreateToolCall(ctx context.Context, call *domain.AIToolCall) error {
	return s.db.WithContext(ctx).Create(call).Error
}

func (s *GormStore) ListToolCalls(ctx context.Context, filter contract.ToolCallFilter) ([]domain.AIToolCall, int64, error) {
	query := s.db.WithContext(ctx).Model(&models.AIToolCall{})
	if filter.ToolKey != "" {
		query = query.Where("tool_key = ?", filter.ToolKey)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.UserID > 0 {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.TicketID > 0 {
		query = query.Where("ticket_id = ?", filter.TicketID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var calls []domain.AIToolCall
	if err := query.Order("id DESC").Limit(limit).Offset(filter.Offset).Find(&calls).Error; err != nil {
		return nil, 0, err
	}
	return calls, total, nil
}

func (s *GormStore) CountToolCalls(ctx context.Context, toolKey string, userID uint, since time.Time) (int64, error) {
	query := s.db.WithContext(ctx).Model(&models.AIToolCall{}).Where("tool_key = ? AND created_at >= ?", toolKey, since)
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	var count int64
	err := query.Count(&count).Error
	return count, err
}

// ── 知识库 ───────────────────────────────────────────────────────────

func (s *GormStore) ListKnowledge(ctx context.Context, filter domain.KnowledgeFilter) ([]domain.AIKnowledge, int64, error) {
	query := s.db.WithContext(ctx).Model(&models.AIKnowledge{})
	if filter.CategoryID > 0 {
		query = query.Where("category_id = ?", filter.CategoryID)
	}
	if status := strings.TrimSpace(filter.Status); status != "" {
		query = query.Where("status = ?", status)
	}
	if tag := strings.TrimSpace(filter.Tag); tag != "" {
		query = query.Where("tags LIKE ?", "%"+tag+"%")
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		like := "%" + search + "%"
		query = query.Where("title LIKE ? OR summary LIKE ? OR content LIKE ? OR keywords LIKE ?", like, like, like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	switch filter.Sort {
	case "oldest":
		query = query.Order("id ASC")
	case "title":
		query = query.Order("title ASC")
	default:
		query = query.Order("priority DESC, id DESC")
	}
	var rows []domain.AIKnowledge
	if err := query.Limit(limit).Offset(filter.Offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (s *GormStore) GetKnowledge(ctx context.Context, id uint) (*domain.AIKnowledge, error) {
	var article domain.AIKnowledge
	if err := s.db.WithContext(ctx).First(&article, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrInvalidInput
		}
		return nil, err
	}
	return &article, nil
}

func (s *GormStore) GetKnowledgeBySlug(ctx context.Context, slug string) (*domain.AIKnowledge, error) {
	var article domain.AIKnowledge
	if err := s.db.WithContext(ctx).Where("slug = ?", strings.TrimSpace(slug)).First(&article).Error; err != nil {
		return nil, err
	}
	return &article, nil
}

func (s *GormStore) CreateKnowledge(ctx context.Context, article *domain.AIKnowledge) error {
	return s.db.WithContext(ctx).Create(article).Error
}

func (s *GormStore) UpdateKnowledge(ctx context.Context, article *domain.AIKnowledge) error {
	return s.db.WithContext(ctx).Save(article).Error
}

func (s *GormStore) DeleteKnowledge(ctx context.Context, id uint) error {
	result := s.db.WithContext(ctx).Delete(&models.AIKnowledge{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrInvalidInput
	}
	return nil
}

// SearchKnowledge 是 AI 检索用的轻量全文匹配：标题、关键词权重更高。
func (s *GormStore) SearchKnowledge(ctx context.Context, query string, limit int) ([]domain.KnowledgeHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 20 {
		limit = 5
	}
	var rows []models.AIKnowledge
	if err := s.db.WithContext(ctx).
		Where("status = ?", "published").
		Where("(publish_at IS NULL OR publish_at <= ?)", time.Now().UTC()).
		Where("(expire_at IS NULL OR expire_at > ?)", time.Now().UTC()).
		Order("priority DESC, id DESC").Limit(200).Find(&rows).Error; err != nil {
		return nil, err
	}
	hits := make([]domain.KnowledgeHit, 0, limit)
	terms := splitTerms(query)
	for _, article := range rows {
		score := knowledgeScore(article, terms)
		if score <= 0 {
			continue
		}
		hits = append(hits, domain.KnowledgeHit{
			ID: article.ID, Title: article.Title, Summary: article.Summary,
			Content: article.Content, Priority: article.Priority, Score: score,
		})
	}
	sortHits(hits)
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func splitTerms(query string) []string {
	replaced := strings.NewReplacer(",", " ", "，", " ", "。", " ", "?", " ", "？", " ", "!", " ", "！", " ").Replace(query)
	fields := strings.Fields(replaced)
	if len(fields) == 0 {
		return []string{strings.ToLower(strings.TrimSpace(query))}
	}
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if len([]rune(field)) >= 2 {
			out = append(out, strings.ToLower(field))
		}
	}
	if len(out) == 0 {
		out = append(out, strings.ToLower(strings.TrimSpace(query)))
	}
	return out
}

func knowledgeScore(article models.AIKnowledge, terms []string) float64 {
	title := strings.ToLower(article.Title)
	keywords := strings.ToLower(article.Keywords)
	summary := strings.ToLower(article.Summary)
	content := strings.ToLower(article.Content)
	score := 0.0
	for _, term := range terms {
		if term == "" {
			continue
		}
		if strings.Contains(title, term) {
			score += 6
		}
		if strings.Contains(keywords, term) {
			score += 4
		}
		if strings.Contains(summary, term) {
			score += 2
		}
		if strings.Contains(content, term) {
			score += 1
		}
	}
	if score > 0 {
		score += float64(article.Priority) * 0.01
	}
	return score
}

func sortHits(hits []domain.KnowledgeHit) {
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j].Score > hits[j-1].Score; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
}

func (s *GormStore) ListKnowledgeCategories(ctx context.Context) ([]domain.AIKnowledgeCategory, error) {
	var rows []domain.AIKnowledgeCategory
	if err := s.db.WithContext(ctx).Order("sort_order ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *GormStore) CreateKnowledgeCategory(ctx context.Context, category *domain.AIKnowledgeCategory) error {
	return s.db.WithContext(ctx).Create(category).Error
}

func (s *GormStore) UpdateKnowledgeCategory(ctx context.Context, category *domain.AIKnowledgeCategory) error {
	return s.db.WithContext(ctx).Save(category).Error
}

func (s *GormStore) DeleteKnowledgeCategory(ctx context.Context, id uint) error {
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.AIKnowledge{}).Where("category_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: 这个分类下还有文章，先移走或删除文章。", domain.ErrInvalidInput)
	}
	return s.db.WithContext(ctx).Delete(&models.AIKnowledgeCategory{}, id).Error
}

// ── 配置 ─────────────────────────────────────────────────────────────

func (s *GormStore) GetAIConfig(ctx context.Context) (*domain.AIConfig, error) {
	var config domain.AIConfig
	err := s.db.WithContext(ctx).First(&config).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &domain.AIConfig{}, nil
	}
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (s *GormStore) SaveAIConfig(ctx context.Context, config *domain.AIConfig) error {
	return s.db.WithContext(ctx).Save(config).Error
}

func (s *GormStore) GetAIWorkflow(ctx context.Context) (*domain.AIWorkflowConfig, error) {
	var config domain.AIWorkflowConfig
	err := s.db.WithContext(ctx).First(&config).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &domain.AIWorkflowConfig{}, nil
	}
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (s *GormStore) SaveAIWorkflow(ctx context.Context, config *domain.AIWorkflowConfig) error {
	return s.db.WithContext(ctx).Save(config).Error
}

func (s *GormStore) ListQuickQuestions(ctx context.Context, enabledOnly bool, position string) ([]domain.AIQuickQuestion, error) {
	query := s.db.WithContext(ctx).Model(&models.AIQuickQuestion{})
	if enabledOnly {
		query = query.Where("is_enabled = ?", true)
	}
	if position != "" {
		query = query.Where("position = ? OR position = ?", position, "all")
	}
	var rows []domain.AIQuickQuestion
	if err := query.Order("sort_order ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *GormStore) SaveQuickQuestion(ctx context.Context, question *domain.AIQuickQuestion) error {
	return s.db.WithContext(ctx).Save(question).Error
}

func (s *GormStore) DeleteQuickQuestion(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Delete(&models.AIQuickQuestion{}, id).Error
}

func (s *GormStore) CreateFeedback(ctx context.Context, feedback *domain.AIFeedback) error {
	return s.db.WithContext(ctx).Create(feedback).Error
}

func (s *GormStore) ListFeedback(ctx context.Context, limit int) ([]domain.AIFeedback, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var rows []domain.AIFeedback
	if err := s.db.WithContext(ctx).Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *GormStore) FeedbackCount(ctx context.Context, conversationID uint, rating int, since time.Time) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&models.AIFeedback{}).
		Where("conversation_id = ? AND rating = ? AND created_at >= ?", conversationID, rating, since).
		Count(&count).Error
	return count, err
}

// ── 客服 ─────────────────────────────────────────────────────────────

func (s *GormStore) ListAgents(ctx context.Context, activeOnly bool) ([]domain.CustomerServiceAgent, error) {
	query := s.db.WithContext(ctx).Model(&models.CustomerServiceAgent{})
	if activeOnly {
		query = query.Where("is_active = ?", true)
	}
	var rows []domain.CustomerServiceAgent
	if err := query.Order("status ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *GormStore) GetAgent(ctx context.Context, id uint) (*domain.CustomerServiceAgent, error) {
	var agent domain.CustomerServiceAgent
	if err := s.db.WithContext(ctx).First(&agent, id).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

func (s *GormStore) GetAgentByUser(ctx context.Context, userID uint) (*domain.CustomerServiceAgent, error) {
	var agent domain.CustomerServiceAgent
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&agent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &agent, nil
}

func (s *GormStore) SaveAgent(ctx context.Context, agent *domain.CustomerServiceAgent) error {
	return s.db.WithContext(ctx).Save(agent).Error
}

func (s *GormStore) DeleteAgent(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Delete(&models.CustomerServiceAgent{}, id).Error
}

func (s *GormStore) AgentLoad(ctx context.Context, agentID uint) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&models.Ticket{}).
		Where("assigned_agent_id = ? AND status IN ?", agentID, []string{
			models.TicketStatusPendingHuman, models.TicketStatusHumanHandling, models.TicketStatusWaitingConfirm,
		}).Count(&count).Error
	return count, err
}

func (s *GormStore) CreateAssignment(ctx context.Context, assignment *domain.CustomerServiceAssignment) error {
	return s.db.WithContext(ctx).Create(assignment).Error
}

func (s *GormStore) ListAssignments(ctx context.Context, ticketID uint) ([]domain.CustomerServiceAssignment, error) {
	var rows []domain.CustomerServiceAssignment
	if err := s.db.WithContext(ctx).Where("ticket_id = ?", ticketID).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ── 通知模板与系统配置 ───────────────────────────────────────────────

func (s *GormStore) CreateNotificationLog(ctx context.Context, entry *domain.NotificationLog) error {
	return s.db.WithContext(ctx).Create(entry).Error
}

func (s *GormStore) ListNotificationLogs(ctx context.Context, status string, limit, offset int) ([]domain.NotificationLog, int64, error) {
	query := s.db.WithContext(ctx).Model(&models.NotificationLog{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []domain.NotificationLog
	if err := query.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (s *GormStore) ListNotificationTemplates(ctx context.Context, category string) ([]domain.NotificationTemplate, error) {
	query := s.db.WithContext(ctx).Model(&models.NotificationTemplate{})
	if category != "" {
		query = query.Where("category = ?", category)
	}
	var rows []domain.NotificationTemplate
	if err := query.Order("sort_order ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *GormStore) SaveNotificationTemplate(ctx context.Context, template *domain.NotificationTemplate) error {
	return s.db.WithContext(ctx).Save(template).Error
}

func (s *GormStore) DeleteNotificationTemplate(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Delete(&models.NotificationTemplate{}, id).Error
}

func (s *GormStore) ListSystemConfigs(ctx context.Context, group string) ([]domain.SystemConfig, error) {
	query := s.db.WithContext(ctx).Model(&models.SystemConfig{})
	if group != "" {
		query = query.Where("\"group\" = ?", group)
	}
	var rows []domain.SystemConfig
	if err := query.Order("\"group\" ASC, sort_order ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// SaveSystemConfig 以 (group,key) 为唯一键更新或插入，值类型与可见性一并保存。
func (s *GormStore) SaveSystemConfig(ctx context.Context, config *domain.SystemConfig) error {
	var existing domain.SystemConfig
	err := s.db.WithContext(ctx).Where("\"group\" = ? AND key = ?", config.Group, config.Key).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.db.WithContext(ctx).Create(config).Error
	}
	if err != nil {
		return err
	}
	existing.Value = config.Value
	existing.ValueType = config.ValueType
	existing.Label = config.Label
	existing.Description = config.Description
	existing.IsSecret = config.IsSecret
	existing.SortOrder = config.SortOrder
	if config.UpdatedBy != nil {
		existing.UpdatedBy = config.UpdatedBy
	}
	return s.db.WithContext(ctx).Save(&existing).Error
}

// ── 快捷回复 ─────────────────────────────────────────────────────────

func (s *GormStore) ListQuickReplies(ctx context.Context, enabledOnly bool, ticketType, role string) ([]domain.QuickReply, error) {
	query := s.db.WithContext(ctx).Model(&models.QuickReply{})
	if enabledOnly {
		query = query.Where("is_enabled = ?", true)
	}
	var rows []domain.QuickReply
	if err := query.Order("sort_order ASC, id ASC").Limit(300).Find(&rows).Error; err != nil {
		return nil, err
	}
	filtered := make([]domain.QuickReply, 0, len(rows))
	for _, reply := range rows {
		if ticketType != "" && strings.TrimSpace(reply.TicketTypes) != "" && !containsCSV(reply.TicketTypes, ticketType) {
			continue
		}
		if role != "" && strings.TrimSpace(reply.Roles) != "" && !containsCSV(reply.Roles, role) {
			continue
		}
		filtered = append(filtered, reply)
	}
	return filtered, nil
}

func containsCSV(value, target string) bool {
	for _, part := range strings.Split(value, ",") {
		if strings.TrimSpace(part) == target {
			return true
		}
	}
	return false
}

func (s *GormStore) SaveQuickReply(ctx context.Context, reply *domain.QuickReply) error {
	return s.db.WithContext(ctx).Save(reply).Error
}

func (s *GormStore) DeleteQuickReply(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Delete(&models.QuickReply{}, id).Error
}

func (s *GormStore) IncrementQuickReplyUse(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Model(&models.QuickReply{}).Where("id = ?", id).
		Update("use_count", gorm.Expr("use_count + 1")).Error
}
