package container

import (
	"context"
	"log"

	"gorm.io/gorm"

	"github.com/kaoqy/Nodeloc-Store/internal/authz"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/catalog"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/notification"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment"
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
}

// New builds the container from config
func New(cfg *config.Config) (*Container, error) {
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

	// Wiring — each module exposes a Wire() function
	identityMod := identity.Wire(db, cfg)

	identityFind := func(ctx context.Context, userID uint) (*models.User, error) {
		user, err := identityMod.Service.Me(ctx, userID)
		if err != nil {
			return nil, err
		}
		return &models.User{
			Base:     models.Base{ID: user.ID},
			Username: user.Username,
			IsActive: user.IsActive,
			IsAdmin:  user.IsAdmin,
			Role:     user.Role,
		}, nil
	}
	paymentMod := payment.Wire(db, cfg, identityFind)
	catalogMod := catalog.Wire(db)
	notificationMod := notification.Wire(db)
	auditMod := audit.Wire(db)

	return &Container{
		DB:           db,
		Config:       cfg,
		Identity:     identityMod,
		Payment:      paymentMod,
		Catalog:      catalogMod,
		Notification: notificationMod,
		Audit:        auditMod,
	}, nil
}
