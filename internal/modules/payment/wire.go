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
		ID:       user.ID,
		Username: user.Username,
		IsActive: user.IsActive,
	}, nil
}

// Wire constructs the payment module from shared dependencies.
func Wire(db *gorm.DB, cfg *config.Config, identityFind func(ctx context.Context, userID uint) (*models.User, error)) *Module {
	store := infrastructure.NewGormStore(db)
	gateway := infrastructure.NewNodeLocGateway(
		cfg.NodeLoc.BaseURL,
		cfg.NodeLoc.PaymentID,
		cfg.NodeLoc.PaymentSecret,
		nil,
	)
	fulfillment := infrastructure.NewFulfillmentService(db)

	lookup := &userLookup{findByID: identityFind}

	svc := application.NewService(store, gateway, fulfillment, lookup, cfg.NodeLoc.PaymentID)
	handler := http.NewHandler(svc)
	return &Module{Service: svc, Handler: handler}
}
