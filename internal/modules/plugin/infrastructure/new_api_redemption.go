package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"regexp"
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
	// newAPIQuotaPerUSD is the upstream's own unit: 500000 quota = 1 USD.
	newAPIQuotaPerUSD   = int64(500000)
	newAPIMaxQuota      = int64(1_000_000_000_000_000)
	newAPIMaxTimeout    = 30 * time.Second
	newAPIMaxResponse   = 1 << 20
	newAPIRequestAccept = "application/json"
)

// redemptionCodePattern bounds what a well-formed upstream code looks like so a
// stray HTML fragment or whitespace blob is never stored as a deliverable.
var redemptionCodePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{6,128}$`)

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
				Key:         "nl_usd_rate",
				Label:       "NL 与美元兑换比例",
				Type:        "text",
				Required:    true,
				Placeholder: "例如 1（表示 1 NL = 1 美元）",
				Help:        "填写“1 NL 等于多少美元”。系统按 订单实付 NL × 该比例 × 500000 计算上游 quota（上游以 500000 quota = 1 美元），必须为正数，且由服务端计算。",
			},
		},
		// The buyer's only New-API-specific input is the top-up amount itself.
		// It is stored on the order and used by the server to compute quota; it is
		// never sent in New-API's JSON, which only carries key and quota.
		FormSchema: []contract.FormField{
			{
				Key:         "nl_amount",
				Label:       "本次充值额度",
				Type:        "number",
				Required:    true,
				Placeholder: "例如 100",
				Help:        "必须在该商品的单次最少与最多充值额度之间。",
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
	if _, err := positiveRate(config["nl_usd_rate"], "NL 与美元兑换比例"); err != nil {
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
	rate, err := positiveRate(config["nl_usd_rate"], "NL 与美元兑换比例")
	if err != nil {
		return contract.DeliveryResult{}, err
	}

	// The chargeable amount is the amount the server actually recorded for this
	// order, never a client-supplied number. Both the redemption name and the
	// quota derive from it.
	paidNL, err := paidNLAmount(request)
	if err != nil {
		return contract.DeliveryResult{}, err
	}
	quota, err := quotaForPaidNL(paidNL, rate)
	if err != nil {
		return contract.DeliveryResult{}, err
	}

	payload := struct {
		Name  string `json:"name"`
		Quota int64  `json:"quota"`
		Count int    `json:"count"`
	}{Name: fmt.Sprintf("%dNL", paidNL), Quota: quota, Count: 1}
	body, err := json.Marshal(payload)
	if err != nil {
		return contract.DeliveryResult{}, fmt.Errorf("编码 New-API 请求失败: %w", err)
	}

	status, responseBody, err := p.post(ctx, baseURL, token, adminID, body)
	if err != nil {
		return contract.DeliveryResult{}, err
	}
	created, evidence, err := extractRedemptionResult(responseBody, status)
	if err != nil {
		if errors.Is(err, errNewAPIDefiniteFailure) {
			return contract.DeliveryResult{}, err
		}
		return contract.DeliveryResult{
			Content:   uncertainDeliveryContent(request, fmt.Sprintf("%dNL", paidNL), quota, status),
			Note:      "New-API 请求已发出，但响应无法确认兑换码是否创建；请管理员核对后再处理，系统不会自动重复创建。",
			Reference: "new-api:uncertain:" + request.OrderNo,
			Uncertain: true,
		}, nil
	}
	return contract.DeliveryResult{
		Content: fmt.Sprintf(
			"New-API 兑换码：%s\n本次支付：%d NL\n兑换额度：%d quota\n\n这是一张 New-API 兑换码，请到 New-API 平台自行兑换。\n%s",
			created, paidNL, quota, evidence,
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

// positiveRate parses the configured NL→USD rate. It is a decimal, so the value
// keeps the operator's precision instead of being truncated to an integer.
func positiveRate(raw, label string) (*big.Rat, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, fmt.Errorf("%s 不能为空", label)
	}
	rate, ok := new(big.Rat).SetString(value)
	if !ok || rate.Sign() <= 0 {
		return nil, fmt.Errorf("%s 必须是大于 0 的数字", label)
	}
	return rate, nil
}

// paidNLAmount reads the NL amount the server recorded for the order. It never
// trusts a client-supplied figure: the amount comes from the order row, which
// was written from the verified payment. Orders from before the column existed
// fall back to the stored purchase form so they stay deliverable.
func paidNLAmount(request contract.DeliveryRequest) (int64, error) {
	if request.PaidNLAmount > 0 {
		return int64(request.PaidNLAmount), nil
	}
	if request.TopupAmount > 0 {
		return int64(request.TopupAmount), nil
	}
	raw := strings.TrimSpace(request.FormValues["nl_amount"])
	if raw == "" {
		return 0, errors.New("订单缺少已支付的 NL 金额")
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, errors.New("订单的 NL 金额必须是正整数")
	}
	return value, nil
}

// quotaForPaidNL computes the upstream quota with exact rational arithmetic:
//
//	quota = paid NL × NL→USD rate × 500000
//
// A non-integral result is rejected rather than silently rounded, so a bad rate
// surfaces as a traceable error instead of an off-by-a-fraction redemption.
func quotaForPaidNL(paidNL int64, rate *big.Rat) (int64, error) {
	if paidNL <= 0 {
		return 0, errors.New("已支付的 NL 金额必须大于 0")
	}
	if rate == nil || rate.Sign() <= 0 {
		return 0, errors.New("NL 与美元兑换比例必须是大于 0 的数字")
	}
	exact := new(big.Rat).Mul(new(big.Rat).SetInt64(paidNL), rate)
	exact.Mul(exact, new(big.Rat).SetInt64(newAPIQuotaPerUSD))
	if !exact.IsInt() {
		return 0, errors.New("按当前汇率换算出的 quota 不是整数，请调整 NL 与美元兑换比例")
	}
	quota := exact.Num()
	if !quota.IsInt64() {
		return 0, errors.New("换算得到的 quota 超出允许范围")
	}
	value := quota.Int64()
	if value <= 0 || value > newAPIMaxQuota {
		return 0, errors.New("换算得到的 quota 超出允许范围")
	}
	return value, nil
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

// extractRedemptionResult validates the upstream answer and returns the created
// code. The documented success shape is
//
//	{ "success": true, "message": "", "data": ["<code>", ...] }
//
// and HTTP 200 alone is never treated as success: success must be true and the
// data array must carry a well-formed code.
func extractRedemptionResult(body []byte, status int) (string, string, error) {
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
	success, ok := payload["success"].(bool)
	if !ok {
		return "", "New-API 响应缺少 success 字段。", errors.New("New-API 响应格式未知")
	}
	if !success {
		message := textValue(payload["message"])
		if message == "" {
			message = "New-API 报告创建失败。"
		}
		return "", message, fmt.Errorf("%w: %s", errNewAPIDefiniteFailure, message)
	}
	codes, err := redemptionCodes(payload["data"])
	if err != nil {
		return "", "New-API 报告成功但没有返回可用的兑换码。", err
	}
	return codes[0], "New-API 返回 success=true，并已从 data 数组提取兑换码。", nil
}

// redemptionCodes reads the code array the upstream returns under "data" and
// rejects anything that is not a clean, well-formed code. A missing array, an
// empty array or a malformed entry is a failure, never a successful delivery.
func redemptionCodes(data any) ([]string, error) {
	items, ok := data.([]any)
	if !ok || len(items) == 0 {
		return nil, errors.New("New-API 响应缺少兑换码数组")
	}
	codes := make([]string, 0, len(items))
	for _, item := range items {
		code, ok := item.(string)
		if !ok {
			return nil, errors.New("New-API 兑换码不是字符串")
		}
		code = strings.TrimSpace(code)
		if !redemptionCodePattern.MatchString(code) {
			return nil, errors.New("New-API 兑换码格式不合法")
		}
		codes = append(codes, code)
	}
	return codes, nil
}

func textValue(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func uncertainDeliveryContent(request contract.DeliveryRequest, name string, quota int64, status int) string {
	return fmt.Sprintf(
		"订单 %s\nNew-API 请求已发出（HTTP %d），但响应无法确认兑换码是否创建。\n兑换码名称：%s\n请求额度：%d quota\n\n为避免重复创建，系统未自动重试。请联系管理员核对 New-API 兑换码后再处理。",
		request.OrderNo, status, name, quota,
	)
}
