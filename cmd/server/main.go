package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kaoqy/Nodeloc-Store/internal/app/container"
	middleware "github.com/kaoqy/Nodeloc-Store/internal/app/httpserver"
	"github.com/kaoqy/Nodeloc-Store/internal/app/stockwatch"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit/application"
	paymentapp "github.com/kaoqy/Nodeloc-Store/internal/modules/payment/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/system"
)

func main() {
	// Process-level defaults (legacy config.yml is still honoured when a
	// bootstrap file has not been written yet; new installs never need one).
	baseCfg, err := config.Load("")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if baseCfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	dataDir := config.DefaultDataDir()
	bootPath := config.BootstrapPathIn(dataDir)
	boot, found, err := config.ReadBootstrap(bootPath)
	if err != nil {
		log.Fatalf("Failed to read bootstrap config: %v", err)
	}
	if !found && baseCfg.Database.DSN != "" {
		// One-time, automatic migration from a hand-written config.yml.
		boot = config.Bootstrap{
			DBDriver:  baseCfg.Database.Driver,
			DBDSN:     baseCfg.Database.DSN,
			Port:      baseCfg.Server.Port,
			JWTSecret: baseCfg.JWT.Secret,
		}
		if boot.DBDriver == "" {
			boot.DBDriver = "sqlite"
		}
		if err := boot.WriteTo(bootPath); err != nil {
			log.Printf("[warn] could not persist migrated bootstrap: %v", err)
		}
		found = true
	}

	sw := &swapHandler{}
	sysSvc := system.NewService(bootPath, baseCfg.Server.Port)

	var live *container.Container
	var liveMu sync.Mutex

	auditor := audit.NewRecorder(func() *application.Service {
		liveMu.Lock()
		defer liveMu.Unlock()
		if live == nil {
			return nil
		}
		return live.Audit.Service
	})

	rebuild := func() error {
		cfg, err := config.Load("")
		if err != nil {
			return err
		}
		current, _, err := config.ReadBootstrap(bootPath)
		if err != nil {
			return err
		}
		if current.JWTSecret == "" {
			current.JWTSecret = cfg.JWT.Secret
		}
		current.ApplyTo(cfg)
		if cfg.JWT.Secret == "" {
			return fmt.Errorf("缺少 JWT 密钥，请重新初始化")
		}
		ctn, err := container.New(cfg, sysSvc)
		if err != nil {
			return err
		}
		router := buildFullRouter(ctn, sysSvc, dataDir, auditor)

		liveMu.Lock()
		old := live
		live = ctn
		liveMu.Unlock()
		if old != nil {
			closeDB(old)
		}
		sw.set(router)
		log.Printf("Application ready (db=%s, port=%d)", cfg.Database.Driver, cfg.Server.Port)
		return nil
	}
	sysSvc.SetRebuild(rebuild)

	if found {
		if err := rebuild(); err != nil {
			log.Fatalf("Failed to start application: %v", err)
		}
	} else {
		sw.set(buildBootstrapRouter(sysSvc, dataDir))
		log.Printf("No bootstrap config found — serving setup wizard at /admin (data dir: %s)", dataDir)
	}

	addr := fmt.Sprintf(":%d", currentPort(bootPath, baseCfg.Server.Port))
	srv := &http.Server{
		Addr:    addr,
		Handler: sw,
	}

	go func() {
		log.Printf("Server starting on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// One background loop keeps the shop running without a person: it asks
	// NodeLoc about orders a lost browser redirect left at 待支付, retries
	// delivery for orders already paid, and warns whoever refills the shelves
	// that a product is short. It resolves the live container on every pass
	// because saving settings rebuilds it and closes the old database.
	stopMaintenance := make(chan struct{})
	go func() {
		maintenanceLoop(stopMaintenance, func() maintenanceDeps {
			liveMu.Lock()
			defer liveMu.Unlock()
			if live == nil {
				return maintenanceDeps{}
			}
			return maintenanceDeps{payments: live.Payment.Service, stock: live.Stock}
		})
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")
	close(stopMaintenance)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server exited")
}

// maintenanceDeps is what the sweep needs this minute. Both halves are optional:
// a container mid-rebuild answers with nothing, and the loop waits for the next
// pass instead of failing on a service it has not been given.
type maintenanceDeps struct {
	payments *paymentapp.Service
	stock    *stockwatch.Watcher
}

// maintenanceLoop keeps the shop self-healing: 查单 for orders the store still
// calls 待支付 (a lost redirect must not cost a buyer their goods), a delivery
// retry for orders already paid, and one restock warning pass so a short shelf
// reaches the accounts that refill it. The first pass runs shortly after
// start-up so a crash that dropped a delivery is fixed without waiting; after
// that it runs every few minutes, which stays far below any sane rate limit on
// the provider side.
func maintenanceLoop(stop <-chan struct{}, resolve func() maintenanceDeps) {
	sweep := func() {
		deps := resolve()
		if deps.payments == nil && deps.stock == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if deps.payments != nil {
			if _, err := deps.payments.AutoReconcilePending(ctx); err != nil {
				log.Printf("payment maintenance: provider check failed: %v", err)
			}
			if moved, err := deps.payments.RetryPendingDeliveries(ctx); err != nil {
				log.Printf("delivery retry sweep: %v", err)
			} else if moved > 0 {
				log.Printf("delivery retry sweep: delivered %d order(s)", moved)
			}
		}
		if deps.stock != nil {
			result, err := deps.stock.Pass(ctx)
			if err != nil {
				log.Printf("restock sweep: %v", err)
			} else if result.Sent > 0 {
				log.Printf("restock sweep: %d product(s) short, %d warning(s) sent", result.Checked, result.Sent)
			}
		}
	}

	first := time.NewTimer(20 * time.Second)
	defer first.Stop()
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-first.C:
			sweep()
		case <-ticker.C:
			sweep()
		}
	}
}

func currentPort(bootPath string, fallback int) int {
	if boot, found, err := config.ReadBootstrap(bootPath); err == nil && found && boot.Port > 0 {
		return boot.Port
	}
	if fallback > 0 {
		return fallback
	}
	return 8080
}

func closeDB(ctn *container.Container) {
	if sqlDB, err := ctn.DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

// swapHandler allows the whole application router to be replaced at runtime
// after the setup wizard or a settings change.
type swapHandler struct {
	mu sync.RWMutex
	h  http.Handler
}

func (s *swapHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	h := s.h
	s.mu.RUnlock()
	if h == nil {
		http.Error(w, "starting", http.StatusServiceUnavailable)
		return
	}
	h.ServeHTTP(w, r)
}

func (s *swapHandler) set(h http.Handler) {
	s.mu.Lock()
	s.h = h
	s.mu.Unlock()
}

// buildBootstrapRouter serves the SPAs plus only the setup endpoints.
func buildBootstrapRouter(sysSvc *system.Service, dataDir string) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.GET("/api/health", healthHandler)
	sysSvc.Handler().RegisterPublicRoutes(router)
	registerSPA(router, filepath.Dir(dataDir), sysSvc)
	return router
}

func buildFullRouter(ctn *container.Container, sysSvc *system.Service, dataDir string, auditor *audit.Recorder) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.Use(middleware.AdminAudit(auditor))
	router.GET("/api/health", healthHandler)

	cfg := ctn.Config
	accounts := ctn.Identity.Handler.AccountReader()
	ctn.Identity.Handler.RegisterRoutes(router, &cfg.JWT)
	ctn.Payment.Handler.RegisterRoutes(router, &cfg.JWT, accounts)
	ctn.Catalog.Handler.RegisterRoutes(router, &cfg.JWT, accounts)
	ctn.Notification.Handler.RegisterRoutes(router, &cfg.JWT, accounts)
	ctn.Activity.Handler.RegisterRoutes(router, &cfg.JWT, accounts)
	ctn.Support.Handler.RegisterRoutes(router, &cfg.JWT, accounts)
	ctn.Plugin.Handler.RegisterRoutes(router, &cfg.JWT, accounts)
	ctn.Audit.Handler.RegisterRoutes(router, &cfg.JWT, accounts)
	sysSvc.Handler().RegisterRoutes(router, &cfg.JWT, accounts)
	registerUploads(router, filepath.Dir(dataDir), &cfg.JWT, accounts, ctn.Identity.Service)

	registerSPA(router, filepath.Dir(dataDir), sysSvc)
	return router
}

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// registerSPA serves the built storefront at / and the admin panel at /admin,
// with history-mode fallbacks. Every SPA path goes through one handler so the
// HTML can be marked non-cacheable, which it needs twice over: it names hashed
// bundles, so a stale copy would 404 all of them after an upgrade, and it
// carries the shop's own words, so a stale copy would keep describing the shop
// the way it did before the owner renamed it.
func registerSPA(router *gin.Engine, rootDir string, sysSvc *system.Service) {
	userDir := resolveDir(rootDir, "web/user")
	adminDir := resolveDir(rootDir, "web/admin")

	router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "这个接口不存在，页面可能还停留在旧版商店，请刷新后重试。", "code": "not_found"})
			return
		}
		// Product images live outside both bundles: files dropped into ./uploads
		// (a mounted volume) are referenced as /uploads/<name>. A miss must 404
		// rather than fall through to index.html, which would render as a broken
		// image with no clue why.
		if rel, ok := strings.CutPrefix(path, "/uploads/"); ok {
			dir := filepath.Join(rootDir, "uploads")
			candidate := filepath.Join(dir, filepath.FromSlash(rel))
			if strings.HasPrefix(candidate, filepath.Clean(dir)+string(os.PathSeparator)) {
				if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
					c.Header("Cache-Control", "public, max-age=86400")
					c.File(candidate)
					return
				}
			}
			c.Status(http.StatusNotFound)
			return
		}
		dir, rel := spaTarget(path, userDir, adminDir)
		if dir == "" {
			c.String(http.StatusNotFound, "Frontend not built")
			return
		}
		if rel == "" {
			rel = "index.html"
		}
		candidate := filepath.Join(dir, filepath.FromSlash(rel))
		if !strings.HasPrefix(candidate, filepath.Clean(dir)+string(os.PathSeparator)) && candidate != filepath.Clean(dir) {
			candidate = ""
		}
		if candidate != "" {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				if strings.Contains(filepath.ToSlash(candidate), "/assets/") {
					c.Header("Cache-Control", "public, max-age=31536000, immutable")
					c.File(candidate)
					return
				}
				serveDocument(c, candidate, sysSvc, dir == adminDir)
				return
			}
		}
		index := filepath.Join(dir, "index.html")
		if _, err := os.Stat(index); err == nil {
			serveDocument(c, index, sysSvc, dir == adminDir)
			return
		}
		c.String(http.StatusNotFound, "Frontend not built")
	})
}

