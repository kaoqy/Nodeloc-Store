package routertest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kaoqy/Nodeloc-Store/internal/app/container"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
)

// TestEveryRouteRegisters builds the whole application against a throwaway
// SQLite database and registers every module's routes onto one router.
//
// Gin panics at registration time when two paths conflict — a static segment and
// a wildcard at the same position, or two different wildcard names — and that
// panic happens inside a JWT-protected group's middleware put at group level. A
// compile is not enough to catch it, and the container is built on every settings
// save, so a conflicting route would take the storefront down at runtime rather
// than at build time. Running the boot path here turns that into a test failure.
func TestEveryRouteRegisters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("create data dir: %v", err)
	}
	// config.DefaultDataDir reads the working directory, so point the process at
	// a temporary tree for the duration of the test.
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	cfg := &config.Config{}
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(dir, "store.db")
	cfg.JWT.Secret = "router-registration-test-secret"

	ctn, err := container.New(cfg, nil)
	if err != nil {
		t.Fatalf("container.New: %v", err)
	}
	defer func() {
		if sqlDB, err := ctn.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()

	router := gin.New()
	accounts := ctn.Identity.Handler.AccountReader()
	ctn.Identity.Handler.RegisterRoutes(router, &cfg.JWT)
	ctn.Payment.Handler.RegisterRoutes(router, &cfg.JWT, accounts)
	ctn.Catalog.Handler.RegisterRoutes(router, &cfg.JWT, accounts)
	ctn.Notification.Handler.RegisterRoutes(router, &cfg.JWT, accounts)
	ctn.Audit.Handler.RegisterRoutes(router, &cfg.JWT, accounts)
	ctn.Plugin.Handler.RegisterRoutes(router, &cfg.JWT, accounts)

	if routes := router.Routes(); len(routes) == 0 {
		t.Fatal("no routes were registered")
	}
}
