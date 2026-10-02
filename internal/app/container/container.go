package container

import (
	"context"
	"errors"
	"log"
	"strings"

	"gorm.io/gorm"

	"github.com/kaoqy/Nodeloc-Store/internal/app/stockwatch"
	"github.com/kaoqy/Nodeloc-Store/internal/authz"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/mail"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/activity"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/catalog"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity"
	identityapplication "github.com/kaoqy/Nodeloc-Store/internal/modules/identity/application"
	identitydomain "github.com/kaoqy/Nodeloc-Store/internal/modules/identity/domain"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/notification"
	notificationapp "github.com/kaoqy/Nodeloc-Store/internal/modules/notification/application"
	notificationcontract "github.com/kaoqy/Nodeloc-Store/internal/modules/notification/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment"
	paymentcontract "github.com/kaoqy/Nodeloc-Store/internal/modules/payment/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support"
	supportapplication "github.com/kaoqy/Nodeloc-Store/internal/modules/support/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/system"
	"github.com/kaoqy/Nodeloc-Store/internal/platform/database/gormdb"
)

// Container holds all application dependencies
type Container struct {
	DB           *gorm.DB
	Config       *config.Config
	Identity     *identity.Module
	Payment      *payment.Module
	Catalog      *catalog.Module
	Notification *notification.Module
	Audit        *audit.Module
	// Activity is the promotion runtime: structured rules, server-side pricing
	// and participation records. Payment prices orders through it.
	Activity *activity.Module
	// Support is the ticket + AI customer service runtime. Tickets default to AI
	// handling, with a controlled tool layer and a human queue behind it.
	Support *support.Module
	// Plugin is the extension runtime. It carries the providers this release
	// ships and the shop's enrollments of them; the money side calls its Fulfill
	// hook for a product a plugin is bound to.
	Plugin *plugin.Module
	// Stock warns the accounts that refill shelves when a published product runs
	// short. The maintenance loop sweeps it and the back office can ask for one
	// pass on demand.
	Stock *stockwatch.Watcher
}

// inbox is the payment module's order events written into the buyer's
// notification list. Publish swallows what it cannot write: the order it reports
// has already moved on, and a buyer who gets no message can still read the same
// truth on the order page.
type inbox struct {
	service *notificationapp.Service
}

func (i inbox) Publish(ctx context.Context, event paymentcontract.BuyerEvent) {
	notification := &models.Notification{UserID: event.UserID, Type: event.Type, Title: event.Title}
	if content := strings.TrimSpace(event.Content); content != "" {
		notification.Content = &content
	}
	if link := strings.TrimSpace(event.Link); link != "" {
		notification.Link = &link
	}
	if err := i.service.Send(ctx, notification); err != nil {
		log.Printf("[inbox] user %d (%s): %v", event.UserID, event.Title, err)
	}
}

