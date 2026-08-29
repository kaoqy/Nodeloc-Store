package catalog

import (
	"github.com/kaoqy/Nodeloc-Store/internal/modules/catalog/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/catalog/infrastructure"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/catalog/transport/http"
	"gorm.io/gorm"
)

// Module holds the catalog module's runtime dependencies.
type Module struct {
	Service *application.Service
	Handler *http.Handler
}

// Wire constructs the catalog module.
func Wire(db *gorm.DB) *Module {
	products := infrastructure.NewProductRepo(db)
	cards := infrastructure.NewCardRepo(db)
	categories := infrastructure.NewCategoryRepo(db)
	coupons := infrastructure.NewCouponRepo(db)

	svc := application.NewService(products, cards, categories, coupons)
	handler := http.NewHandler(svc)
	return &Module{Service: svc, Handler: handler}
}
