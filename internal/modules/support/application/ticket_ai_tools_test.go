package application

import (
	"context"
	"strings"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/domain"
)

// scriptedModel 按脚本返回模型回复，用来验证「模型 → 工具 → 模型」的循环。
type scriptedModel struct {
	replies []string
	seen    []contract.ModelRequest
}

func (m *scriptedModel) Complete(_ context.Context, _ *domain.AIConfig, request contract.ModelRequest) (*contract.ModelReply, error) {
	m.seen = append(m.seen, request)
	index := len(m.seen) - 1
	if index >= len(m.replies) {
		return &contract.ModelReply{Content: "没有更多脚本了"}, nil
	}
	return &contract.ModelReply{Content: m.replies[index]}, nil
}

// stubOrders 只认一个属于用户 7 的订单。
type stubOrders struct{}

func (stubOrders) OrdersForUser(context.Context, uint, int) ([]contract.OrderContext, error) {
	return []contract.OrderContext{{OrderNo: "NL1", ProductName: "测试卡", TotalAmount: 100, Status: "paid"}}, nil
}
func (stubOrders) OrderForUser(_ context.Context, userID uint, orderNo string) (*contract.OrderContext, error) {
	if userID != 7 || orderNo != "NL1" {
		return nil, nil
	}
	return &contract.OrderContext{OrderNo: "NL1", ProductName: "测试卡", TotalAmount: 100, Status: "paid", FulfillmentStatus: "delivered"}, nil
}

// stubRefunder 记录退款调用，验证 AI 真能发起退款。
type stubRefunder struct {
	called  []string
	refunds int
}

func (r *stubRefunder) RefundableOrders(context.Context, uint, int) ([]contract.OrderContext, error) {
	return []contract.OrderContext{{OrderNo: "NL1", ProductName: "测试卡", TotalAmount: 100, Status: "paid"}}, nil
}
func (r *stubRefunder) RefundOrderForUser(_ context.Context, userID uint, orderNo string) (int, string, error) {
	if userID != 7 {
		return 0, "", domain.ErrForbidden
	}
	r.called = append(r.called, orderNo)
	r.refunds++
	return 100, "refunded", nil
}

func newChatService(t *testing.T, repo *stubRepo, model contract.ModelClient, refunder contract.Refunder) *Service {
	t.Helper()
	service, err := NewService(Deps{Repo: repo, Orders: stubOrders{}, Refunder: refunder, Model: model, SecretKey: "test"})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	service.registerBuiltins()
	return service
}

// TestToolLoopCallsOrderToolAndFeedsResultBack pins the core fix: the prompt tells
// the model which tools exist, the model asks for one, the tool runs, and the
// result is fed back before the final answer.
func TestToolLoopCallsOrderToolAndFeedsResultBack(t *testing.T) {
	repo := &stubRepo{tools: map[string]domain.AIToolDefinition{
		"order.detail": {Base: models.Base{ID: 1}, Key: "order.detail", Name: "查询订单详情", IsEnabled: true, RiskLevel: domain.RiskLow},
	}}
	model := &scriptedModel{replies: []string{
		"我查一下。\n{\"tool\":\"order.detail\",\"params\":{\"order_no\":\"NL1\"}}",
		"你的订单 NL1 已经付款并交付。",
	}}
	service := newChatService(t, repo, model, &stubRefunder{})

	answer, calls, err := service.runToolLoop(context.Background(),
		&domain.AIConfig{Model: "test", TimeoutMS: 3000, MaxReplyLen: 500},
		"你是客服",
		[]contract.ModelMessage{{Role: "user", Content: "我的订单 NL1 到哪了"}},
		domain.ChatInput{UserID: 7, UserRole: "user"}, 1,
	)
	if err != nil {
		t.Fatalf("tool loop failed: %v", err)
	}
	if len(calls) != 1 || calls[0].ToolKey != "order.detail" {
		t.Fatalf("tool calls = %+v", calls)
	}
	if !strings.Contains(answer, "已经付款并交付") {
		t.Fatalf("answer = %q", answer)
	}
	// 第二轮的对话里必须带上工具结果。
	if len(model.seen) != 2 {
		t.Fatalf("model rounds = %d, want 2", len(model.seen))
	}
	joined := ""
	for _, message := range model.seen[1].Messages {
		joined += message.Content
	}
	if !strings.Contains(joined, "NL1") {
		t.Fatalf("tool result was not fed back: %q", joined)
	}
}

// TestTicketAnswerUsesTools proves 工单里的 AI 不再绕过工具：一条退款请求会
// 真的执行退款，而不是只输出一段文字。
func TestTicketAnswerUsesTools(t *testing.T) {
	repo := &stubRepo{
		tools: map[string]domain.AIToolDefinition{
			"refund.list":  {Base: models.Base{ID: 1}, Key: "refund.list", Name: "查询可退款订单", IsEnabled: true, RiskLevel: domain.RiskLow},
			"refund.order": {Base: models.Base{ID: 2}, Key: "refund.order", Name: "发起订单退款", IsEnabled: true, RiskLevel: domain.RiskHigh},
		},
		aiConfig: &domain.AIConfig{Base: models.Base{ID: 1}, IsEnabled: true, Model: "test", TimeoutMS: 3000, MaxReplyLen: 500, MaxContext: 10, Temperature: 0.3, TopP: 1},
		workflow: &domain.AIWorkflowConfig{Base: models.Base{ID: 1}, CanRefund: true, TransferAfterDownvotes: 2},
	}
	model := &scriptedModel{replies: []string{
		"{\"tool\":\"refund.order\",\"params\":{\"order_no\":\"NL1\",\"reason\":\"卡密无效\"}}",
		"已经为你退款 100。",
	}}
	refunder := &stubRefunder{}
	service := newChatService(t, repo, model, refunder)

	answer, calls, err := service.AnswerTicket(context.Background(), &domain.Ticket{
		Base: models.Base{ID: 5}, TicketNo: "TK1", UserID: 7, Subject: "卡密无效，要求退款", Handler: models.TicketHandlerAI,
	}, nil)
	if err != nil {
		t.Fatalf("ticket answer failed: %v", err)
	}
	if refunder.refunds != 1 || len(refunder.called) != 1 {
		t.Fatalf("refund was not executed from the ticket: %+v", refunder)
	}
	if len(calls) != 1 || calls[0].ToolKey != "refund.order" {
		t.Fatalf("tool calls = %+v", calls)
	}
	if !strings.Contains(answer, "退款") {
		t.Fatalf("answer = %q", answer)
	}
}
