package payment

import (
	"context"

	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/infrastructure"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/transport/http"
	"gorm.io/gorm"
)

// Module holds the payment module's runtime dependencies.
type Module struct {
	Service *application.Service
	Handler *http.Handler
	Gateway contract.PaymentGateway
}

// userLookup adapts identity.Me to the payment contract's UserLookup.
type userLookup struct {
	findByID func(ctx context.Context, id uint) (*models.User, error)
}

func (u *userLookup) FindByID(ctx context.Context, id uint) (*contract.UserInfo, error) {
	user, err := u.findByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, nil
	}
	return &contract.UserInfo{
		ID:            user.ID,
		Username:      user.Username,
		IsActive:      user.IsActive,
		OAuthUID:      derefString(user.OAuthUID),
		OAuthUsername: derefString(user.OAuthUsername),
	}, nil
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// Wire constructs the payment module from shared dependencies.
func Wire(db *gorm.DB, cfg *config.Config, identityFind func(ctx context.Context, userID uint) (*models.User, error), coupons contract.CouponPricing, events contract.BuyerNotifier) (*Module, error) {
	store := infrastructure.NewGormStore(db)
	// payment_orders and transactions are owned by this module, so this module
	// migrates them; models.Migrate only covers the shared entities.
	if err := store.Migrate(context.Background()); err != nil {
		return nil, err
	}
	gateway := infrastructure.NewNodeLocGateway(
		cfg.NodeLoc.BaseURL,
		cfg.NodeLoc.PaymentID,
		cfg.NodeLoc.PaymentToken,
		cfg.NodeLoc.PaymentSecret,
		nil,
	)
	lookup := &userLookup{findByID: identityFind}

	// The store owns card allocation and delivery records, so it also performs
	// fulfillment; keeping both in one transactional implementation avoids the
	// two divergent delivery paths that used to coexist here.
	svc := application.NewService(store, gateway, store, lookup, coupons, events, cfg.NodeLoc.PaymentID)
	handler := http.NewHandler(svc)
	return &Module{Service: svc, Handler: handler, Gateway: gateway}, nil
}
