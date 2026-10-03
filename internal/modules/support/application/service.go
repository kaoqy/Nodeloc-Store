package application

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/domain"
)

// Service 是工单与 AI 客服的用例层。它把三条线放在一起：
//  1. 工单流转（AI 默认接待，可转人工）
//  2. AI 对话与受控工具调用
//  3. 知识库、快捷回复、客服人员等配置
//
// 所有金额、订单归属、用户身份都由提供方校验，服务层只做编排。
type Service struct {
	repo        contract.Repository
	tools       ToolRegistry
	orders      contract.OrderReader
	users       contract.UserReader
	catalog     contract.CatalogReader
	activities  contract.ActivityReader
	notifier    contract.Notifier
	mailer      contract.MailSender
	staff       contract.StaffRecipients
	refunder    contract.Refunder
	cards       contract.CardReader
	fulfillment contract.FulfillmentRetrier
	pricing     contract.ProductPricer
	model       contract.ModelClient
	now         func() time.Time
	encKey      []byte
}

// Deps 是服务层依赖，未提供的能力会自动降级（例如没有配置 SMTP 时不发邮件）。
type Deps struct {
	Repo        contract.Repository
	Tools       ToolRegistry
	Orders      contract.OrderReader
	Users       contract.UserReader
	Catalog     contract.CatalogReader
	Activities  contract.ActivityReader
	Notifier    contract.Notifier
	Mailer      contract.MailSender
	Staff       contract.StaffRecipients
	Refunder    contract.Refunder
	Cards       contract.CardReader
	Fulfillment contract.FulfillmentRetrier
	Pricing     contract.ProductPricer
	Model       contract.ModelClient
	// SecretKey 用来加密 AI API Key；为空时用进程内随机密钥，
	// 重启后需要重新保存一次密钥，这是可接受的安全默认。
	SecretKey string
}

func NewService(deps Deps) (*Service, error) {
	if deps.Repo == nil {
		return nil, errors.New("support service requires a repository")
	}
	service := &Service{
		repo:        deps.Repo,
		tools:       deps.Tools,
		orders:      deps.Orders,
		users:       deps.Users,
		catalog:     deps.Catalog,
		activities:  deps.Activities,
		notifier:    deps.Notifier,
		mailer:      deps.Mailer,
		staff:       deps.Staff,
		refunder:    deps.Refunder,
		cards:       deps.Cards,
		fulfillment: deps.Fulfillment,
		pricing:     deps.Pricing,
		model:       deps.Model,
		now:         time.Now,
	}
	service.encKey = deriveKey(deps.SecretKey)
	if service.tools == nil {
		service.tools = NewRegistry()
	}
	return service, nil
}

// SetModel 注入大模型客户端。放在 setter 里是因为客户端需要服务的解密函数，
// 而解密函数属于服务本身，构造顺序上只能后置。
func (s *Service) SetModel(client contract.ModelClient) {
	if client != nil {
		s.model = client
	}
}

// deriveKey 把任意长度的密钥收敛成 AES-256 需要的 32 字节。
func deriveKey(secret string) []byte {
	if strings.TrimSpace(secret) == "" {
		// 没有配置 JWT 密钥时用随机密钥，保证 API Key 不以明文落库。
		buf := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, buf); err != nil {
			sum := sha256.Sum256([]byte("nodeloc-store-support"))
			return sum[:]
		}
		return buf
	}
	sum := sha256.Sum256([]byte("nodeloc-store:" + secret))
	return sum[:]
}

// EncryptSecret 加密 API Key 之类的凭据，返回 base64 密文。
func (s *Service) EncryptSecret(plain string) (string, error) {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return "", nil
	}
	block, err := aes.NewCipher(s.encKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(append(nonce, sealed...)), nil
}

// DecryptSecret 解开密文。密文损坏或密钥换了都返回错误，调用方据此提示重填。
func (s *Service) DecryptSecret(encoded string) (string, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.encKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("encrypted secret is too short")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// DefaultAIConfig 首次访问时给出一份可用配置，管理员在后台改。
func DefaultAIConfig() domain.AIConfig {
	return domain.AIConfig{
		Provider:        "openai-compatible",
		Model:           "gpt-4o-mini",
		TimeoutMS:       30000,
		MaxContext:      12,
		MaxReplyLen:     2000,
		Temperature:     0.3,
		TopP:            1,
		IsEnabled:       false,
		GuestAllowed:    true,
		GuestDailyLimit: 10,
		UserDailyLimit:  100,
		IPRateLimit:     30,
		MaxMessageLen:   2000,
		AgentName:       "智能客服",
		SystemPrompt:    defaultSystemPrompt,
		Greeting:        "你好，我是本店智能客服。你可以直接描述问题，我会先帮你查订单、看规则，解决不了再转人工。",
		FallbackReply:   "这个问题我暂时没有把握回答。你可以换个说法，或者点「转人工客服」让同事接手。",
		TransferTip:     "正在为你转接人工客服，已经把你的对话一起带过去了。",
		TicketTip:       "已经为你建好工单，客服会在工作时间内跟进。",
		SensitiveTip:    "这类问题涉及账户与资金安全，需要人工同事核实后处理，我先帮你转人工。",
	}
}

func DefaultAIWorkflow() domain.AIWorkflowConfig {
	return domain.AIWorkflowConfig{
		DefaultHandleMinutes:   10,
		MaxFailures:            3,
		TransferAfterFailures:  2,
		TransferAfterDownvotes: 2,
		TransferOnExplicit:     true,
		TransferHighAmount:     true,
		HighAmountThreshold:    500,
		TransferRefund:         true,
		TransferCardDispute:    true,
		TransferPaymentIssue:   true,
		TransferAbuse:          true,
		CanCreateTicket:        true,
		CanUpdateTicket:        false,
		CanNotify:              false,
		CanQueryOrder:          true,
		CanQueryShipping:       true,
		CanRecommendActivity:   true,
		CanGrantCoupon:         false,
		CanRefund:              true,
		RequireHumanRefund:     false,
		TransferNotice:         "已转入人工队列，客服会按顺序跟进。",
		WorkingHours:           "每天 09:00 - 21:00",
		EstimateReplyMinutes:   30,
	}
}

const defaultSystemPrompt = `你是本店的在线客服。
规则：
1. 只依据知识库内容和工具返回的数据回答，不要编造订单、库存、价格或政策。
2. 涉及金额、订单归属、退款、卡密争议时，先查询该用户自己的数据；查不到就说明查不到。
3. 绝不输出完整卡密、密钥、他人订单或任何越权数据；需要时只给脱敏摘要。
4. 用户明确要求人工、情绪激动、或问题涉及退款/换货/账号封禁时，主动建议转人工。
5. 回答简洁、分点，先给结论再给操作步骤。`

// logf 统一日志前缀，方便在容器日志里定位 AI 与工单的问题。
func logf(format string, args ...any) {
	log.Printf("[support] "+format, args...)
}

// jsonString 把任意值序列化成审计用的文本，过长时截断。
func jsonString(value any, limit int) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	text := string(encoded)
	if limit > 0 && len(text) > limit {
		text = text[:limit] + "…(truncated)"
	}
	return text
}

// redact 把常见敏感内容从审计文本里去掉。
func redact(value string, limit int) string {
	text := strings.TrimSpace(value)
	for _, prefix := range []string{"sk-", "tk_", "pay_", "Bearer "} {
		if index := strings.Index(text, prefix); index >= 0 {
			text = text[:index+len(prefix)] + "***"
		}
	}
	if limit > 0 && len(text) > limit {
		text = text[:limit] + "…(truncated)"
	}
	return text
}
