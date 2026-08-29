package http

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/domain"
)

const (
	claimsKey        = "identity_claims"
	oauthStateCookie = "nodeloc_oauth_state"
)

type Handler struct {
	service      *application.Service
	secureCookie bool
}

func NewHandler(service *application.Service, secureCookie bool) *Handler {
	return &Handler{service: service, secureCookie: secureCookie}
}

func (h *Handler) RegisterRoutes(router gin.IRouter, jwtConfig *config.JWTConfig) {
	auth := router.Group("/api/v1/auth")
	auth.POST("/register", h.Register)
	auth.POST("/login", h.Login)
	auth.POST("/logout", h.Logout)
	auth.GET("/oauth/initiate", h.InitiateOAuth)
	auth.GET("/oauth/callback", h.OAuthCallback)
	auth.POST("/bind-oauth", h.AuthMiddleware(), h.BindOAuth)
	auth.DELETE("/unbind-oauth", h.AuthMiddleware(), h.UnbindOAuth)
	auth.GET("/me", h.AuthMiddleware(), h.Me)
}

func (h *Handler) Register(c *gin.Context) {
	var request struct {
		Username string  `json:"username" binding:"required"`
		Email    *string `json:"email"`
		Password string  `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, domain.ErrInvalidInput)
		return
	}
	result, err := h.service.Register(c.Request.Context(), application.RegisterInput{
		Username: request.Username,
		Email:    request.Email,
		Password: request.Password,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (h *Handler) Login(c *gin.Context) {
	var request struct {
		Identifier string `json:"identifier" binding:"required"`
		Password   string  `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, domain.ErrInvalidCredentials)
		return
	}
	result, err := h.service.Login(c.Request.Context(), application.LoginInput{
		Identifier: request.Identifier,
		Password:   request.Password,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Logout(c *gin.Context) {
	c.SetCookie(oauthStateCookie, "", -1, "/", "", h.secureCookie, true)
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

func (h *Handler) InitiateOAuth(c *gin.Context) {
	redirectURL, state, err := h.service.InitiateOAuth("")
	if err != nil {
		writeError(c, err)
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(oauthStateCookie, state, int((10*time.Minute).Seconds()), "/api/v1/auth", "", h.secureCookie, true)
	if c.Query("redirect") == "true" {
		c.Redirect(http.StatusFound, redirectURL)
		return
	}
	c.JSON(http.StatusOK, gin.H{"authorization_url": redirectURL, "state": state})
}

func (h *Handler) OAuthCallback(c *gin.Context) {
	stateCookie, err := c.Cookie(oauthStateCookie)
	if err != nil || stateCookie == "" || c.Query("state") == "" || stateCookie != c.Query("state") {
		writeError(c, domain.ErrInvalidCredentials)
		return
	}
	params := queryParams(c)
	result, err := h.service.OAuthLogin(c.Request.Context(), c.Query("code"), params)
	if err != nil {
		writeError(c, err)
		return
	}
	c.SetCookie(oauthStateCookie, "", -1, "/api/v1/auth", "", h.secureCookie, true)
	c.JSON(http.StatusOK, result)
}

// AuthMiddleware validates JWT and sets identity claims in context.
func (h *Handler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		if len(header) < 8 || !strings.EqualFold(header[:7], "Bearer ") {
			writeError(c, domain.ErrInvalidCredentials)
			c.Abort()
			return
		}
		claims, err := h.service.Authenticate(c.Request.Context(), strings.TrimSpace(header[7:]))
		if err != nil {
			writeError(c, domain.ErrInvalidCredentials)
			c.Abort()
			return
		}
		c.Set(claimsKey, claims)
		c.Set("user_id", claims.UserID)
		c.Next()
	}
}

func (h *Handler) BindOAuth(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		writeError(c, domain.ErrInvalidCredentials)
		return
	}
	var request struct {
		Code   string            `json:"code" binding:"required"`
		Params map[string]string `json:"params" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, domain.ErrInvalidInput)
		return
	}
	user, err := h.service.BindOAuth(c.Request.Context(), claims.UserID, request.Code, request.Params)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (h *Handler) UnbindOAuth(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		writeError(c, domain.ErrInvalidCredentials)
		return
	}
	user, err := h.service.UnbindOAuth(c.Request.Context(), claims.UserID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (h *Handler) Me(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		writeError(c, domain.ErrInvalidCredentials)
		return
	}
	user, err := h.service.Me(c.Request.Context(), claims.UserID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func claimsFromContext(c *gin.Context) (*domain.TokenClaims, bool) {
	value, exists := c.Get(claimsKey)
	if !exists {
		return nil, false
	}
	claims, ok := value.(*domain.TokenClaims)
	return claims, ok && claims != nil && claims.UserID != 0
}

func queryParams(c *gin.Context) map[string]string {
	params := make(map[string]string, len(c.Request.URL.Query()))
	for key, values := range c.Request.URL.Query() {
		if len(values) > 0 {
			params[key] = values[0]
		}
	}
	return params
}

// currentUserID extracts the authenticated user ID from context.
func currentUserID(c *gin.Context) (uint, bool) {
	v, ok := c.Get("user_id")
	if !ok {
		return 0, false
	}
	switch id := v.(type) {
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

func writeError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	message := "internal server error"
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		status, message = http.StatusBadRequest, err.Error()
	case errors.Is(err, domain.ErrInvalidCredentials):
		status, message = http.StatusUnauthorized, err.Error()
	case errors.Is(err, domain.ErrInactiveUser):
		status, message = http.StatusForbidden, err.Error()
	case errors.Is(err, domain.ErrUserNotFound), errors.Is(err, domain.ErrIdentityNotFound):
		status, message = http.StatusNotFound, err.Error()
	case errors.Is(err, domain.ErrUsernameTaken), errors.Is(err, domain.ErrEmailTaken), errors.Is(err, domain.ErrIdentityAlreadyBound), errors.Is(err, domain.ErrLastLoginMethod):
		status, message = http.StatusConflict, err.Error()
	}
	c.JSON(status, gin.H{"error": message})
}
