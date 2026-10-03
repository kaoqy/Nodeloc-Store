package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/contract"
)

// Service 是工单与客服的用例层：工单流转、客服人员、快捷回复、
// 通知模板与统一配置中心都在这里编排。
//
// AI 客服相关能力（会话、工具调用、知识库、模型客户端）已下线，
// 相关的表结构保留一个版本以便回滚，代码不再读写。
type Service struct {
	repo     contract.Repository
	notifier contract.Notifier
	mailer   contract.MailSender
	staff    contract.StaffRecipients
	now      func() time.Time
}

// Deps 是服务层依赖，未提供的能力会自动降级（例如没有配置 SMTP 时不发邮件）。
type Deps struct {
	Repo     contract.Repository
	Notifier contract.Notifier
	Mailer   contract.MailSender
	Staff    contract.StaffRecipients
}

func NewService(deps Deps) (*Service, error) {
	if deps.Repo == nil {
		return nil, errors.New("support service requires a repository")
	}
	return &Service{
		repo:     deps.Repo,
		notifier: deps.Notifier,
		mailer:   deps.Mailer,
		staff:    deps.Staff,
		now:      time.Now,
	}, nil
}

// logf 统一日志前缀，方便在容器日志里定位工单的问题。
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
