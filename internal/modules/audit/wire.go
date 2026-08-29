package audit

import (
	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit/infrastructure"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit/transport/http"
	"gorm.io/gorm"
)

// Module holds the audit module's runtime dependencies.
type Module struct {
	Service *application.Service
	Handler *http.Handler
}

// Wire constructs the audit module.
func Wire(db *gorm.DB) *Module {
	repo := infrastructure.NewGormStore(db)
	svc := application.NewService(repo)
	handler := http.NewHandler(svc)
	return &Module{Service: svc, Handler: handler}
}
