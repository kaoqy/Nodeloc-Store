package plugin

import (
	"context"

	"gorm.io/gorm"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/infrastructure"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/transport/http"
)

// Module holds the plugin module's runtime dependencies.
type Module struct {
	Service  *application.Service
	Handler  *http.Handler
	Registry *infrastructure.Registry
}

// Wire constructs the plugin module with the providers this release carries.
//
// Providers are registered here, once, next to the shop's own code: the runtime
// is part of the binary and a plugin is an enrollment of one of these
// capabilities. A new provider is added by implementing contract.Provider and
// naming it in this list — nothing else in the application changes.
func Wire(db *gorm.DB, runtimeConfig contract.RuntimeConfigProvider) *Module {
	registry := infrastructure.NewRegistry(
		infrastructure.NewManualDelivery(),
		infrastructure.NewNewAPIRedemption(runtimeConfig),
	)
	repo := infrastructure.NewGormStore(db)
	service, err := application.NewService(repo, registry, runtimeConfig)
	if err != nil {
		// The only way this fails is a nil dependency, which is a programming
		// error rather than a runtime condition.
		panic(err)
	}
	if err := service.EnsureBuiltinProvider(context.Background(), infrastructure.NewAPIRedemptionKey); err != nil {
		panic(err)
	}
	return &Module{Service: service, Handler: http.NewHandler(service), Registry: registry}
}
