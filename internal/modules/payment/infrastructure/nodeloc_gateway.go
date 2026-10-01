package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/domain"
	"github.com/kaoqy/Nodeloc-Store/internal/shared"
)

// NodeLocGateway implements contract.PaymentGateway using NodeLoc Payment.
//
// NodeLoc hands a merchant three separate credentials and only one of them is
// interchangeable: the Payment ID (pay_xxx) selects the application in the URL,
// the Token (tk_xxx) signs 下单/转账, and the Secret Key signs 查单 and verifies
// the payment callback. The docs are explicit that the token is never used as an
// HMAC key directly — it is hashed with SHA-256 first — while the secret key is
// used as issued. Signing an outbound call with the wrong credential is what
// makes every payment feature fail at once.
type NodeLocGateway struct {
	baseURL   string
	paymentID string
	token     string
	secretKey string
	client    *http.Client
}

func NewNodeLocGateway(baseURL, paymentID, token, secretKey string, client *http.Client) *NodeLocGateway {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &NodeLocGateway{
		baseURL:   strings.TrimRight(baseURL, "/"),
		paymentID: paymentID,
		token:     token,
		secretKey: secretKey,
		client:    client,
	}
}

// Missing reports which of the four payment settings are still empty, named the
// way the 设置 page names its fields. A storefront that cannot take money is
// otherwise diagnosed by guessing, and the guess is usually wrong.
func (g *NodeLocGateway) Missing() []string {
	missing := make([]string, 0, 4)
	if g == nil {
		return append(missing, "payment 网关未初始化")
	}
	if strings.TrimSpace(g.baseURL) == "" {
		missing = append(missing, "base_url")
	}
	if strings.TrimSpace(g.paymentID) == "" {
		missing = append(missing, "payment_id")
	}
	if strings.TrimSpace(g.token) == "" {
		missing = append(missing, "token")
	}
	if strings.TrimSpace(g.secretKey) == "" {
		missing = append(missing, "secret_key")
	}
	return missing
}

func (g *NodeLocGateway) CreatePayment(ctx context.Context, request contract.CreatePaymentRequest) (*contract.CreatePaymentResult, error) {
	params := map[string]string{
		"amount":      strconv.Itoa(request.Amount),
		"description": request.Description,
		"order_id":    request.OrderID,
	}
	body, raw, err := g.post(ctx, "/payment/pay/"+url.PathEscape(g.paymentID)+"/process", params, g.tokenKey)
	if err != nil {
		return nil, err
	}
	return &contract.CreatePaymentResult{
		TransactionID: firstString(body, "transaction_id", "trade_no", "id"),
		PaymentURL:    firstString(body, "payment_url", "pay_url", "url", "redirect_url"),
		Status:        normalizeStatus(firstString(body, "status", "state")),
		Raw:           raw,
	}, nil
}

func (g *NodeLocGateway) QueryPayment(ctx context.Context, transactionID string) (*contract.QueryPaymentResult, error) {
	if transactionID == "" {
		return nil, errors.New("transaction_id is required")
	}
	params := map[string]string{"transaction_id": transactionID}
	body, raw, err := g.post(ctx, "/payment/query/"+url.PathEscape(g.paymentID), params, g.secretKeyFn)
	if err != nil {
		return nil, err
	}
	return &contract.QueryPaymentResult{
		TransactionID: firstNonEmpty(firstString(body, "transaction_id", "trade_no", "id"), transactionID),
		// 查单 returns our order number as external_reference; order_id is what we
		// sent when creating the payment, and some releases echo neither.
		OrderID:        firstString(body, "external_reference", "order_id", "order_no"),
		Amount:         firstInt(body, "amount", "total_amount"),
		Status:         normalizeStatus(firstString(body, "status", "state")),
		PlatformFee:    optionalInt(body, "platform_fee", "fee"),
		MerchantPoints: optionalInt(body, "merchant_points", "merchant_amount"),
		Raw:            raw,
	}, nil
}

func (g *NodeLocGateway) Transfer(ctx context.Context, request contract.TransferRequest) (*contract.TransferResult, error) {
	params := map[string]string{
		"to_user_id":  request.ToUserID,
		"to_username": request.ToUsername,
		"amount":      strconv.Itoa(request.Amount),
		"order_id":    request.OrderID,
	}
	body, raw, err := g.post(ctx, "/payment/transfer/"+url.PathEscape(g.paymentID), params, g.tokenKey)
	if err != nil {
		return nil, err
	}
	return &contract.TransferResult{
		TransactionID: firstString(body, "transaction_id", "trade_no", "id"),
		Status:        normalizeStatus(firstString(body, "status", "state")),
		Raw:           raw,
	}, nil
}

// VerifyCallback checks the redirect the buyer's browser comes back with.
//
// NodeLoc's own pages disagree about the key: one section names the merchant
// Secret Key, another says the redirect is HMAC'd with the SHA-256 of the tk_xxx
// token. Both credentials belong to this store's payment application, so a
// redirect that verifies under either one is genuinely NodeLoc's — and trying
// only the first is what sent real, signed payments to the 「signature」 failure
// page while the buyer had already paid.
func (g *NodeLocGateway) VerifyCallback(params map[string]string) bool {
	if g == nil {
		return false
	}
	if key := strings.TrimSpace(g.secretKey); key != "" && shared.VerifyCallback(params, key) {
		return true
	}
	if token := strings.TrimSpace(g.token); token != "" && shared.VerifyCallback(params, shared.HashedTokenKey(token)) {
		return true
	}
	return false
}

