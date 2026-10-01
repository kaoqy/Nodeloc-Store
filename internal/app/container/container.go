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
	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/catalog"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity"
	identitydomain "github.com/kaoqy/Nodeloc-Store/internal/modules/identity/domain"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/notification"
	notificationapp "github.com/kaoqy/Nodeloc-Store/internal/modules/notification/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment"
	paymentcontract "github.com/kaoqy/Nodeloc-Store/internal/modules/payment/contract"
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
	auditMod := audit.Wire(db)

	// Restock warnings are the one thing that closes the loop for the shop
	// itself: the catalogue already knows which shelves are short, the inbox
	// already reaches the accounts that refill them, and only this wire joins
	// the two.
	stockWatch := stockwatch.New(catalogMod.Service, restockOutbox{service: notificationMod.Service}, restockRoster(db))
	// The back office can also ask for one pass instead of waiting for the
	// background sweep, so the catalogue's route needs the same watcher.
	catalogMod.Handler.SetRestockWarner(stockWatch)

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
		Stock:        stockWatch,
	}, nil
}
