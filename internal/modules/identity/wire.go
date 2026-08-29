package identity

import (
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/infrastructure"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/transport/http"
	"gorm.io/gorm"
)

// Module holds the identity module's runtime dependencies.
type Module struct {
	Service *application.Service
	Handler *http.Handler
}

// Wire constructs the identity module from shared dependencies.
func Wire(db *gorm.DB, cfg *config.Config) *Module {
	repo := infrastructure.NewGormUserRepo(db)

	oauth, err := infrastructure.NewNodeLocOAuth(infrastructure.NodeLocOAuthConfig{
		BaseURL:      cfg.NodeLoc.BaseURL,
		ClientID:     cfg.NodeLoc.ClientID,
		ClientSecret: cfg.NodeLoc.ClientSecret,
		RedirectURI:  cfg.NodeLoc.RedirectURI,
	}, nil)
	if err != nil {
		panic("identity: failed to create OAuth client: " + err.Error())
	}

	tokens, err := infrastructure.NewJWTService(infrastructure.JWTConfig{
		Secret:     cfg.JWT.Secret,
		Issuer:     "nodeloc-store",
		AccessTTL:  time.Duration(cfg.JWT.AccessTTL) * time.Second,
		RefreshTTL: time.Duration(cfg.JWT.RefreshTTL) * time.Second,
	})
	if err != nil {
		panic("identity: failed to create JWT service: " + err.Error())
	}

	svc, err := application.NewService(repo, oauth, tokens)
	if err != nil {
		panic("identity: failed to create service: " + err.Error())
	}

	handler := http.NewHandler(svc, cfg.App.Scheme == "https")
	return &Module{Service: svc, Handler: handler}
}
