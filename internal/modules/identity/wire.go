package identity

import (
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
	repo := infrastructure.NewGORMUserRepo(db)
	oauth := infrastructure.NewNodeLocOAuthClient(&cfg.NodeLoc)
	tokens := infrastructure.NewJWTService(&cfg.JWT)

	svc, err := application.NewService(repo, oauth, tokens)
	if err != nil {
		panic("identity: failed to create service: " + err.Error())
	}

	handler := http.NewHandler(svc, cfg.App.Scheme == "https")
	return &Module{Service: svc, Handler: handler}
}
