package infrastructure

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/domain"
)

// NewAPIRedemptionKey is the built-in provider for creating New-API redemption
// codes after payment.
const NewAPIRedemptionKey = "new-api-redemption-v1"

// errNewAPIDefiniteFailure marks responses that prove New-API rejected the
// request. These must surface as hard delivery errors instead of being
// downgraded to the uncertain (manual review) path, which exists only for
// responses whose outcome cannot be determined.
var errNewAPIDefiniteFailure = errors.New("New-API 明确拒绝了创建请求")

const (
	newAPIKeyLength     = 13
	newAPIKeyAlphabet   = "abcdefghijklmnopqrstuvwxyz0123456789"
	newAPIMaxQuota      = int64(1_000_000_000_000)
	newAPIMaxTimeout    = 30 * time.Second
	newAPIMaxResponse   = 1 << 20
	newAPIRequestAccept = "application/json"
)

// NewAPIRedemption creates one redemption code through New-API's admin API.
//
// New-API's documented contract is POST /api/redemption/ with a key and quota,
// answered by an ApiResponse envelope ({ success, message, data }) whose data
// carries the created Redemption (including key). The published create example
// does not show the exact payload, so the adapter accepts the envelope, a bare
// Redemption object, and a data string. Only an explicit success response with
// a key is treated as delivered; an unparseable or ambiguous body becomes an
// uncertain result so the order is never silently marked as delivered.
type NewAPIRedemption struct {
	client        *http.Client
	runtimeConfig contract.RuntimeConfigProvider
}

// withClient is package-private test wiring. Production always uses the guarded
// client built by NewNewAPIRedemption.
func (p *NewAPIRedemption) withClient(client *http.Client) *NewAPIRedemption {
	p.client = client
	return p
}

