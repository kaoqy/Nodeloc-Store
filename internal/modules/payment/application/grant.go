package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/domain"
)

// maxGrantAmount is the shop's own ceiling for one 转账, not NodeLoc's. The
// provider also publishes a per-application minimum and maximum, and those stay
// authoritative: this only stops a mistyped field from asking for six figures.
const maxGrantAmount = 1000

// GrantFailure is a refused 店家转账: a code the interface can switch on, the
// status the route should answer with, and copy in the shop owner's language.
// NodeLoc refuses transfers in short English strings that each name a different
// thing to fix, so they are translated here instead of being thrown at a form.
type GrantFailure struct {
	Code      string
	Status    int
	Message   string
	Retryable bool
}

func (f *GrantFailure) Error() string { return f.Message }

// GrantInput is one 店家主动转账 from the shop's NodeLoc application to a buyer.
type GrantInput struct {
	UserID uint
	Amount int
	Note   string
	// OperatorID is who pressed the button. Their name is read back from the
	// account rather than trusted from the request body, so the ledger cannot be
	// made to blame somebody else.
	OperatorID uint
}

// Grant sends points to a buyer's NodeLoc account and records the attempt either
// way. A transfer that NodeLoc refused still belongs in the ledger: it is the
// difference between "the shop never paid you" and "the shop tried and was
// rejected by its own provider".
func (s *Service) Grant(ctx context.Context, input GrantInput) (*domain.Transfer, error) {
	if input.UserID == 0 || input.OperatorID == 0 {
		return nil, ErrInvalidInput
	}
	if input.Amount <= 0 || input.Amount > maxGrantAmount {
		return nil, &GrantFailure{Code: "grant_amount", Status: 400,
			Message: fmt.Sprintf("单次转账需为 1–%d NL 之间的整数。", maxGrantAmount)}
	}
	recipient, err := s.users.FindByID(ctx, input.UserID)
	if err != nil {
		return nil, fmt.Errorf("lookup grant recipient: %w", err)
	}
	if recipient == nil {
		// The row is gone rather than the request being malformed, and the owner
		// reading this has a stale user list on screen, not a typo to fix.
		return nil, &GrantFailure{Code: "grant_recipient_missing", Status: http.StatusNotFound,
			Message: "没有找到这个账号，它可能已被删除。刷新用户列表后再试。"}
	}
	if !recipient.IsActive {
		// A banned account is not "this order is not yours", which is what the
		// shared forbidden copy says; the owner needs to know to unban first.
		return nil, &GrantFailure{Code: "grant_recipient_inactive", Status: http.StatusConflict,
			Message: "这个账号已被禁用，请先在用户详情页恢复它，再转账。"}
	}
	if strings.TrimSpace(recipient.OAuthUID) == "" && strings.TrimSpace(recipient.OAuthUsername) == "" {
		return nil, ErrGrantRecipientUnknown
	}
	operator, err := s.users.FindByID(ctx, input.OperatorID)
	if err != nil {
		return nil, fmt.Errorf("lookup grant operator: %w", err)
	}
	if operator == nil || !operator.IsActive {
		return nil, ErrForbidden
	}
	transfer := &domain.Transfer{
		UserID:       recipient.ID,
		Username:     recipient.Username,
		ToUserID:     strings.TrimSpace(recipient.OAuthUID),
		ToUsername:   strings.TrimSpace(recipient.OAuthUsername),
		Reference:    newGrantRef(),
		Provider:     "nodeloc",
		Amount:       input.Amount,
		Status:       domain.StatusPending,
		Note:         trimRunes(input.Note, 200),
		OperatorID:   operator.ID,
		OperatorName: operator.Username,
	}

	result, grantErr := s.gateway.Transfer(ctx, contract.TransferRequest{
		ToUserID:   transfer.ToUserID,
		ToUsername: transfer.ToUsername,
		Amount:     transfer.Amount,
		OrderID:    transfer.Reference,
	})
	if grantErr != nil {
		return s.rejectGrant(ctx, transfer, classifyGrantError(grantErr))
	}
	// Only an answer that says it worked counts as money moved. A body with no
	// status is not a success — the refund path insists on the same, and a shop
	// that booked an unclear answer as a completed 转账 would refuse to send the
	// one that actually arrives.
	if result == nil || result.Status != domain.StatusSucceeded {
		return s.rejectGrant(ctx, transfer, grantUnanswered(result))
	}

	now := time.Now().UTC()
	transfer.Status = domain.StatusSucceeded
	transfer.ProviderTransactionID = stringPointer(strings.TrimSpace(result.TransactionID))
	transfer.CompletedAt = &now
	transfer.Detail = summarizePayload(result.Raw)
	if err := s.orders.CreateTransfer(ctx, transfer); err != nil {
		// NodeLoc has already moved the points, so a ledger write that fails must
		// not be reported as a failed transfer — it would invite a second one.
		log.Printf("payment grant %s succeeded but was not written to the ledger: %v", transfer.Reference, err)
	}
	s.notifyGrant(ctx, transfer)
	return transfer, nil
}