// New builds the container from config. Runtime settings stored by the
// in-app wizard are overlaid onto cfg before the modules are wired. sys may
// be nil (tests); when present it is re-attached to the fresh dependencies.
func New(cfg *config.Config, sys *system.Service) (*Container, error) {
	// Database
	db, err := gormdb.New(&cfg.Database)
	if err != nil {
		return nil, err
	}

	// Migrate
	if err := models.Migrate(db); err != nil {
		return nil, err
	}

	// RBAC
	if err := authz.Init(db); err != nil {
		return nil, err
	}
	if err := authz.SeedDefaults(); err != nil {
		log.Printf("[warn] RBAC seed failed: %v", err)
	}
	// Stores built before the wizard handed out super_admin have no owner at all.
	if err := system.EnsureOwnerRole(db); err != nil {
		log.Printf("[warn] owner role check failed: %v", err)
	}

	// Wizard-stored settings win over process defaults.
	rt, err := system.LoadRuntime(db)
	if err != nil {
		return nil, err
	}
	if rt != nil {
		rt.ApplyTo(cfg)
	}

	// Wiring — each module exposes a Wire() function
	identityMod := identity.Wire(db, cfg)

	identityFind := func(ctx context.Context, userID uint) (*models.User, error) {
		user, err := identityMod.Service.Me(ctx, userID)
		if errors.Is(err, identitydomain.ErrUserNotFound) {
			// Payment asks "who is this account" before a 转账 or a refund. "There
			// is no such account" is an answer, not a fault, and payment words its
			// own refusal once it sees nil.
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &models.User{
			Base:          models.Base{ID: user.ID},
			Username:      user.Username,
			IsActive:      user.IsActive,
			IsAdmin:       user.IsAdmin,
			Role:          user.Role,
			OAuthUID:      user.OAuthUID,
			OAuthUsername: user.OAuthUsername,
		}, nil
	}
	// Catalogue and inbox first: checkout asks the catalogue what a 优惠码 is worth
	// and hands the buyer's order events to the inbox, so payment is wired
	// against modules that already exist.
	catalogMod := catalog.Wire(db, cfg)
	notificationMod := notification.Wire(db)
	paymentMod, err := payment.Wire(db, cfg, identityFind, catalogMod.Service, inbox{service: notificationMod.Service})
	if err != nil {
		return nil, err
	}
	activityMod := activity.Wire(db)
	supportMod := support.Wire(db, cfg, support.Dependencies{
		Orders:     support.OrderAdapter{DB: db},
		Users:      support.UserAdapter{Find: func(ctx context.Context, userID uint) (*models.User, error) { return identityFind(ctx, userID) }},
		Catalog:    support.CatalogAdapter{DB: db},
		Activities: support.ActivityAdapter{DB: db},
		Notifier: support.NotifierAdapter{Send: func(ctx context.Context, notification *models.Notification) error {
			return notificationMod.Service.Send(ctx, notification)
		}},
	})
	auditMod := audit.Wire(db)
	pluginMod := plugin.Wire(db)
	// 活动定价挂到支付模块上：下单金额仍由后端计算，活动只贡献结构化规则。
	paymentMod.Service.SetActivityPricing(activityMod.Service)
	// 活动的默认限次与叠加策略来自配置中心。
	activityMod.Handler.SetDefaults(activityDefaults{support: supportMod.Service})
	// The money side delivers a plugin-bound order through the plugin runtime;
	// every other product keeps the shop's own card/manual queue.
	paymentMod.Service.SetPluginDeliverer(plugin.NewDeliveryBridge(pluginMod.Service))

	// Restock warnings are the one thing that closes the loop for the shop
	// itself: the catalogue already knows which shelves are short, the inbox
	// already reaches the accounts that refill them, and only this wire joins
	// the two.
	// 站外提醒: every 站内 notification is mirrored to the buyer's email when the
	// shop has SMTP switched on. The three pieces live in other modules (accounts
	// and settings), so they are adapted here rather than reached for from inside
	// the notification module.
	notificationMod.Service.EnableMail(
		mailAddresses{identity: identityMod.Service},
		mailSettings{sys: sys, db: db},
		mailSender{},
	)

	stockWatch := stockwatch.New(catalogMod.Service, restockOutbox{service: notificationMod.Service}, restockRoster(db))
	// The back office can also ask for one pass instead of waiting for the
	// background sweep, so the catalogue's route needs the same watcher.
	catalogMod.Handler.SetRestockWarner(stockWatch)
	// 前台展示开关读配置中心：销量是否展示、库存预警阈值都由店家决定。
	catalogMod.Handler.SetStorefrontFlags(func(ctx context.Context) map[string]bool {
		return map[string]bool{
			"show_sold_count": supportMod.Service.SystemConfigBool(ctx, "product", "show_sold_count", true),
		}
	})

	// The loop closes in the other direction here: a card import tells the money
	// side that the stock a waiting order was short of has arrived, so the buyer
	// is delivered on the spot instead of on the next background sweep.
	catalogMod.Service.SetDeliveryWake(paymentMod.Service)

	if sys != nil {
		sys.Attach(db,
			// The OAuth probe redeems a code that cannot exist at NodeLoc's token
			// endpoint. The authorize page is no evidence — the live forum answers it
			// with its own login redirect even for a client_id it has never heard of,
			// so building a URL used to hand out 「配置正确」 while every buyer fell off
			// after logging in. The token endpoint checks the client pair first, which
			// is the one thing 设置 needs to know, and it costs nobody a login.
			func(ctx context.Context) system.OAuthProbe {
				outcome := identityMod.Service.ProbeOAuth(ctx)
				link, _, _ := identityMod.Service.InitiateOAuth("settings-test")
				return system.OAuthProbe{Code: outcome.Code, Detail: outcome.Detail, AuthorizeURL: link}
			},
			// The probe replays 下单 for one of this shop's own settled orders, which
			// NodeLoc answers 「order already exists」 — the only server-side payment
			// call that proves the signing credentials without putting a charge on
			// anybody (查单 is a browser-session route on the real forum). The payment
			// module classifies its own answer, so 设置 reads a code instead of
			// re-reading the money module's error strings.
			func(ctx context.Context) system.PaymentProbe {
				outcome := paymentMod.Service.Probe(ctx)
				return system.PaymentProbe{
					Code:      outcome.Code,
					Message:   outcome.Message,
					Detail:    outcome.Detail,
					Retryable: outcome.Retryable,
					Style:     outcome.Style,
				}
			},
		)
	}

	return &Container{
		DB:           db,
		Config:       cfg,
		Identity:     identityMod,
		Payment:      paymentMod,
		Catalog:      catalogMod,
		Notification: notificationMod,
		Audit:        auditMod,
		Activity:     activityMod,
		Plugin:       pluginMod,
		Support:      supportMod,
		Stock:        stockWatch,
	}, nil
}

// activityDefaults adapts the configuration centre to the activity handler's
// default-value port.
type activityDefaults struct {
	support *supportapplication.Service
}

func (a activityDefaults) PerUserLimit(ctx context.Context) int {
	return a.support.SystemConfigInt(ctx, "activity", "default_per_user_limit", 1)
}

func (a activityDefaults) AllowStacking(ctx context.Context) bool {
	blocked := a.support.SystemConfigBool(ctx, "activity", "block_activity_stacking", true)
	return !blocked
}

// mailAddresses resolves the inbox address for one account. It goes through the
// identity module rather than the database so a rule about who may be mailed
// (for example an account that was switched off) has one home.
type mailAddresses struct {
	identity *identityapplication.Service
}

func (a mailAddresses) EmailFor(ctx context.Context, userID uint) (string, error) {
	user, err := a.identity.Me(ctx, userID)
	if err != nil {
		// A message can outlive the account it was written for. That is not a
		// failure of the 站内 notification, so the address simply reads as absent.
		if errors.Is(err, identitydomain.ErrUserNotFound) {
			return "", nil
		}
		return "", err
	}
	if user == nil || user.Email == nil {
		return "", nil
	}
	return strings.TrimSpace(*user.Email), nil
}

// mailSettings adapts the system module's SMTP settings to the notification
// module's own shape. The settings are read on every send so turning SMTP on or
// off in 设置 applies to the next message without a restart.
type mailSettings struct {
	sys *system.Service
	db  *gorm.DB
}

func (m mailSettings) MailConfig(ctx context.Context) (notificationcontract.MailConfig, bool) {
	var (
		config   system.SMTPConfig
		siteName string
		scheme   string
		domain   string
	)
	if m.sys != nil {
		if smtp, err := m.sys.SMTPConfig(); err == nil {
			config = smtp
		}
		// GetSettings answers {"settings": RuntimeConfig, …}: the runtime document
		// is the value under "settings", not the map itself. Its SMTP password is
		// masked for the screen, which is why the credential is read separately
		// through SMTPConfig above — a masked password would fail every login.
		if settings, err := m.sys.GetSettings(); err == nil {
			if runtime, ok := settings["settings"].(system.RuntimeConfig); ok {
				siteName, scheme, domain = runtime.App.Name, runtime.App.Scheme, runtime.App.Domain
			}
		}
	} else if m.db != nil {
		if rt, err := system.LoadRuntime(m.db); err == nil && rt != nil {
			config = rt.SMTP
			siteName, scheme, domain = rt.App.Name, rt.App.Scheme, rt.App.Domain
		}
	}
	if !config.On() || strings.TrimSpace(config.Host) == "" || strings.TrimSpace(config.From) == "" {
		return notificationcontract.MailConfig{}, false
	}
	return notificationcontract.MailConfig{
		Host:     config.Host,
		Port:     config.Port,
		Username: config.Username,
		Password: config.Password,
		Secure:   config.Secure,
		From:     config.From,
		SiteName: strings.TrimSpace(siteName),
		BaseURL:  siteBaseURL(scheme, domain),
	}, true
}

// siteBaseURL is the shop's own origin, used only to make a relative
// notification link clickable in an email. An empty answer is fine: the mail then
// carries the path alone rather than a guessed host.
func siteBaseURL(scheme, domain string) string {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return ""
	}
	if strings.HasPrefix(domain, "http://") || strings.HasPrefix(domain, "https://") {
		return strings.TrimRight(domain, "/")
	}
	scheme = strings.TrimSpace(scheme)
	if scheme != "http" {
		scheme = "https"
	}
	return scheme + "://" + strings.TrimRight(domain, "/")
}

// mailSender is the one place a notification becomes an email. It reuses the
// SMTP client the 设置 page already tests, so what the owner verifies there is
// exactly what the fan-out uses.
type mailSender struct{}

func (mailSender) SendMail(config notificationcontract.MailConfig, to, subject, body string) error {
	return (mail.Config{
		Host:   config.Host,
		Port:   config.Port,
		User:   config.Username,
		Pass:   config.Password,
		Secure: config.Secure,
		From:   config.From,
	}).Send(mail.Message{To: to, Subject: subject, Body: body})
}