func NewNewAPIRedemption(runtimeConfig contract.RuntimeConfigProvider) *NewAPIRedemption {
	return &NewAPIRedemption{
		runtimeConfig: runtimeConfig,
		client: &http.Client{
			Timeout: newAPIMaxTimeout,
			Transport: &http.Transport{
				Proxy: nil,
				DialContext: (&net.Dialer{
					Timeout:   5 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   8 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
				ExpectContinueTimeout: 2 * time.Second,
				IdleConnTimeout:       60 * time.Second,
			},
		},
	}
}

func (p *NewAPIRedemption) Key() string { return NewAPIRedemptionKey }

func (p *NewAPIRedemption) Manifest() contract.Manifest {
	return contract.Manifest{
		Key:          NewAPIRedemptionKey,
		Name:         "New-API 兑换码",
		Description:  "付款成功后，由服务端向 New-API 创建兑换码并交付给买家；买家需要到 New-API 平台自行兑换，不会自动充值到账户。",
		Version:      "1.0.0",
		Author:       "NodeLoc Store",
		Capabilities: []string{domain.CapabilityForm, domain.CapabilityFulfill},
		ConfigSchema: []contract.ConfigField{
			{
				Key:         "base_url",
				Label:       "API 基础地址",
				Type:        "text",
				Required:    true,
				Placeholder: "https://new-api.example.com",
				Help:        "只填写站点根地址；系统会请求 {base_url}/api/redemption/。生产环境应使用 HTTPS，禁止内网、本机或云元数据地址。",
			},
			{
				Key:         "admin_access_token",
				Label:       "管理员 AccessToken",
				Type:        "password",
				Required:    true,
				Placeholder: "仅保存到服务端，不会回显",
				Help:        "对应 Authorization: Bearer 后的值。未修改时保持掩码，只有明确清除才会删除。",
			},
			{
				Key:         "admin_user_id",
				Label:       "管理员用户 ID",
				Type:        "number",
				Required:    true,
				Placeholder: "例如 1",
				Help:        "对应 New-Api-User 请求头，必须是正整数。",
			},
			{
				Key:         "quota_per_nl",
				Label:       "每 NL 兑换 quota",
				Type:        "number",
				Required:    true,
				Placeholder: "例如 100000",
				Help:        "quota = 订单实付 NL 金额 × quota_per_nl。只能为正整数，后端会重新计算，不采用前端 quota。",
			},
			{
				Key:         "success_field",
				Label:       "成功响应的兑换码字段",
				Type:        "text",
				Required:    false,
				Placeholder: "默认 data.key",
				Help:        "New-API 官方 ApiResponse 为 { success, message, data }，兑换码在 Redemption.key。默认按 data.key 提取；仅在自建版本改过响应结构时才需要覆盖。",
			},
		},
		// These fields describe the buyer's purchase form. They are delivered to
		// the order for display and support, but never sent in New-API's JSON.
		FormSchema: []contract.FormField{
			{
				Key:         "nl_account",
				Label:       "New-API 账号或用户名",
				Type:        "text",
				Required:    true,
				Placeholder: "用于交付记录，不会自动充值",
				MaxLength:   120,
			},
			{
				Key:         "nl_amount",
				Label:       "购买 NL 数量",
				Type:        "number",
				Required:    true,
				Placeholder: "例如 100",
				MaxLength:   18,
			},
		},
	}
}

// Validate checks the complete configuration before the channel may be enabled.
func (p *NewAPIRedemption) Validate(config map[string]string, secrets map[string]string) error {
	if _, err := normalizeBaseURL(config["base_url"]); err != nil {
		return err
	}
	if strings.TrimSpace(secrets["admin_access_token"]) == "" {
		return errors.New("请填写 New-API 管理员 AccessToken")
	}
	if _, err := positiveInt(config["admin_user_id"], "管理员用户 ID"); err != nil {
		return err
	}
	if _, err := positiveQuota(config["quota_per_nl"], "每 NL 兑换 quota"); err != nil {
		return err
	}
	return nil
}

func (p *NewAPIRedemption) Deliver(ctx context.Context, request contract.DeliveryRequest) (contract.DeliveryResult, error) {
	config, err := decodePluginConfig(request.FormValues["__plugin_config"])
	if err != nil {
		return contract.DeliveryResult{}, fmt.Errorf("New-API 配置无法读取：%w", err)
	}
	secrets, err := decodePluginConfig(request.FormValues["__plugin_secrets"])
	if err != nil {
		return contract.DeliveryResult{}, fmt.Errorf("New-API 凭据无法读取：%w", err)
	}
	if p.runtimeConfig != nil {
		runtimeSettings, runtimeSecrets, err := p.runtimeConfig.NewAPIConfig(ctx)
		if err != nil {
			return contract.DeliveryResult{}, fmt.Errorf("读取 New-API 运行时配置失败：%w", err)
		}
		for key, value := range runtimeSettings {
			config[key] = value
		}
		for key, value := range runtimeSecrets {
			secrets[key] = value
		}
	}
	baseURL, err := normalizeBaseURL(config["base_url"])
	if err != nil {
		return contract.DeliveryResult{}, err
	}
	token, err := requiredSecret(secrets, "admin_access_token")
	if err != nil {
		return contract.DeliveryResult{}, err
	}
	adminID, err := positiveInt(config["admin_user_id"], "管理员用户 ID")
	if err != nil {
		return contract.DeliveryResult{}, err
	}
	perNL, err := positiveQuota(config["quota_per_nl"], "每 NL 兑换 quota")
	if err != nil {
		return contract.DeliveryResult{}, err
	}

	nlAmount, err := purchaseNLAmount(request.FormValues)
	if err != nil {
		return contract.DeliveryResult{}, err
	}
	quota, err := quotaForOrder(nlAmount, int64(request.Quantity), perNL)
	if err != nil {
		return contract.DeliveryResult{}, err
	}
	key, err := newAPIKey()
	if err != nil {
		return contract.DeliveryResult{}, fmt.Errorf("生成 New-API 兑换码失败: %w", err)
	}

	payload := struct {
		Key   string `json:"key"`
		Quota int64  `json:"quota"`
	}{Key: key, Quota: quota}
	body, err := json.Marshal(payload)
	if err != nil {
		return contract.DeliveryResult{}, fmt.Errorf("编码 New-API 请求失败: %w", err)
	}

	status, responseBody, err := p.post(ctx, baseURL, token, adminID, body)
	if err != nil {
		return contract.DeliveryResult{}, err
	}
	created, evidence, err := extractRedemptionResult(responseBody, status, strings.TrimSpace(config["success_field"]))
	if err != nil {
		if errors.Is(err, errNewAPIDefiniteFailure) {
			return contract.DeliveryResult{}, err
		}
		return contract.DeliveryResult{
			Content:   uncertainDeliveryContent(request, key, quota, status),
			Note:      "New-API 请求已发出，但响应无法确认兑换码是否创建；请管理员核对后再处理，系统不会自动重复创建。",
			Reference: "new-api:uncertain:" + request.OrderNo,
			Uncertain: true,
		}, nil
	}
	return contract.DeliveryResult{
		Content: fmt.Sprintf(
			"New-API 兑换码：%s\n兑换额度：%d quota\n\n这是一张 New-API 兑换码，请到 New-API 平台自行兑换。\n%s",
			created, quota, evidence,
		),
		Note:      "New-API 兑换码已创建；此为兑换码交付，不是自动充值到账。",
		Reference: "new-api:" + created,
	}, nil
}

func (p *NewAPIRedemption) post(ctx context.Context, baseURL, token, adminID string, body []byte) (int, []byte, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/api/redemption/"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, fmt.Errorf("创建 New-API 请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("New-Api-User", adminID)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", newAPIRequestAccept)

	resp, err := p.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("New-API 请求失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, newAPIMaxResponse+1))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("读取 New-API 响应失败: %w", err)
	}
	if len(raw) > newAPIMaxResponse {
		return resp.StatusCode, nil, errors.New("New-API 响应过大，已停止读取")
	}
	return resp.StatusCode, raw, nil
}

func newAPIKey() (string, error) {
	out := make([]byte, newAPIKeyLength)
	for i := range out {
		value, err := rand.Int(rand.Reader, bigInt(len(newAPIKeyAlphabet)))
		if err != nil {
			return "", err
		}
		out[i] = newAPIKeyAlphabet[value.Int64()]
	}
	return string(out), nil
}

// bigInt is a tiny local alias so key generation has no dependency on
// math/big beyond this adapter.
func bigInt(max int) *big.Int {
	return big.NewInt(int64(max))
}

func normalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("请填写 New-API API 基础地址")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("New-API API 基础地址必须包含 http 或 https 协议和主机名")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("New-API API 基础地址只允许 http 或 https")
	}
	if parsed.User != nil {
		return "", errors.New("New-API API 基础地址不能包含用户名或密码")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("New-API API 基础地址不能包含查询参数或片段")
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" || isUnsafeHost(host) {
		return "", errors.New("New-API API 基础地址不能指向本机、内网或云元数据地址")
	}
	if parsed.Scheme != "https" && !isLoopbackDevelopmentHost(host) {
		return "", errors.New("New-API API 基础地址在生产环境必须使用 HTTPS")
	}
	return parsed.String(), nil
}

