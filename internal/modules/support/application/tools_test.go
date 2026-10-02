package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/domain"
)

// stubRepo implements the support repository surface the tool tests touch.
// Every method the tests do not exercise returns an empty answer rather than
// panicking, so a test only has to override what it cares about.
type stubRepo struct {
	tools    map[string]domain.AIToolDefinition
	perm     []domain.AIToolPermission
	calls    int
	feedback []domain.AIFeedback
}

func (s *stubRepo) GetTool(_ context.Context, key string) (*domain.AIToolDefinition, error) {
	tool, ok := s.tools[key]
	if !ok {
		return nil, domain.ErrToolNotFound
	}
	return &tool, nil
}

func (s *stubRepo) ListToolPermissions(context.Context) ([]domain.AIToolPermission, error) {
	return s.perm, nil
}

func (s *stubRepo) CountToolCalls(context.Context, string, uint, time.Time) (int64, error) {
	return 0, nil
}

func (s *stubRepo) CreateToolCall(context.Context, *domain.AIToolCall) error { s.calls++; return nil }

func (s *stubRepo) ListToolCalls(context.Context, contract.ToolCallFilter) ([]domain.AIToolCall, int64, error) {
	return nil, 0, nil
}
func (s *stubRepo) ListTools(context.Context, bool) ([]domain.AIToolDefinition, error) {
	return nil, nil
}
func (s *stubRepo) SaveTool(context.Context, *domain.AIToolDefinition) error           { return nil }
func (s *stubRepo) DeleteTool(context.Context, uint) error                             { return nil }
func (s *stubRepo) SetToolPermission(context.Context, *domain.AIToolPermission) error  { return nil }
func (s *stubRepo) ListFeedback(context.Context, int) ([]domain.AIFeedback, error)     { return nil, nil }
func (s *stubRepo) CreateFeedback(context.Context, *domain.AIFeedback) error           { return nil }
func (s *stubRepo) FeedbackCount(context.Context, uint, int, time.Time) (int64, error) { return 0, nil }
func (s *stubRepo) GetAIConfig(context.Context) (*domain.AIConfig, error) {
	return &domain.AIConfig{}, nil
}
func (s *stubRepo) SaveAIConfig(context.Context, *domain.AIConfig) error { return nil }
func (s *stubRepo) GetAIWorkflow(context.Context) (*domain.AIWorkflowConfig, error) {
	return &domain.AIWorkflowConfig{}, nil
}
func (s *stubRepo) SaveAIWorkflow(context.Context, *domain.AIWorkflowConfig) error { return nil }
func (s *stubRepo) SearchKnowledge(context.Context, string, int) ([]domain.KnowledgeHit, error) {
	return nil, nil
}
func (s *stubRepo) ListKnowledge(context.Context, domain.KnowledgeFilter) ([]domain.AIKnowledge, int64, error) {
	return nil, 0, nil
}
func (s *stubRepo) GetKnowledge(context.Context, uint) (*domain.AIKnowledge, error) { return nil, nil }
func (s *stubRepo) GetKnowledgeBySlug(context.Context, string) (*domain.AIKnowledge, error) {
	return nil, nil
}
func (s *stubRepo) CreateKnowledge(context.Context, *domain.AIKnowledge) error { return nil }
func (s *stubRepo) UpdateKnowledge(context.Context, *domain.AIKnowledge) error { return nil }
func (s *stubRepo) DeleteKnowledge(context.Context, uint) error                { return nil }
func (s *stubRepo) ListKnowledgeCategories(context.Context) ([]domain.AIKnowledgeCategory, error) {
	return nil, nil
}
func (s *stubRepo) CreateKnowledgeCategory(context.Context, *domain.AIKnowledgeCategory) error {
	return nil
}
func (s *stubRepo) UpdateKnowledgeCategory(context.Context, *domain.AIKnowledgeCategory) error {
	return nil
}
func (s *stubRepo) DeleteKnowledgeCategory(context.Context, uint) error { return nil }
func (s *stubRepo) ListQuickQuestions(context.Context, bool, string) ([]domain.AIQuickQuestion, error) {
	return nil, nil
}
func (s *stubRepo) SaveQuickQuestion(context.Context, *domain.AIQuickQuestion) error { return nil }
func (s *stubRepo) DeleteQuickQuestion(context.Context, uint) error                  { return nil }
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
func (s *stubRepo) AppendTicketLog(context.Context, *domain.TicketLog) error   { return nil }
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
func (s *stubRepo) CreateConversation(context.Context, *domain.AIConversation) error { return nil }
func (s *stubRepo) UpdateConversation(context.Context, *domain.AIConversation) error { return nil }
func (s *stubRepo) GetConversation(context.Context, uint) (*domain.AIConversation, error) {
	return nil, errors.New("not found")
}
func (s *stubRepo) ListConversations(context.Context, uint, int, int) ([]domain.AIConversation, int64, error) {
	return nil, 0, nil
}
func (s *stubRepo) ListMessagesByConversation(context.Context, uint, int) ([]domain.AIMessage, error) {
	return nil, nil
}
func (s *stubRepo) CreateAIMessage(context.Context, *domain.AIMessage) error { return nil }
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
func (s *stubRepo) ListNotificationTemplates(context.Context, string) ([]domain.NotificationTemplate, error) {
	return nil, nil
}
func (s *stubRepo) SaveNotificationTemplate(context.Context, *domain.NotificationTemplate) error {
	return nil
}
func (s *stubRepo) DeleteNotificationTemplate(context.Context, uint) error { return nil }
func (s *stubRepo) ListSystemConfigs(context.Context, string) ([]domain.SystemConfig, error) {
	return nil, nil
}
func (s *stubRepo) SaveSystemConfig(context.Context, *domain.SystemConfig) error { return nil }

