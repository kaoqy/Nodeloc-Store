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

// VerifyCallback checks the redirect against the merchant secret key, which is
// the credential the docs name for callbacks — the token is not involved here.
func (g *NodeLocGateway) VerifyCallback(params map[string]string) bool {
	return shared.VerifyCallback(params, g.secretKey)
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
	if g.baseURL == "" || g.paymentID == "" || g.secretKey == "" {
		return nil, nil, domain.ErrPaymentNotConfigured
	}
	key, err := keyOf()
	if err != nil {
		return nil, nil, err
	}

	params["signature"] = shared.Sign(params, key)
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+path, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, nil, fmt.Errorf("build NodeLoc request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("NodeLoc request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, nil, fmt.Errorf("read NodeLoc response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, raw, fmt.Errorf("NodeLoc returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, raw, fmt.Errorf("decode NodeLoc response: %w", err)
	}
	if success, ok := envelope["success"].(bool); ok && !success {
		return nil, raw, errors.New(firstNonEmpty(firstString(envelope, "message", "error", "detail"), "NodeLoc operation failed"))
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