func isUnsafeHost(host string) bool {
	blocked := []string{
		"localhost", "localhost.localdomain", "metadata.google.internal",
		"169.254.169.254", "100.100.100.200",
	}
	for _, item := range blocked {
		if host == item {
			return true
		}
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.IsUnspecified()
	}
	return false
}

func isLoopbackDevelopmentHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func positiveInt(raw, label string) (string, error) {
	value := strings.TrimSpace(raw)
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 || strconv.FormatInt(parsed, 10) != value {
		return "", fmt.Errorf("%s 必须是正整数", label)
	}
	return value, nil
}

func positiveQuota(raw, label string) (int64, error) {
	value := strings.TrimSpace(raw)
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 || strconv.FormatInt(parsed, 10) != value {
		return 0, fmt.Errorf("%s 必须是正整数", label)
	}
	if parsed > newAPIMaxQuota {
		return 0, fmt.Errorf("%s 超过允许上限", label)
	}
	return parsed, nil
}

func purchaseNLAmount(formValues map[string]string) (int64, error) {
	raw := strings.TrimSpace(formValues["nl_amount"])
	if raw == "" {
		return 0, errors.New("购买表单缺少 NL 数量")
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, errors.New("NL 数量必须是正整数")
	}
	return value, nil
}

func quotaForOrder(nlAmount, quantity int64, quotaPerNL int64) (int64, error) {
	if nlAmount <= 0 {
		return 0, errors.New("NL 数量必须大于 0")
	}
	if quantity <= 0 {
		return 0, errors.New("订单数量必须大于 0")
	}
	quota := nlAmount * quotaPerNL
	if quota <= 0 || quota > newAPIMaxQuota {
		return 0, errors.New("计算得到的 quota 超出允许范围")
	}
	return quota, nil
}

