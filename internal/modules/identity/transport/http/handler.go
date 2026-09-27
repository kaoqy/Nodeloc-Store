package http

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	middleware "github.com/kaoqy/Nodeloc-Store/internal/app/httpserver"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/domain"
)

const (
	claimsKey        = "identity_claims"
	oauthStateCookie = "nodeloc_oauth_state"
	oauthBindCookie  = "nodeloc_oauth_bind"
)

type Handler struct {
	service *application.Service
}

func NewHandler(service *application.Service) *Handler {
	return &Handler{service: service}
}

// cookieSecure marks the OAuth state cookie Secure only when this request
// really arrived over TLS. Reading the configured scheme instead is what caused
// "登录未完成，可能是链接过期": a store reached over plain http (LAN, or a
// container port without TLS) keeps the default https setting, the browser then
// silently drops the cookie and the callback can never match its state.
func (h *Handler) cookieSecure(c *gin.Context) bool {
	if c.Request != nil && c.Request.TLS != nil {
		return true
	}
	if c.GetHeader("X-Forwarded-Proto") == "https" || c.GetHeader("X-Forwarded-Ssl") == "on" {
		return true
	}
	return false
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

	admin := router.Group("/api/v1/admin/users")
	admin.Use(h.AuthMiddleware(), middleware.RequireAdmin(h.AccountReader()))
	admin.GET("", h.AdminListUsers)
	admin.GET("/:id", h.AdminGetUser)
	admin.POST("/:id/role", h.AdminSetRole)
	admin.POST("/:id/toggle-admin", h.AdminToggleAdmin)
	admin.POST("/:id/toggle-active", h.AdminToggleActive)
	admin.POST("/:id/points", h.AdminAdjustPoints)
}

// AccountReader lets the admin guard re-check the role behind the current
// token, so granting or revoking admin access applies immediately.
func (h *Handler) AccountReader() middleware.AccountReader {
	return func(ctx context.Context, userID uint) (middleware.AccountState, bool) {
		user, err := h.service.AdminGetUser(ctx, userID)
		if err != nil || user == nil {
			return middleware.AccountState{}, false
		}
		return middleware.AccountState{Role: user.Role, IsAdmin: user.IsAdmin, IsActive: user.IsActive}, true
	}
}

