package support

import (
	"context"
	"errors"
	"log"
	"strings"

	"gorm.io/gorm"

	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/mail"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/infrastructure"
	supporthttp "github.com/kaoqy/Nodeloc-Store/internal/modules/support/transport/http"
)

// Module 是工单与客服模块的运行时依赖。
type Module struct {
	Service *application.Service
	Handler *supporthttp.Handler
	Store   *infrastructure.GormStore
}

// Dependencies 是容器注入给本模块的其它模块能力。
//
// 订单 / 用户 / 商品 / 活动等只读适配器原先只服务于 AI 工具调用，
// 随 AI 客服一并下线；工单现在只依赖通知与邮件两条出口。
type Dependencies struct {
	Notifier contract.Notifier
	Mailer   contract.MailSender
	// Staff 用于把提醒事件投递给管理端员工（站内 + 邮箱）。
	Staff contract.StaffRecipients
}

// Wire 构造模块。
func Wire(db *gorm.DB, cfg *config.Config, deps Dependencies) *Module {
	_ = cfg // 保留签名：AI 相关配置读取已下线，容器仍按原样注入。
	store := infrastructure.NewGormStore(db)
	service, err := application.NewService(application.Deps{
		Repo:     store,
		Notifier: deps.Notifier,
		Mailer:   deps.Mailer,
		Staff:    deps.Staff,
	})
	if err != nil {
		panic(err)
	}
	return &Module{Service: service, Handler: supporthttp.NewHandler(service), Store: store}
}

// ── 通知适配器 ───────────────────────────────────────────────────────

// NotifierAdapter 把站内通知写进买家的收件箱，可选再发一封邮件。
type NotifierAdapter struct {
	Send func(ctx context.Context, notification *models.Notification) error
	Mail func(ctx context.Context, userID uint, subject, body string) error
}

func (a NotifierAdapter) NotifyUser(ctx context.Context, userID uint, notificationType, title, content, link string) error {
	if a.Send == nil || userID == 0 {
		return errNotifierUnavailable
	}
	notification := &models.Notification{UserID: userID, Type: notificationType, Title: title}
	if strings.TrimSpace(content) != "" {
		value := content
		notification.Content = &value
	}
	if strings.TrimSpace(link) != "" {
		value := link
		notification.Link = &value
	}
	if err := a.Send(ctx, notification); err != nil {
		return err
	}
	if a.Mail != nil {
		if err := a.Mail(ctx, userID, title, content); err != nil {
			supportlog("notification mail failed: %v", err)
		}
	}
	return nil
}

// MailAdapter 使用店铺的 SMTP 配置发信；未开启时静默跳过。
type MailAdapter struct {
	Config MailConfigReader
	Lookup MailAddressReader
}

// MailConfigReader 返回当前 SMTP 配置与是否开启。
type MailConfigReader interface {
	SMTPConfig() (MailSettings, error)
}

// MailSettings 是 mail.Send 需要的连接参数。
type MailSettings struct {
	Host, Username, Password, Secure, From, SiteName string
	Port                                             int
	On                                               bool
}

// MailAddressReader 读取用户的收件地址。
type MailAddressReader interface {
	EmailFor(ctx context.Context, userID uint) (string, error)
}

// settings 读一次 SMTP 配置；未开启或未配置时返回 false。
func (a MailAdapter) settings() (mail.Config, bool, error) {
	if a.Config == nil {
		return mail.Config{}, false, nil
	}
	settings, err := a.Config.SMTPConfig()
	if err != nil {
		return mail.Config{}, false, err
	}
	if !settings.On {
		return mail.Config{}, false, nil
	}
	return mail.Config{
		Host: settings.Host, Port: settings.Port, User: settings.Username,
		Pass: settings.Password, Secure: settings.Secure, From: settings.From,
	}, true, nil
}

func (a MailAdapter) SendToUser(ctx context.Context, userID uint, subject, body string) error {
	if a.Lookup == nil {
		return nil
	}
	settings, on, err := a.settings()
	if err != nil || !on {
		return err
	}
	address, err := a.Lookup.EmailFor(ctx, userID)
	if err != nil || strings.TrimSpace(address) == "" {
		return err
	}
	return settings.Send(mail.Message{To: address, Subject: subject, Body: body})
}

func (a MailAdapter) SendToAddress(ctx context.Context, address, subject, body string) error {
	if strings.TrimSpace(address) == "" {
		return nil
	}
	settings, on, err := a.settings()
	if err != nil || !on {
		return err
	}
	return settings.Send(mail.Message{To: strings.TrimSpace(address), Subject: subject, Body: body})
}

// errNotifierUnavailable 表示容器没有注入站内通知出口，提醒事件只能落库。
var errNotifierUnavailable = errors.New("support: notifier is not configured")

// supportlog 与 application 层保持同样的日志前缀。
func supportlog(format string, args ...any) {
	log.Printf("[support] "+format, args...)
}