func requiredSecret(config map[string]string, key string) (string, error) {
	value := strings.TrimSpace(config[key])
	if value == "" {
		return "", fmt.Errorf("New-API 缺少管理员 AccessToken")
	}
	return value, nil
}

// decodePluginConfig reads the non-secret settings plus write-only credentials
// that the application passes to providers in the delivery request. The
// application currently serializes only non-secret settings into __plugin_config;
// the credentials are deliberately injected separately below by the caller.
func decodePluginConfig(raw string) (map[string]string, error) {
	out := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func extractRedemptionResult(body []byte, status int, field string) (string, string, error) {
	if status < 200 || status >= 300 {
		return "", "", fmt.Errorf("%w: New-API 返回 HTTP %d", errNewAPIDefiniteFailure, status)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return "", "New-API 返回了空响应。", errors.New("New-API 空响应")
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "New-API 返回了非 JSON 响应。", errors.New("New-API 响应不是 JSON")
	}

	// New-API's documented admin response envelope is
	//   { "success": boolean, "message": string, "data": ... }
	// and the documented Redemption object carries key/quota/status.
	// The create endpoint's example is not published, so accept both the
	// envelope and a bare Redemption object, but never treat a missing key as
	// success.
	if success, ok := payload["success"].(bool); ok {
		if !success {
			message := textValue(payload["message"])
			if message == "" {
				message = "New-API 报告创建失败。"
			}
			return "", message, fmt.Errorf("%w: %s", errNewAPIDefiniteFailure, message)
		}
		if key, ok := redemptionKeyInData(payload["data"], field); ok {
			return key, "New-API 返回 success=true 且包含兑换码 key。", nil
		}
		return "", "New-API 报告 success=true，但没有返回兑换码 key。", errors.New("New-API 响应缺少兑换码")
	}

	// Some self-hosted builds return the Redemption object directly.
	if key := textValue(payload["key"]); key != "" {
		return key, "New-API 直接返回了兑换码对象。", nil
	}
	return "", "New-API 响应既没有 ApiResponse.success，也没有 Redemption.key。", errors.New("New-API 响应格式未知")
}

func redemptionKeyInData(data any, field string) (string, bool) {
	path := strings.TrimSpace(field)
	if path == "" {
		path = "key"
	}
	path = strings.TrimPrefix(path, "data.")
	// The envelope's data may be the redemption key itself rather than a
	// Redemption object, e.g. {"success":true,"data":"abc123xyz7890"}.
	if path == "key" {
		if text, ok := data.(string); ok {
			if key := strings.TrimSpace(text); key != "" {
				return key, true
			}
		}
	}
	current := data
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return "", false
		}
		next, ok := object[part]
		if !ok {
			return "", false
		}
		current = next
	}
	if key := textValue(current); key != "" {
		return key, true
	}
	// Redemption data may be nested under an object with a "key" member even
	// when the configured path points at the object itself.
	if object, ok := current.(map[string]any); ok {
		if key := textValue(object["key"]); key != "" {
			return key, true
		}
	}
	return "", false
}

func textValue(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func uncertainDeliveryContent(request contract.DeliveryRequest, key string, quota int64, status int) string {
	return fmt.Sprintf(
		"订单 %s\nNew-API 请求已发出（HTTP %d），但响应格式尚未确认。\n本次请求的兑换码键值：%s\n请求额度：%d quota\n\n为避免重复创建，系统未自动重试。请联系管理员核对 New-API 兑换码后再处理。",
		request.OrderNo, status, key, quota,
	)
}