// serveDocument sends one SPA document with the shop's own head tags written in.
// Anything else below a bundle root — a font, a manifest, an image — goes out as
// it was built, and a document that cannot be read is a miss rather than a
// half-written page.
func serveDocument(c *gin.Context, path string, sysSvc *system.Service, admin bool) {
	if !strings.EqualFold(filepath.Ext(path), ".html") {
		c.Header("Cache-Control", "no-cache")
		c.File(path)
		return
	}
	document, err := os.ReadFile(path)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	stamped := stampShell(string(document), sysSvc.GetShellIdentity(), requestBase(c.Request), admin)
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(stamped))
}

// spaTarget maps a request path to the SPA that owns it and the file below that
// SPA's root. Unknown paths resolve to their SPA's index.html upstream.
func spaTarget(path, userDir, adminDir string) (dir, rel string) {
	for _, prefix := range []string{"/admin", "/web/admin"} {
		if path == strings.TrimSuffix(prefix, "/") || strings.HasPrefix(path, prefix+"/") {
			return adminDir, strings.Trim(strings.TrimPrefix(path, prefix), "/")
		}
	}
	if strings.HasPrefix(path, "/web/user/") {
		return userDir, strings.TrimPrefix(path, "/web/user/")
	}
	return userDir, strings.Trim(path, "/")
}

func resolveDir(root, rel string) string {
	for _, base := range []string{rel, filepath.Join(".", rel)} {
		abs, err := filepath.Abs(filepath.Join(root, base))
		if err != nil {
			continue
		}
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			return abs
		}
	}
	return ""
}
