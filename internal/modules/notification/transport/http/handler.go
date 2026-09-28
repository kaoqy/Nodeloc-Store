package http

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	middleware "github.com/kaoqy/Nodeloc-Store/internal/app/httpserver"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/notification/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/notification/infrastructure"
)

type Handler struct{ service *application.Service }

func NewHandler(service *application.Service) *Handler { return &Handler{service: service} }

func (h *Handler) RegisterRoutes(engine gin.IRouter, jwtConfig *config.JWTConfig, accounts middleware.AccountReader) {
	// Send takes an explicit recipient, so it must stay staff-only: any
	// authenticated buyer could otherwise inject messages into someone's inbox.
	guard := middleware.RequirePermission(accounts, "notifications", "manage")
	api := engine.Group("/api/v1")
	api.Use(middleware.JWTMiddleware(jwtConfig))
	api.GET("/notifications", h.List)
	api.POST("/notifications", guard, h.Send)
	// The two inbox-wide routes are registered before /notifications/:id/read so
	// the address bar of a buyer's inbox has a count and a one-click clear.
	api.GET("/notifications/unread", h.UnreadCount)
	api.POST("/notifications/read-all", h.MarkAllRead)
	api.POST("/notifications/:id/read", h.MarkAsRead)
	api.POST("/admin/notifications/broadcast", guard, h.Broadcast)
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
		unauthorized(c)
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	items, total, err := h.service.List(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) Send(c *gin.Context) {
	if _, ok := currentUserID(c); !ok {
		unauthorized(c)
		return
	}
	var request sendRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		invalidInput(c)
		return
	}
	notification := &models.Notification{UserID: request.UserID, Type: request.Type, Title: request.Title, Content: request.Content, Link: request.Link}
	if err := h.service.Send(c.Request.Context(), notification); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, notification)
}

func (h *Handler) MarkAsRead(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		unauthorized(c)
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "通知编号不正确", "code": "invalid_input"})
		return
	}
	if err := h.service.MarkAsRead(c.Request.Context(), uint(id), userID); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "is_read": true})
}

// UnreadCount answers the header badge: how many messages this buyer has not
// opened yet.
func (h *Handler) UnreadCount(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		unauthorized(c)
		return
	}
	unread, err := h.service.CountUnread(c.Request.Context(), userID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"unread": unread})
}

// MarkAllRead clears the whole inbox with one click.
func (h *Handler) MarkAllRead(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		unauthorized(c)
		return
	}
	updated, err := h.service.MarkAllRead(c.Request.Context(), userID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"unread": 0, "marked": updated})
}

func (h *Handler) Broadcast(c *gin.Context) {
	if _, ok := currentUserID(c); !ok {
		unauthorized(c)
		return
	}
	var request broadcastRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		invalidInput(c)
		return
	}
	count, err := h.service.Broadcast(c.Request.Context(), request.Type, request.Title, request.Content, request.Link)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"sent": count})
}

// The domain's own error texts are log material; what reaches a 通知 panel has
// to read like Chinese, not like a stack trace.
func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, application.ErrInvalidNotification):
		invalidInput(c)
	case errors.Is(err, infrastructure.ErrNotificationNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "通知不存在或已被删除", "code": "not_found"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "操作失败，请稍后再试", "code": "internal_error"})
	}
}

func invalidInput(c *gin.Context) {
	c.JSON(http.StatusBadRequest, gin.H{"error": "信息填得不对，请检查后重试", "code": "invalid_input"})
}

func unauthorized(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, gin.H{"error": "登录状态已失效，请重新登录", "code": "invalid_credentials"})
}
