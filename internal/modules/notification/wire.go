package notification

import (
	"github.com/kaoqy/Nodeloc-Store/internal/modules/notification/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/notification/infrastructure"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/notification/transport/http"
	"gorm.io/gorm"
)

// Module holds the notification module's runtime dependencies.
type Module struct {
	Service *application.Service
	Handler *http.Handler
}

// Wire constructs the notification module.
func Wire(db *gorm.DB) *Module {
	repo := infrastructure.NewGormStore(db)
	notifier := infrastructure.NewMemoryNotifier()

	svc := application.NewService(repo, notifier)
	handler := http.NewHandler(svc)
	return &Module{Service: svc, Handler: handler}
}
