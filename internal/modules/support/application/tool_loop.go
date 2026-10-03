package application

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/domain"
)

// ToolSummary 是一条工具说明，用于写进系统提示词，也是后台
// 「AI 可调用的工具」卡片的展示数据，所以字段要有 JSON 名。
type ToolSummary struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Params      []string `json:"params"`
}

// enabledToolCatalogue 返回「当前启用、且这个角色可以用」的工具清单。
//
// 这是 AI 知道自己能做什么的唯一来源：提示词里列出什么，它才可能调用什么；
// 执行时还会再校验一次（CallTool），所以写进提示词不会扩大权限。
func (s *Service) enabledToolCatalogue(ctx context.Context, role string) []ToolSummary {
	s.registerBuiltins()
	stored, err := s.repo.ListTools(ctx, true)
	if err != nil {
		return nil
	}
	byKey := make(map[string]domain.AIToolDefinition, len(stored))
	for _, tool := range stored {
		byKey[tool.Key] = tool
	}
	out := make([]ToolSummary, 0, len(stored))
	for _, spec := range s.tools.All() {
		record, ok := byKey[spec.Key]
		if !ok || !record.IsEnabled {
			continue
		}
		allowed, err := s.toolAllowedForRole(ctx, &record, role)
		if err != nil || !allowed {
			continue
		}
		out = append(out, ToolSummary{
			Key:         spec.Key,
			Name:        spec.Name,
			Description: strings.TrimSpace(spec.Description),
			Params:      spec.AllowedParams,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// toolCallPattern 匹配模型输出的工具调用。
//
// 约定是一行 JSON：{"tool":"order.detail","params":{"order_no":"NL1"}}。
// 只认这个形状，避免把模型闲聊里的花括号误当成调用。
var toolCallPattern = regexp.MustCompile(`\{\s*"tool"\s*:\s*"([a-zA-Z0-9_.]+)"\s*,\s*"params"\s*:\s*(\{[^}]*\})\s*\}`)

// ParsedToolCall 是一次从模型输出里解析出来的工具调用。
type ParsedToolCall struct {
	ToolKey string
	Params  map[string]any
}

// parseToolCall 从模型回复里提取第一个工具调用；没有则返回 nil。
func parseToolCall(content string) *ParsedToolCall {
	match := toolCallPattern.FindStringSubmatch(content)
	if match == nil {
		return nil
	}
	params := map[string]any{}
	if err := json.Unmarshal([]byte(match[2]), &params); err != nil {
		// 参数不是合法 JSON：按无参数调用处理，让工具自己报缺参数，
		// 这样错误会出现在工具日志里，而不是静默丢失。
		params = map[string]any{}
	}
	return &ParsedToolCall{ToolKey: strings.TrimSpace(match[1]), Params: params}
}

// stripToolCall 从回复里去掉工具调用那一行，只保留给用户看的文字。
func stripToolCall(content string) string {
	return strings.TrimSpace(toolCallPattern.ReplaceAllString(content, ""))
}

// runToolLoop 执行「模型 → 工具 → 模型」的循环，最多 MaxToolRounds 轮。
//
// 每一轮：把当前消息与工具说明交给模型；如果模型输出工具调用，就执行它、
// 把结果作为一条 system 消息回喂，然后再次询问模型；直到模型给出最终回答。
// 每次调用都会走 CallTool 的权限/参数/限频校验，并写 ai_tool_calls。
func (s *Service) runToolLoop(
	ctx context.Context,
	config *domain.AIConfig,
	systemPrompt string,
	messages []contract.ModelMessage,
	input domain.ChatInput,
	conversationID uint,
) (string, []domain.ToolCallResult, error) {
	return s.runToolLoopWith(ctx, config, systemPrompt, messages, input, conversationID, true)
}

// runToolLoopWith 是工具循环的实现。allowConfirm 决定「需要用户二次确认」的工具
// 是返回确认卡片（网页客服），还是直接跳过（工单内的自动应答没有确认入口）。
func (s *Service) runToolLoopWith(
	ctx context.Context,
	config *domain.AIConfig,
	systemPrompt string,
	messages []contract.ModelMessage,
	input domain.ChatInput,
	conversationID uint,
	allowConfirm bool,
) (string, []domain.ToolCallResult, error) {
	const MaxToolRounds = 3

	current := append([]contract.ModelMessage{}, messages...)
	var calls []domain.ToolCallResult
	lastAnswer := ""

	for round := 0; round < MaxToolRounds; round++ {
		reply, err := s.model.Complete(ctx, config, contract.ModelRequest{
			SystemPrompt: systemPrompt,
			Messages:     current,
			MaxTokens:    maxInt(config.MaxReplyLen, 2000) / 2,
			Temperature:  config.Temperature,
			TopP:         config.TopP,
			TimeoutMS:    config.TimeoutMS,
		})
		if err != nil {
			return "", calls, err
		}
		parsed := parseToolCall(reply.Content)
		if parsed == nil {
			answer := strings.TrimSpace(reply.Content)
			if answer != "" {
				lastAnswer = answer
			}
			return answer, calls, nil
		}
		// 保留这一轮的文字：模型可能先解释再调用工具，轮次用尽时用它兜底。
		if text := stripToolCall(reply.Content); text != "" {
			lastAnswer = text
		}
		// 工单里没有确认按钮，所以默认只能自动执行低风险工具。
		// 少数「动的是用户自己的东西」的工具（例如原路退款给本人）
		// 由具体请求本身构成同意，允许在工单场景直接执行。
		if !allowConfirm {
			spec, ok := s.tools.Lookup(parsed.ToolKey)
			if ok && spec.RequireConfirm && !ticketAutoAllowed[parsed.ToolKey] {
				current = append(current,
					contract.ModelMessage{Role: "assistant", Content: reply.Content},
					contract.ModelMessage{Role: "system", Content: "工具 " + parsed.ToolKey + " 需要用户本人确认，当前场景无法确认，请直接用已有信息回答，必要时建议转人工。"},
				)
				continue
			}
		}
		result, err := s.CallTool(ctx, domain.ToolCallInput{
			ToolKey:        parsed.ToolKey,
			UserID:         input.UserID,
			UserRole:       input.UserRole,
			ConversationID: conversationID,
			TicketID:       input.TicketID,
			Params:         parsed.Params,
			IP:             input.IP,
		})
		if result != nil {
			calls = append(calls, *result)
		}
		if err != nil {
			// 工具失败也要回喂给模型，让它能向用户解释，而不是直接崩。
			current = append(current,
				contract.ModelMessage{Role: "assistant", Content: reply.Content},
				contract.ModelMessage{Role: "system", Content: "工具 " + parsed.ToolKey + " 执行失败：" + toolFailureText(err)},
			)
			continue
		}
		current = append(current,
			contract.ModelMessage{Role: "assistant", Content: reply.Content},
			contract.ModelMessage{Role: "system", Content: "工具 " + parsed.ToolKey + " 返回：" + jsonString(result.Data, 2000)},
		)
	}
	// 轮次用尽：用模型最后一段文字兜底，避免什么都不回。
	return lastAnswer, calls, nil
}

// toolFailureText 把工具错误翻成模型能理解的句子。
func toolFailureText(err error) string {
	if err == nil {
		return "未知错误"
	}
	return strings.TrimSpace(strings.TrimPrefix(err.Error(), "invalid input:"))
}

// ToolCatalogue 是后台能看到的「AI 可用工具」清单，与写进提示词的内容一致。
func (s *Service) ToolCatalogue(ctx context.Context, role string) ([]ToolSummary, error) {
	if err := s.SyncToolDefinitions(ctx); err != nil {
		return nil, err
	}
	return s.enabledToolCatalogue(ctx, role), nil
}

// toolsForAdmin 给后台展示当前启用且可被 AI 调用的工具（含参数说明）。
func (s *Service) toolsForAdmin(ctx context.Context) ([]ToolSummary, error) {
	s.registerBuiltins()
	if err := s.SyncToolDefinitions(ctx); err != nil {
		return nil, err
	}
	stored, err := s.repo.ListTools(ctx, false)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]domain.AIToolDefinition, len(stored))
	for _, tool := range stored {
		byKey[tool.Key] = tool
	}
	out := make([]ToolSummary, 0, len(stored))
	for _, spec := range s.tools.All() {
		record, ok := byKey[spec.Key]
		if !ok {
			continue
		}
		out = append(out, ToolSummary{
			Key: spec.Key, Name: spec.Name,
			Description: spec.Description, Params: spec.AllowedParams,
		})
		_ = record
	}
	return out, nil
}

// ensure fmt stays used even if this file is trimmed later.
var _ = fmt.Sprintf

// ticketAutoAllowed 是工单场景里可以不打断用户直接执行的确认类工具。
// 它们共同的特征是只影响调用者本人，且用户这次提问本身就是请求。
var ticketAutoAllowed = map[string]bool{
	"refund.order":         true,
	"ticket.add_message":   true,
	"notification.send":    false,
	"ticket.update_status": false,
	"coupon.grant":         false,
}
