package activity

import (
	"gorm.io/gorm"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/activity/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/activity/infrastructure"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/activity/transport/http"
)

// Module 是活动模块的运行时依赖。
type Module struct {
	Service *application.Service
	Handler *http.Handler
}

// Wire 构造活动模块。
func Wire(db *gorm.DB) *Module {
	repo := infrastructure.NewGormStore(db)
	service, err := application.NewService(repo)
	if err != nil {
		panic(err)
	}
	return &Module{Service: service, Handler: http.NewHandler(service)}
}
