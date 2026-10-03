package application

import (
	"context"
	"errors"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/domain"
)

// stubRepo 实现工单与设置相关的仓储接口。测试只覆写自己关心的字段，
// 其余方法返回空结果而不是 panic。AI 相关的仓储方法已经随 AI 客服下线，
// 这里的表面只保留通知模板等仍在使用的部分。
type stubRepo struct {
	templates []domain.NotificationTemplate
}

func (s *stubRepo) ListTickets(context.Context, domain.TicketFilter) ([]domain.TicketView, int64, error) {
	return nil, 0, nil
}

func (s *stubRepo) GetTicket(context.Context, uint) (*domain.Ticket, error) {
	return nil, domain.ErrTicketNotFound
}

func (s *stubRepo) GetTicketByNo(context.Context, string) (*domain.Ticket, error) {
	return nil, domain.ErrTicketNotFound
}

func (s *stubRepo) CreateTicket(context.Context, *domain.Ticket) error   { return nil }
func (s *stubRepo) UpdateTicket(context.Context, *domain.Ticket) error   { return nil }
func (s *stubRepo) DeleteTicket(context.Context, uint) error             { return nil }
func (s *stubRepo) NextTicketNo(context.Context, string) (string, error) { return "TK0001", nil }

func (s *stubRepo) ListMessages(context.Context, uint, bool, int, int) ([]domain.TicketMessage, int64, error) {
	return nil, 0, nil
}

func (s *stubRepo) CreateMessage(context.Context, *domain.TicketMessage) error { return nil }
func (s *stubRepo) UpdateMessage(context.Context, *domain.TicketMessage) error { return nil }

func (s *stubRepo) AppendTicketLog(context.Context, *domain.TicketLog) error { return nil }

func (s *stubRepo) ListTicketLogs(context.Context, uint, int, int) ([]domain.TicketLog, int64, error) {
	return nil, 0, nil
}

func (s *stubRepo) CreateAttachment(context.Context, *domain.TicketAttachment) error { return nil }

func (s *stubRepo) ListAttachments(context.Context, uint) ([]domain.TicketAttachment, error) {
	return nil, nil
}

func (s *stubRepo) TicketStats(context.Context, uint) (*domain.TicketStats, error) {
	return &domain.TicketStats{}, nil
}

func (s *stubRepo) RelatedTickets(context.Context, uint, uint, int) ([]domain.Ticket, error) {
	return nil, nil
}

func (s *stubRepo) ListAgents(context.Context, bool) ([]domain.CustomerServiceAgent, error) {
	return nil, nil
}

func (s *stubRepo) GetAgent(context.Context, uint) (*domain.CustomerServiceAgent, error) {
	return nil, errors.New("not found")
}

func (s *stubRepo) GetAgentByUser(context.Context, uint) (*domain.CustomerServiceAgent, error) {
	return nil, nil
}

func (s *stubRepo) SaveAgent(context.Context, *domain.CustomerServiceAgent) error { return nil }
func (s *stubRepo) DeleteAgent(context.Context, uint) error                       { return nil }
func (s *stubRepo) AgentLoad(context.Context, uint) (int64, error)                { return 0, nil }

func (s *stubRepo) CreateAssignment(context.Context, *domain.CustomerServiceAssignment) error {
	return nil
}

func (s *stubRepo) ListAssignments(context.Context, uint) ([]domain.CustomerServiceAssignment, error) {
	return nil, nil
}

func (s *stubRepo) ListQuickReplies(context.Context, bool, string, string) ([]domain.QuickReply, error) {
	return nil, nil
}

func (s *stubRepo) SaveQuickReply(context.Context, *domain.QuickReply) error { return nil }
func (s *stubRepo) DeleteQuickReply(context.Context, uint) error             { return nil }
func (s *stubRepo) IncrementQuickReplyUse(context.Context, uint) error       { return nil }

func (s *stubRepo) CreateNotificationLog(context.Context, *domain.NotificationLog) error {
	return nil
}

func (s *stubRepo) ListNotificationLogs(context.Context, string, int, int) ([]domain.NotificationLog, int64, error) {
	return nil, 0, nil
}

func (s *stubRepo) ListNotificationTemplates(context.Context, string) ([]domain.NotificationTemplate, error) {
	return s.templates, nil
}

func (s *stubRepo) SaveNotificationTemplate(context.Context, *domain.NotificationTemplate) error {
	return nil
}

func (s *stubRepo) DeleteNotificationTemplate(context.Context, uint) error { return nil }

func (s *stubRepo) ListSystemConfigs(context.Context, string) ([]domain.SystemConfig, error) {
	return nil, nil
}

func (s *stubRepo) SaveSystemConfig(context.Context, *domain.SystemConfig) error { return nil }

func (s *stubRepo) DeleteSystemConfig(context.Context, string, string) error { return nil }
