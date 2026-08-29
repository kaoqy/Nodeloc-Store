package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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

	container, err := container.New(cfg)
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
	container.Identity.Handler.RegisterRoutes(router, &cfg.JWT)
	container.Payment.Handler.RegisterRoutes(router)
	container.Catalog.Handler.RegisterRoutes(router, &cfg.JWT)
	container.Notification.Handler.RegisterRoutes(router)
	container.Audit.Handler.RegisterRoutes(router)

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
