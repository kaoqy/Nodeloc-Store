package http

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
	middleware "github.com/kaoqy/Nodeloc-Store/internal/app/httpserver"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/notification/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/notification/infrastructure"
)

type Handler struct{ service *application.Service }

func NewHandler(service *application.Service) *Handler { return &Handler{service: service} }

func (h *Handler) RegisterRoutes(engine *gin.Engine, jwtConfig *config.JWTConfig) {
	api := engine.Group("/api/v1")
	api.Use(middleware.JWTMiddleware(jwtConfig))
	api.GET("/notifications", h.List)
	api.POST("/notifications", h.Send)
	api.POST("/notifications/:id/read", h.MarkAsRead)
	api.POST("/admin/notifications/broadcast", middleware.RequireAdmin(), h.Broadcast)
}

type sendRequest struct {
	UserID  uint    `json:"user_id" binding:"required"`
	Type    string  `json:"type" binding:"required"`
	Title   string  `json:"title" binding:"required"`
	Content *string `json:"content"`
	Link    *string `json:"link"`
}

type broadcastRequest struct {
	Type    string  `json:"type" binding:"required"`
	Title   string  `json:"title" binding:"required"`
	Content *string `json:"content"`
	Link    *string `json:"link"`
}

func currentUserID(c *gin.Context) (uint, bool) {
	value, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	switch id := value.(type) {
	case uint:
		return id, id != 0
	case int:
		return uint(id), id > 0
	case float64:
		return uint(id), id > 0
	case string:
		parsed, err := strconv.ParseUint(id, 10, 64)
		return uint(parsed), err == nil && parsed != 0
	default:
		return 0, false
	}
}

func (h *Handler) List(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	items, total, err := h.service.List(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) Send(c *gin.Context) {
	if _, ok := currentUserID(c); !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	var request sendRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	notification := &models.Notification{UserID: request.UserID, Type: request.Type, Title: request.Title, Content: request.Content, Link: request.Link}
	if err := h.service.Send(c.Request.Context(), notification); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, application.ErrInvalidNotification) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, notification)
}

func (h *Handler) MarkAsRead(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid notification id"})
		return
	}
	if err := h.service.MarkAsRead(c.Request.Context(), uint(id), userID); err != nil {
		if errors.Is(err, infrastructure.ErrNotificationNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "is_read": true})
}

func (h *Handler) Broadcast(c *gin.Context) {
	if _, ok := currentUserID(c); !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	var request broadcastRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	count, err := h.service.Broadcast(c.Request.Context(), request.Type, request.Title, request.Content, request.Link)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, application.ErrInvalidNotification) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"sent": count})
}
