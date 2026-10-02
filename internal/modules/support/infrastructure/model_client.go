package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/domain"
)

// ModelClient 通过 OpenAI 兼容的 /chat/completions 协议调用大模型。
// 店铺可以在后台改 API 地址与模型名，因此既能接官方服务，也能接自建网关。
type ModelClient struct {
	http *http.Client
}

func NewModelClient() *ModelClient {
	return &ModelClient{http: &http.Client{Timeout: 120 * time.Second}}
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	TopP        float64       `json:"top_p,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Complete 实现 contract.ModelClient。
func (c *ModelClient) Complete(ctx context.Context, config *domain.AIConfig, request contract.ModelRequest) (*contract.ModelReply, error) {
	if config == nil || strings.TrimSpace(config.BaseURL) == "" {
		return nil, domain.ErrNotConfigured
	}
	apiKey, err := decryptWith(config, request)
	if err != nil {
		return nil, err
	}
	_ = apiKey // 密钥由调用方解密后通过 header 传入见下
	return nil, domain.ErrNotConfigured
}

// decryptWith 只用于占位，真正的实现见 SecureModelClient。
func decryptWith(*domain.AIConfig, contract.ModelRequest) (string, error) {
	return "", errors.New("use SecureModelClient")
}

// SecureModelClient 在调用前解密 API Key，密钥不会以明文出现在任何读接口。
type SecureModelClient struct {
	inner     *ModelClient
	decryptor func(string) (string, error)
}

// NewSecureModelClient 组合密钥解密与 HTTP 调用。
func NewSecureModelClient(decryptor func(string) (string, error)) *SecureModelClient {
	return &SecureModelClient{inner: NewModelClient(), decryptor: decryptor}
}

func (c *SecureModelClient) Complete(ctx context.Context, config *domain.AIConfig, request contract.ModelRequest) (*contract.ModelReply, error) {
	if config == nil || strings.TrimSpace(config.BaseURL) == "" || strings.TrimSpace(config.Model) == "" {
		return nil, domain.ErrNotConfigured
	}
	apiKey := ""
	if c.decryptor != nil {
		key, err := c.decryptor(config.APIKeyEnc)
		if err != nil {
			return nil, fmt.Errorf("%w: API Key 无法解密，请重新保存。", domain.ErrNotConfigured)
		}
		apiKey = key
	}
	if apiKey == "" {
		return nil, fmt.Errorf("%w: 请先配置 API Key。", domain.ErrNotConfigured)
	}
	endpoint := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}
	payload := chatRequest{
		Model:       config.Model,
		Messages:    []chatMessage{{Role: "system", Content: request.SystemPrompt}},
		Temperature: request.Temperature, TopP: request.TopP, MaxTokens: request.MaxTokens,
	}
	for _, message := range request.Messages {
		role := message.Role
		switch role {
		case "user", "assistant", "system":
		default:
			role = "user"
		}
		payload.Messages = append(payload.Messages, chatMessage{Role: role, Content: message.Content})
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	timeout := request.TimeoutMS
	if timeout < 1000 {
		timeout = 30000
	}
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(callCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+apiKey)
	response, err := c.inner.http.Do(httpRequest)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: AI 服务响应超时。", domain.ErrProviderFailed)
		}
		return nil, fmt.Errorf("%w: %v", domain.ErrProviderFailed, err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 400 {
		detail := strings.TrimSpace(string(raw))
		if len(detail) > 300 {
			detail = detail[:300]
		}
		return nil, fmt.Errorf("%w: AI 服务返回 %d：%s", domain.ErrProviderFailed, response.StatusCode, detail)
	}
	var decoded chatResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("%w: 无法解析 AI 回复。", domain.ErrProviderFailed)
	}
	if decoded.Error != nil && decoded.Error.Message != "" {
		return nil, fmt.Errorf("%w: %s", domain.ErrProviderFailed, decoded.Error.Message)
	}
	if len(decoded.Choices) == 0 {
		return nil, fmt.Errorf("%w: AI 没有返回内容。", domain.ErrProviderFailed)
	}
	return &contract.ModelReply{
		Content: strings.TrimSpace(decoded.Choices[0].Message.Content),
		Tokens:  decoded.Usage.TotalTokens,
		Raw:     raw,
	}, nil
}