// rejectGrant writes the refusal into the ledger and hands the operator the
// sentence they saw, so the two can never disagree.
func (s *Service) rejectGrant(ctx context.Context, transfer *domain.Transfer, failure *GrantFailure) (*domain.Transfer, error) {
	transfer.Status = domain.StatusFailed
	transfer.Detail = failure.Message
	if err := s.orders.CreateTransfer(ctx, transfer); err != nil {
		log.Printf("payment grant %s to user %d: the refusal could not be written to the ledger: %v",
			transfer.Reference, transfer.UserID, err)
	}
	log.Printf("payment grant %s to user %d refused: %s", transfer.Reference, transfer.UserID, failure.Message)
	return nil, failure
}

// ListTransfers reads the 转账 ledger, newest first; userID 0 is the whole shop.
func (s *Service) ListTransfers(ctx context.Context, userID uint, limit, offset int) ([]domain.Transfer, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	return s.orders.ListTransfers(ctx, userID, limit, offset)
}

// notifyGrant tells the buyer the shop sent them points. The message says what
// the shop can actually know — how much, and why if the owner wrote a note — and
// where the balance lives, because it is NodeLoc's, not the storefront's.
func (s *Service) notifyGrant(ctx context.Context, transfer *domain.Transfer) {
	if s.events == nil || transfer == nil || transfer.UserID == 0 {
		return
	}
	content := "商店向你的 NodeLoc 账户转入了 " + strconv.Itoa(transfer.Amount) + " NL（流水号 " + transfer.Reference + "）。"
	if strings.TrimSpace(transfer.Note) != "" {
		content += "店家留言：" + transfer.Note
	}
	s.events.Publish(ctx, contract.BuyerEvent{
		UserID:  transfer.UserID,
		Type:    "transfer",
		Title:   "店家向你转账 " + strconv.Itoa(transfer.Amount) + " NL",
		Content: content,
	})
}

// classifyGrantError turns a provider error into shop-facing copy. Anything that
// is not a refusal keeps its own wording, because the two failure families —
// "the shop is misconfigured" and "NodeLoc said no" — are fixed differently.
func classifyGrantError(err error) *GrantFailure {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, domain.ErrPaymentNotConfigured), errors.Is(err, domain.ErrProviderUnreachable):
		// "The shop is misconfigured" and "the provider is down" already have
		// wording in Classify, and it is better than a re-translation here.
		shared := Classify(err)
		return &GrantFailure{Code: shared.Code, Status: shared.Status, Message: shared.Detail, Retryable: shared.Retryable}
	case !errors.Is(err, domain.ErrProviderRejected):
		return &GrantFailure{Code: "grant_failed", Status: 502, Message: err.Error()}
	}
	provider := strings.TrimSpace(err.Error())
	// A duplicate order_id comes back as the typed 下单-already-exists refusal, so
	// its text is our own sentinel, not NodeLoc's sentence. For a 转账 that means
	// the reference was taken already — which is exactly the answer that must not
	// be re-tried, and quoting our sentinel as 原文 would be a lie.
	var already *domain.PaymentAlreadyRequested
	if errors.As(err, &already) {
		return &GrantFailure{Code: "grant_reference_exists", Status: 409,
			Message: "这个转账单号 NodeLoc 已经收过，请到流水里确认这一笔是否已经到账，不要重复转出。"}
	}
	// The wrapped sentinel prefix is the transport's, not the provider's answer.
	if index := strings.Index(provider, ": "); index >= 0 && strings.Contains(provider[:index], "provider") {
		provider = strings.TrimSpace(provider[index+2:])
	}
	lowered := strings.ToLower(provider)
	quoted := func(tail string) string { return tail + "（NodeLoc 原文：" + provider + "）" }
	switch {
	case strings.Contains(lowered, "receiver id and username do not match"):
		return &GrantFailure{Code: "grant_recipient_mismatch", Status: 409,
			Message: quoted("NodeLoc 认为收款方对不上：这个店铺账号记录的 uid 与用户名不是同一个人。请让对方重新用 NodeLoc 登录一次以更新绑定。")}
	case strings.Contains(lowered, "cannot transfer to yourself"):
		return &GrantFailure{Code: "grant_self", Status: 409,
			Message: quoted("这笔转账的收款方就是商店自己的支付应用，NodeLoc 不允许自己给自己转。")}
	case strings.Contains(lowered, "insufficient balance"):
		return &GrantFailure{Code: "grant_insufficient_balance", Status: 409,
			Message: quoted("商店的 NodeLoc 余额不够这次转账。" + balanceHint(provider))}
	case strings.Contains(lowered, "below minimum limit"):
		return &GrantFailure{Code: "grant_below_minimum", Status: 400,
			Message: quoted("金额低于商店支付应用设定的最小转账额。" + limitHint(provider, "minimum"))}
	case strings.Contains(lowered, "exceeds maximum limit"):
		return &GrantFailure{Code: "grant_above_maximum", Status: 400,
			Message: quoted("金额超过商店支付应用设定的最大转账额。" + limitHint(provider, "maximum"))}
	case strings.Contains(lowered, "transfer feature is disabled"):
		return &GrantFailure{Code: "grant_disabled", Status: 409,
			Message: quoted("商店的支付应用没有开启转账功能，请到 NodeLoc 商户后台开启后再试。")}
	case strings.Contains(lowered, "invalid signature"), strings.Contains(lowered, "signature mismatch"), strings.Contains(lowered, "signature does not match"):
		return &GrantFailure{Code: "grant_signature", Status: 502,
			Message: quoted("签名未通过：支付凭据与 NodeLoc 记录的不一致，请到 设置 核对 Payment Token。")}
	case strings.Contains(lowered, "invalid amount"):
		return &GrantFailure{Code: "grant_amount", Status: 400,
			Message: quoted("NodeLoc 不接受这个转账金额，请填 1 以上的整数。")}
	case strings.Contains(lowered, "order already exists"):
		return &GrantFailure{Code: "grant_reference_exists", Status: 409,
			Message: quoted("这个转账单号 NodeLoc 已经收过，请到流水里确认这一笔是否已经到账，不要重复转出。")}
	default:
		return &GrantFailure{Code: "grant_rejected", Status: 502, Message: "NodeLoc 拒绝了这次转账：" + provider}
	}
}