func newToolService(t *testing.T, repo *stubRepo) *Service {
	t.Helper()
	service, err := NewService(Deps{Repo: repo})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	service.registerBuiltins()
	return service
}

// TestCallToolRejectsUnknownTool is the first guard: a tool the operator never
// registered cannot be reached by naming it.
func TestCallToolRejectsUnknownTool(t *testing.T) {
	repo := &stubRepo{tools: map[string]domain.AIToolDefinition{}}
	service := newToolService(t, repo)
	if _, err := service.CallTool(context.Background(), domain.ToolCallInput{ToolKey: "sql.exec"}); !errors.Is(err, domain.ErrToolNotFound) {
		t.Fatalf("unknown tool answered %v, want ErrToolNotFound", err)
	}
	if repo.calls != 1 {
		t.Fatalf("refusal was not audited: calls=%d", repo.calls)
	}
}

// TestCallToolRejectsDisabledAndUnconfirmed covers the stored switch and the
// second-confirmation gate on a high-risk tool.
func TestCallToolRejectsDisabledAndUnconfirmed(t *testing.T) {
	repo := &stubRepo{tools: map[string]domain.AIToolDefinition{
		"ticket.update_status": {Base: models.Base{ID: 1}, Key: "ticket.update_status", Name: "更新工单状态", IsEnabled: false, RiskLevel: domain.RiskHigh},
		"order.detail":         {Base: models.Base{ID: 2}, Key: "order.detail", Name: "查询订单详情", IsEnabled: true, RiskLevel: domain.RiskLow, RequireConfirm: true},
	}}
	service := newToolService(t, repo)

	if _, err := service.CallTool(context.Background(), domain.ToolCallInput{ToolKey: "ticket.update_status", UserID: 7}); !errors.Is(err, domain.ErrToolDisabled) {
		t.Fatalf("disabled tool answered %v, want ErrToolDisabled", err)
	}
	result, err := service.CallTool(context.Background(), domain.ToolCallInput{ToolKey: "order.detail", UserID: 7, Params: map[string]any{"order_no": "NL1"}})
	if !errors.Is(err, domain.ErrToolConfirm) {
		t.Fatalf("confirm-required tool answered %v, want ErrToolConfirm", err)
	}
	if result == nil || !result.RequireConfirm {
		t.Fatalf("result did not ask for confirmation: %+v", result)
	}
}

// TestCallToolRejectsGuestAndForeignParams checks identity and the parameter
// allowlist: a guest cannot read orders, and an undeclared parameter is refused
// rather than silently dropped.
func TestCallToolRejectsGuestAndForeignParams(t *testing.T) {
	repo := &stubRepo{tools: map[string]domain.AIToolDefinition{
		"order.list": {Base: models.Base{ID: 1}, Key: "order.list", Name: "查询当前用户订单", IsEnabled: true, RiskLevel: domain.RiskLow},
	}}
	service := newToolService(t, repo)
	if _, err := service.CallTool(context.Background(), domain.ToolCallInput{ToolKey: "order.list", UserID: 0}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("guest answered %v, want ErrForbidden", err)
	}
	_, err := service.CallTool(context.Background(), domain.ToolCallInput{
		ToolKey: "order.list", UserID: 9,
		Params: map[string]any{"user_id": 123},
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("undeclared parameter answered %v, want ErrInvalidInput", err)
	}
}

// TestValidateParams enforces declared types so a string cannot stand in for an
// integer id.
func TestValidateParams(t *testing.T) {
	spec := &ToolSpec{
		Key: "order.detail", AllowedParams: []string{"order_no", "limit"},
		RequiredParams: []string{"order_no"},
		ParamTypes:     map[string]string{"order_no": "string", "limit": "int"},
	}
	if _, err := ValidateParams(spec, map[string]any{"limit": 5}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("missing required param answered %v", err)
	}
	if _, err := ValidateParams(spec, map[string]any{"order_no": "NL1", "limit": "many"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("wrong type answered %v", err)
	}
	clean, err := ValidateParams(spec, map[string]any{"order_no": " NL1 ", "limit": "3"})
	if err != nil {
		t.Fatalf("valid params refused: %v", err)
	}
	if clean["order_no"] != "NL1" || clean["limit"] != 3 {
		t.Fatalf("params were not normalized: %+v", clean)
	}
}
