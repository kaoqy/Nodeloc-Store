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
	"github.com/kaoqy/Nodeloc-Store/internal/shared"
)

// NodeLocGateway implements contract.PaymentGateway using NodeLoc Payment.
type NodeLocGateway struct {
	baseURL   string
	paymentID string
	secretKey string
	client    *http.Client
}

func NewNodeLocGateway(baseURL, paymentID, secretKey string, client *http.Client) *NodeLocGateway {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &NodeLocGateway{
		baseURL:   strings.TrimRight(baseURL, "/"),
		paymentID: paymentID,
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
	body, raw, err := g.post(ctx, "/payment/pay/"+url.PathEscape(g.paymentID)+"/process", params)
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
	body, raw, err := g.post(ctx, "/payment/query/"+url.PathEscape(g.paymentID), params)
	if err != nil {
		return nil, err
	}
	return &contract.QueryPaymentResult{
		TransactionID: firstNonEmpty(firstString(body, "transaction_id", "trade_no", "id"), transactionID),
		OrderID:       firstString(body, "order_id", "order_no"),
		Amount:        firstInt(body, "amount", "total_amount"),
		Status:        normalizeStatus(firstString(body, "status", "state")),
		PlatformFee:   optionalInt(body, "platform_fee", "fee"),
		MerchantPoints: optionalInt(body, "merchant_points", "merchant_amount"),
		Raw:           raw,
	}, nil
}

func (g *NodeLocGateway) Transfer(ctx context.Context, request contract.TransferRequest) (*contract.TransferResult, error) {
	params := map[string]string{
		"to_user_id": request.ToUserID,
		"to_username": request.ToUsername,
		"amount":      strconv.Itoa(request.Amount),
		"order_id":    request.OrderID,
	}
	body, raw, err := g.post(ctx, "/payment/transfer/"+url.PathEscape(g.paymentID), params)
	if err != nil {
		return nil, err
	}
	return &contract.TransferResult{
		TransactionID: firstString(body, "transaction_id", "trade_no", "id"),
		Status:        normalizeStatus(firstString(body, "status", "state")),
		Raw:           raw,
	}, nil
}

// VerifyCallback uses the raw secret key, as required by NodeLoc callbacks.
func (g *NodeLocGateway) VerifyCallback(params map[string]string) bool {
	copyParams := make(map[string]string, len(params))
	for key, value := range params {
		copyParams[key] = value
	}
	return shared.VerifyCallback(copyParams, g.secretKey)
}

func (g *NodeLocGateway) post(ctx context.Context, path string, params map[string]string) (map[string]any, []byte, error) {
	if g.baseURL == "" || g.paymentID == "" || g.secretKey == "" {
		return nil, nil, errors.New("NodeLoc payment gateway is not fully configured")
	}

	params["signature"] = shared.SignWithHashedToken(params, g.secretKey)
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