// grantUnanswered words the case where NodeLoc replied without saying it worked.
func grantUnanswered(result *contract.TransferResult) *GrantFailure {
	if result == nil {
		return &GrantFailure{Code: "grant_unanswered", Status: 502,
			Message: "NodeLoc 没有给出转账结果，请先在流水里确认这一笔是否到账，不要立刻重转。"}
	}
	status := strings.TrimSpace(result.Status)
	if status == "" {
		status = "unknown"
	}
	message := "NodeLoc 回报这次转账未完成（状态 " + status + "）。请先在流水里确认这一笔是否到账，不要立刻重转。"
	if raw := summarizePayload(result.Raw); raw != "" {
		message += "（NodeLoc 原文：" + raw + "）"
	}
	return &GrantFailure{Code: "grant_unanswered", Status: 502, Message: message}
}

// balanceHint and limitHint lift the numbers out of NodeLoc's parenthetical so
// the owner reads "需要 500，现有 12" instead of parsing English.
func balanceHint(provider string) string {
	start := strings.Index(strings.ToLower(provider), "insufficient balance")
	if start < 0 {
		return ""
	}
	need, have := "", ""
	for _, part := range strings.Split(firstParenthesized(provider[start:]), ",") {
		number := numericTail(part)
		if number == "" {
			continue
		}
		switch {
		case strings.Contains(strings.ToLower(part), "need"):
			need = number
		case strings.Contains(strings.ToLower(part), "have"):
			have = number
		}
	}
	if need == "" || have == "" {
		return ""
	}
	return "本次需要 " + need + "，账户现有 " + have + "。"
}

func limitHint(provider, which string) string {
	start := strings.Index(strings.ToLower(provider), which)
	if start < 0 {
		return ""
	}
	limit := firstParenthesized(provider[start:])
	if limit == "" {
		return ""
	}
	return "限额为 " + limit + "。"
}

// firstParenthesized returns what NodeLoc wrote inside the first (…) group, which
// is where these refusals put their numbers: "Insufficient balance (need 5000,
// have 12)" carries both in one group, "…limit (10)" one.
func firstParenthesized(text string) string {
	open := strings.Index(text, "(")
	if open < 0 {
		return ""
	}
	rest := text[open+1:]
	closing := strings.Index(rest, ")")
	if closing < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:closing])
}

// numericTail picks the amount out of "need 5000" or "have 1,200".
func numericTail(part string) string {
	fields := strings.Fields(part)
	for i := len(fields) - 1; i >= 0; i-- {
		candidate := strings.Trim(fields[i], ".,:;")
		if candidate == "" {
			continue
		}
		if _, err := strconv.ParseFloat(strings.ReplaceAll(candidate, ",", ""), 64); err == nil {
			return candidate
		}
	}
	return ""
}

func summarizePayload(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	text := strings.Join(strings.Fields(string(raw)), " ")
	if len(text) > 480 {
		return text[:480] + "…"
	}
	return text
}

func trimRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

// newGrantRef builds the reference NodeLoc deduplicates transfers by. It carries
// a timestamp so a day of grants reads in order, and random bytes so two shops
// started in the same second cannot collide.
func newGrantRef() string {
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return fmt.Sprintf("NLG%d", time.Now().UnixNano())
	}
	return "NLG" + time.Now().UTC().Format("20060102150405") + strings.ToUpper(hex.EncodeToString(suffix))
}
