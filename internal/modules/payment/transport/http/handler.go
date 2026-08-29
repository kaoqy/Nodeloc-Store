package http

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
	middleware "github.com/kaoqy/Nodeloc-Store/internal/app/httpserver"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/application"
)

type Handler struct {
	service *application.Service
}

type createPaymentRequest struct {
	OrderNo     string `json:"order_no" binding:"required"`
	Description string `json:"description"`
}

func NewHandler(service *application.Service) *Handler {
	if service == nil {
		panic("payment: nil service")
	}
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router gin.IRouter, jwtConfig *config.JWTConfig) {
	payment := router.Group("/api/v1/payment")
	payment.Use(middleware.JWTMiddleware(jwtConfig))
	payment.POST("/create", h.CreatePayment)
	payment.GET("/return", h.Return)
	payment.GET("/orders/:order_no", h.GetOrder)
	payment.GET("/orders", h.ListOrders)

	// Callback is CSRF-exempt and uses HMAC signature verification instead
	router.POST("/api/v1/payment/callback", h.Callback)
}

func (h *Handler) CreatePayment(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var request createPaymentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	result, err := h.service.CreatePayment(c.Request.Context(), application.CreatePaymentInput{
		UserID:      userID,
		OrderNo:     strings.TrimSpace(request.OrderNo),
		Description: strings.TrimSpace(request.Description),
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (h *Handler) Callback(c *gin.Context) {
	params, err := requestParams(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid callback payload"})
		return
	}

	result, err := h.service.HandleCallback(c.Request.Context(), params)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *Handler) Return(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	orderNo := strings.TrimSpace(c.Query("order_id"))
	if orderNo == "" {
		orderNo = strings.TrimSpace(c.Query("order_no"))
	}
	if orderNo == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "order_id is required"})
		return
	}

	order, err := h.service.GetOrder(c.Request.Context(), userID, orderNo)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"order": order})
}

func (h *Handler) GetOrder(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	orderNo := strings.TrimSpace(c.Param("order_no"))
	if orderNo == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "order_no is required"})
		return
	}

	order, err := h.service.GetOrder(c.Request.Context(), userID, orderNo)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"order": order})
}

func (h *Handler) ListOrders(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	limit, err := parseNonNegativeInt(c.DefaultQuery("limit", "20"))
	if err != nil || limit == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be a positive integer"})
		return
	}
	if limit > 100 {
		limit = 100
	}
	offset, err := parseNonNegativeInt(c.DefaultQuery("offset", "0"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "offset must be a non-negative integer"})
		return
	}

	result, err := h.service.ListOrders(c.Request.Context(), userID, limit, offset)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func requestParams(c *gin.Context) (map[string]string, error) {
	params := make(map[string]string)
	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	if strings.Contains(contentType, "application/json") {
		var payload map[string]any
		if err := c.ShouldBindJSON(&payload); err != nil {
			return nil, err
		}
		for key, value := range payload {
			switch typed := value.(type) {
			case string:
				params[key] = typed
			case float64:
				params[key] = strconv.FormatFloat(typed, 'f', -1, 64)
			case bool:
				params[key] = strconv.FormatBool(typed)
			}
		}
		return params, nil
	}

	if err := c.Request.ParseForm(); err != nil {
		return nil, err
	}
	for key, values := range c.Request.Form {
		if len(values) != 0 {
			params[key] = values[0]
		}
	}
	return params, nil
}

func currentUserID(c *gin.Context) (uint, bool) {
	value, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	switch id := value.(type) {
	case uint:
		return id, id != 0
	case uint64:
		return uint(id), id != 0
	case int:
		return uint(id), id > 0
	case int64:
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

func parseNonNegativeInt(value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, errors.New("invalid integer")
	}
	return parsed, nil
}

func writeError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, application.ErrInvalidInput),
		errors.Is(err, application.ErrAmountMismatch),
		errors.Is(err, application.ErrPaymentNotComplete):
		status = http.StatusBadRequest
	case errors.Is(err, application.ErrInvalidCallback):
		status = http.StatusUnauthorized
	case errors.Is(err, application.ErrForbidden):
		status = http.StatusForbidden
	case strings.Contains(strings.ToLower(err.Error()), "not found"):
		status = http.StatusNotFound
	}
	c.JSON(status, gin.H{"error": err.Error()})
}
