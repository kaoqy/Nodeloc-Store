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
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kaoqy/Nodeloc-Store/internal/app/container"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
)

func main() {
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	ctn, err := container.New(cfg)
	if err != nil {
		log.Fatalf("Failed to build container: %v", err)
	}

	router := gin.Default()
	router.Use(gin.Recovery())

	// Health check
	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Register module routes
	ctn.Identity.Handler.RegisterRoutes(router, &cfg.JWT)
	ctn.Payment.Handler.RegisterRoutes(router, &cfg.JWT)
	ctn.Catalog.Handler.RegisterRoutes(router, &cfg.JWT)
	ctn.Notification.Handler.RegisterRoutes(router, &cfg.JWT)
	ctn.Audit.Handler.RegisterRoutes(router, &cfg.JWT)

	// Static files — user storefront
	setupStatic(router, "/web/user", "/")

	// Static files — admin panel
	setupStatic(router, "/web/admin", "/admin")

	// Start server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	go func() {
		log.Printf("Server starting on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Graceful shutdown
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

// setupStatic serves static files from a directory and provides SPA fallback.
func setupStatic(router *gin.Engine, dir string, basePath string) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		log.Printf("Failed to resolve static dir %s: %v", dir, err)
		return
	}

	// Check if directory exists
	if _, err := os.Stat(absDir); os.IsNotExist(err) {
		log.Printf("Static dir %s does not exist, skipping", absDir)
		return
	}

	// Serve static files
	router.Static(basePath, absDir)

	// SPA fallback — serve index.html for unknown routes
	router.NoRoute(func(c *gin.Context) {
		// Don't interfere with API routes
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		indexPath := filepath.Join(absDir, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			c.File(indexPath)
		} else {
			c.String(http.StatusNotFound, "Frontend not built")
		}
	})

	log.Printf("Static files served from %s at %s", absDir, basePath)
}