func (h *Handler) AdminListUsers(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit <= 0 {
		limit = 20
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		offset = 0
	}
	users, total, err := h.service.AdminListUsers(c.Request.Context(), limit, offset, c.Query("q"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": users, "total": total, "limit": limit, "offset": offset})
}

func (h *Handler) AdminGetUser(c *gin.Context) {
	user, err := h.service.AdminGetUser(c.Request.Context(), idParam(c))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (h *Handler) AdminSetRole(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		writeError(c, domain.ErrInvalidCredentials)
		return
	}
	var request struct {
		Role string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, domain.ErrInvalidInput)
		return
	}
	user, err := h.service.AdminSetRole(c.Request.Context(), claims.UserID, idParam(c), request.Role)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (h *Handler) AdminToggleAdmin(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		writeError(c, domain.ErrInvalidCredentials)
		return
	}
	user, err := h.service.AdminToggleAdmin(c.Request.Context(), claims.UserID, idParam(c))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (h *Handler) AdminToggleActive(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		writeError(c, domain.ErrInvalidCredentials)
		return
	}
	user, err := h.service.AdminToggleActive(c.Request.Context(), claims.UserID, idParam(c))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (h *Handler) AdminAdjustPoints(c *gin.Context) {
	var request struct {
		Delta *int `json:"delta" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.Delta == nil {
		writeError(c, domain.ErrInvalidInput)
		return
	}
	user, err := h.service.AdminAdjustPoints(c.Request.Context(), idParam(c), *request.Delta)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

// idParam reads the numeric :id path parameter; 0 means it was missing or junk.
func idParam(c *gin.Context) uint {
	id, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 32)
	if err != nil {
		return 0
	}
	return uint(id)
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
		Password   string `json:"password" binding:"required"`
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
	c.SetCookie(oauthStateCookie, "", -1, "/", "", h.cookieSecure(c), true)
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

func (h *Handler) InitiateOAuth(c *gin.Context) {
	redirectURL, state, err := h.service.InitiateOAuth("")
	if err != nil {
		writeError(c, err)
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(oauthStateCookie, state, int((10 * time.Minute).Seconds()), "/api/v1/auth", "", h.cookieSecure(c), true)
	// A bind intent tells the callback to hand the code back to the signed-in
	// SPA instead of logging the NodeLoc identity in.
	if c.Query("bind") == "true" {
		c.SetCookie(oauthBindCookie, "1", int((10 * time.Minute).Seconds()), "/api/v1/auth", "", h.cookieSecure(c), true)
	} else {
		c.SetCookie(oauthBindCookie, "", -1, "/api/v1/auth", "", h.cookieSecure(c), true)
	}
	if c.Query("redirect") == "true" {
		c.Redirect(http.StatusFound, redirectURL)
		return
	}
	c.JSON(http.StatusOK, gin.H{"authorization_url": redirectURL, "state": state})
}

func (h *Handler) OAuthCallback(c *gin.Context) {
	binding := false
	if value, err := c.Cookie(oauthBindCookie); err == nil && value != "" {
		binding = true
	}
	c.SetCookie(oauthBindCookie, "", -1, "/api/v1/auth", "", h.cookieSecure(c), true)

	// NodeLoc answers a rejected authorization with error + error_description +
	// state and no code. Without this branch that reason only showed up as a
	// generic "链接过期" on the login page.
	if providerError := strings.TrimSpace(c.Query("error")); providerError != "" {
		log.Printf("nodeloc oauth: the provider refused the authorization: error=%s description=%q",
			providerError, strings.TrimSpace(c.Query("error_description")))
		h.oauthFailure(c, "denied", domain.ErrInvalidCredentials, binding)
		return
	}

	stateCookie, cookieErr := c.Cookie(oauthStateCookie)
	switch {
	case cookieErr != nil || stateCookie == "":
		log.Printf("nodeloc oauth: no state cookie on the callback, so the round trip was interrupted (state_in_redirect=%v)",
			c.Query("state") != "")
		h.oauthFailure(c, "expired", domain.ErrInvalidCredentials, binding)
		return
	case c.Query("state") == "" || stateCookie != c.Query("state"):
		log.Printf("nodeloc oauth: state mismatch between cookie and callback query")
		h.oauthFailure(c, "state", domain.ErrInvalidCredentials, binding)
		return
	}
	if binding {
		// The SPA redeems the code against POST /auth/bind-oauth with its own
		// bearer token; fragments never reach a server or log.
		c.SetCookie(oauthStateCookie, "", -1, "/api/v1/auth", "", h.cookieSecure(c), true)
		fragment := url.Values{}
		fragment.Set("bind_code", c.Query("code"))
		fragment.Set("state", c.Query("state"))
		c.Redirect(http.StatusFound, "/profile#"+fragment.Encode())
		return
	}
	params := queryParams(c)
	result, err := h.service.OAuthLogin(c.Request.Context(), c.Query("code"), params)
	if err != nil {
		log.Printf("nodeloc oauth: the code exchange failed: %v", err)
		h.oauthFailure(c, "provider", err, false)
		return
	}
	c.SetCookie(oauthStateCookie, "", -1, "/api/v1/auth", "", h.cookieSecure(c), true)
	// XHR clients (SPA) get JSON; browser navigations are bounced back to the
	// SPA callback page with the token in the URL fragment (never logged).
	if acceptsJSON(c) {
		c.JSON(http.StatusOK, result)
		return
	}
	fragment := url.Values{}
	fragment.Set("access_token", result.Tokens.AccessToken)
	c.Redirect(http.StatusFound, "/oauth/callback#"+fragment.Encode())
}

// oauthFailure answers the callback: JSON for the SPA, otherwise a redirect so
// a browser that NodeLoc bounced back here never sees a bare error document.
// The reason code travels to the SPA so the copy can name the actual cause.
func (h *Handler) oauthFailure(c *gin.Context, reason string, err error, binding bool) {
	if acceptsJSON(c) {
		writeError(c, err)
		return
	}
	if binding {
		c.Redirect(http.StatusFound, "/profile?oauth_error="+reason)
		return
	}
	c.Redirect(http.StatusFound, "/login?oauth_error="+reason)
}

func acceptsJSON(c *gin.Context) bool {
	return strings.Contains(c.GetHeader("Accept"), "application/json")
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
		// RequireAdmin reads the shared context keys, so this middleware has to
		// populate them as well or every admin user route is rejected as anonymous.
		c.Set(middleware.UserRoleKey, claims.Role)
		c.Set(middleware.IsAdminKey, claims.IsAdmin)
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
	default:
		// The client only sees a generic message, so the real cause has to reach
		// the server log or unexplained 500s cannot be diagnosed.
		log.Printf("[identity] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
	}
	c.JSON(status, gin.H{"error": message})
}
