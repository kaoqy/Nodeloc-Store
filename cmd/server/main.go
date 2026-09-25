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
	"github.com/kaoqy/Nodeloc-Store/internal/config"
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
		router := buildFullRouter(ctn, sysSvc, dataDir)

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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server exited")
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
	registerSPA(router, filepath.Dir(dataDir))
	return router
}

func buildFullRouter(ctn *container.Container, sysSvc *system.Service, dataDir string) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.GET("/api/health", healthHandler)

	cfg := ctn.Config
	ctn.Identity.Handler.RegisterRoutes(router, &cfg.JWT)
	ctn.Payment.Handler.RegisterRoutes(router, &cfg.JWT)
	ctn.Catalog.Handler.RegisterRoutes(router, &cfg.JWT)
	ctn.Notification.Handler.RegisterRoutes(router, &cfg.JWT)
	ctn.Audit.Handler.RegisterRoutes(router, &cfg.JWT)
	sysSvc.Handler().RegisterRoutes(router, &cfg.JWT, ctn.Identity.Handler.AuthMiddleware())

	registerSPA(router, filepath.Dir(dataDir))
	return router
}

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// registerSPA serves the built storefront at / and the admin panel at /admin,
// with history-mode fallbacks.
func registerSPA(router *gin.Engine, rootDir string) {
	userDir := resolveDir(rootDir, "web/user")
	adminDir := resolveDir(rootDir, "web/admin")
	if userDir != "" {
		router.Static("/web/user", userDir)
	}
	if adminDir != "" {
		router.Static("/web/admin", adminDir)
		router.Static("/admin", adminDir)
	}
	router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		dir := userDir
		if strings.HasPrefix(path, "/admin") {
			dir = adminDir
		}
		if dir == "" {
			c.String(http.StatusNotFound, "Frontend not built")
			return
		}
		// Serve real static assets directly, fall back to index.html.
		rel := strings.TrimPrefix(strings.TrimPrefix(path, "/admin"), "/")
		if rel == "" {
			rel = "index.html"
		}
		candidate := filepath.Join(dir, filepath.FromSlash(rel))
		if !strings.HasPrefix(candidate, filepath.Clean(dir)+string(os.PathSeparator)) && candidate != filepath.Clean(dir) {
			candidate = ""
		}
		if candidate != "" {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				c.File(candidate)
				return
			}
		}
		index := filepath.Join(dir, "index.html")
		if _, err := os.Stat(index); err == nil {
			c.File(index)
			return
		}
		c.String(http.StatusNotFound, "Frontend not built")
	})
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