// tokenKey is the HMAC key NodeLoc expects on 下单 and 转账: the SHA-256 hex
// digest of the tk_xxx token, never the token itself.
func (g *NodeLocGateway) tokenKey() (string, error) {
	if strings.TrimSpace(g.token) == "" {
		return "", fmt.Errorf("%w：未填写 Payment Token（tk_xxx）", domain.ErrPaymentNotConfigured)
	}
	return shared.HashedTokenKey(g.token), nil
}

// secretKeyFn is the HMAC key NodeLoc expects on 查单: the secret key as issued.
func (g *NodeLocGateway) secretKeyFn() (string, error) {
	return g.secretKey, nil
}

func (g *NodeLocGateway) post(ctx context.Context, path string, params map[string]string, keyOf func() (string, error)) (map[string]any, []byte, error) {
	// Every call needs all four settings, even the ones the docs only sign with
	// one of them: a store that can 下单 but not 查单 would take money it can
	// never settle, which is a worse failure than refusing the sale. The message
	// names the missing fields so the shop owner fixes one setting instead of
	// guessing why the storefront says 无法支付.
	if missing := g.Missing(); len(missing) > 0 {
		return nil, nil, fmt.Errorf("%w：缺少 %s", domain.ErrPaymentNotConfigured, strings.Join(missing, "、"))
	}
	key, err := keyOf()
	if err != nil {
		return nil, nil, err
	}

	// NodeLoc recomputes the signature from the parameters it actually received,
	// and an empty one may not survive the trip. Dropping it from both the signed
	// string and the body keeps the two sides looking at the same fields — with a
	// recipient bound by NodeLoc uid only, signing "to_username=" would reject
	// the refund as tampered.
	for name, value := range params {
		if value == "" {
			delete(params, name)
		}
	}
	// The docs require a 10-digit second timestamp on every payment call and
	// refuse one that is more than five minutes from their clock. It goes in
	// before signing, so it is part of the sorted string like any other field;
	// a store whose clock has drifted gets its own diagnosis rather than the
	// generic 「凭据不匹配」.
	params["timestamp"] = strconv.FormatInt(time.Now().Unix(), 10)

	params["signature"] = shared.Sign(params, key)
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+path, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: build NodeLoc request: %v", domain.ErrProviderUnreachable, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", domain.ErrProviderUnreachable, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: read NodeLoc response: %v", domain.ErrProviderUnreachable, err)
	}
	if resp.StatusCode >= 500 {
		return nil, raw, fmt.Errorf("%w: NodeLoc returned HTTP %d: %s", domain.ErrProviderUnreachable, resp.StatusCode, summarize(raw))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, raw, refusal(fmt.Sprintf("NodeLoc returned HTTP %d: %s", resp.StatusCode, summarize(raw)))
	}

	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, raw, fmt.Errorf("%w: decode NodeLoc response: %v", domain.ErrProviderUnreachable, err)
	}
	if success, ok := envelope["success"].(bool); ok && !success {
		return nil, raw, refusal(firstNonEmpty(firstString(envelope, "message", "error", "detail"), "NodeLoc operation failed"))
	}
	if data, ok := envelope["data"].(map[string]any); ok {
		for key, value := range envelope {
			if _, exists := data[key]; !exists {
				data[key] = value
			}
		}
		return data, raw, nil
	}
	return envelope, raw, nil
}

// refusal reads a NodeLoc rejection. 「Order already exists with status …」 is the
// one that is not a fault: the payment already sits on NodeLoc's side under our
// order number, which is what happens every time a buyer returns to 立即购买 on an
// order they started before. Refusing it as a generic rejection is how a store
// ends up with orders nobody can ever pay.
func refusal(message string) error {
	if status, ok := alreadyRequestedStatus(message); ok {
		return &domain.PaymentAlreadyRequested{Status: status}
	}
	return fmt.Errorf("%w: %s", domain.ErrProviderRejected, message)
}

// alreadyRequestedStatus pulls the status word out of NodeLoc's duplicate-order
// refusal, in either language the provider may answer in. An empty word still
// means the order exists; only the caller's judgement about what to do with it
// changes.
func alreadyRequestedStatus(message string) (string, bool) {
	lower := strings.ToLower(message)
	marker := "order already exists"
	if !strings.Contains(lower, marker) {
		if !strings.Contains(lower, "订单已存在") {
			return "", false
		}
		marker = "订单已存在"
	}
	tail := lower[strings.Index(lower, marker)+len(marker):]
	tail = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(tail), "with"), "，")
	tail = strings.TrimPrefix(strings.TrimSpace(tail), "status")
	return strings.Trim(tail, " ：:。.，,\"'"), true
}

// summarize keeps a provider body short enough to log and to show a shop owner,
// and strips the whitespace that makes a raw dump unreadable in a toast.
func summarize(body []byte) string {
	text := strings.Join(strings.Fields(string(body)), " ")
	const limit = 240
	if len(text) > limit {
		return text[:limit] + "…"
	}
	if text == "" {
		return "empty response"
	}
	return text
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := values[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			if typed != "" {
				return typed
			}
		case json.Number:
			return typed.String()
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64)
		}
	}
	return ""
}

func firstInt(values map[string]any, keys ...string) int {
	for _, key := range keys {
		value, ok := values[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return int(typed)
		case json.Number:
			parsed, _ := strconv.Atoi(typed.String())
			return parsed
		case string:
			parsed, _ := strconv.Atoi(typed)
			return parsed
		}
	}
	return 0
}

func optionalInt(values map[string]any, keys ...string) *int {
	for _, key := range keys {
		if _, ok := values[key]; ok {
			value := firstInt(values, key)
			return &value
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func normalizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "paid", "success", "succeeded", "completed", "complete":
		return "succeeded"
	case "failed", "failure", "error", "cancelled", "canceled", "expired":
		return "failed"
	case "processing", "pending", "created", "unpaid", "":
		return "pending"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}
