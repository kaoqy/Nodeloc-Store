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
	"github.com/kaoqy/Nodeloc-Store/internal/authz"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/domain"
)

const (
	claimsKey        = "identity_claims"
	oauthStatePrefix = "nodeloc_oauth_state_"
	oauthBindPrefix  = "nodeloc_oauth_bind_"
	// oauthCookiePath is scoped to the auth routes: the round trip only ever
	// lands back on /api/v1/auth/oauth/callback, and a path this narrow is what
	// clearing the cookie has to name for the browser to actually drop it.
	oauthCookiePath = "/api/v1/auth"

	oauthStepInitiate = "initiate"
	oauthStepCallback = "callback"
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
	authed := router.Group("/api/v1/auth", h.AuthMiddleware())
	auth := router.Group("/api/v1/auth")
	auth.POST("/register", h.Register)
	auth.POST("/login", h.Login)
	auth.POST("/logout", h.Logout)
	auth.POST("/refresh", h.RefreshToken)
	auth.GET("/oauth/initiate", h.InitiateOAuth)
	auth.GET("/oauth/callback", h.OAuthCallback)
	authed.POST("/bind-oauth", h.BindOAuth)
	authed.DELETE("/unbind-oauth", h.UnbindOAuth)
	authed.GET("/me", h.Me)
	authed.PATCH("/me", h.UpdateProfile)
	authed.GET("/me/permissions", h.MyPermissions)
	authed.GET("/me/points", h.MyPoints)
	authed.POST("/me/sync-oauth", h.SyncOAuthProfile)
	authed.GET("/checkin/status", h.CheckinStatus)
	authed.GET("/checkin/history", h.CheckinHistory)
	authed.POST("/checkin", h.CheckIn)

	// The back office is gated per resource, not "any admin at all": 客服 gets
	// the user list, granting roles stays with 管理员 and above.
	accounts := h.AccountReader()
	guard := func(resource, action string) gin.HandlerFunc {
		return middleware.RequirePermission(accounts, resource, action)
	}
	admin := router.Group("/api/v1/admin/users", h.AuthMiddleware())
	admin.GET("", guard("users", "view"), h.AdminListUsers)
	admin.GET("/:id", guard("users", "view"), h.AdminGetUser)
	admin.POST("/:id/role", guard("roles", "manage"), h.AdminSetRole)
	admin.POST("/:id/toggle-admin", guard("roles", "manage"), h.AdminToggleAdmin)
	admin.POST("/:id/toggle-active", guard("users", "manage"), h.AdminToggleActive)
	admin.POST("/:id/points", guard("users", "manage"), h.AdminAdjustPoints)

	// 设置 页读这份登录记录：买家只会说「登录不了」，而真正的原因——回调地址对不上、
	// 浏览器没带回 state、NodeLoc 拒绝了授权——只出现在容器日志里。
	settings := router.Group("/api/v1/admin", h.AuthMiddleware())
	settings.GET("/oauth-attempts", guard("settings", "view"), h.AdminOAuthAttempts)
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
	user, err := h.service.AdminSetRole(c.Request.Context(), claims.UserID, actorRole(c), idParam(c), request.Role)
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
	user, err := h.service.AdminToggleAdmin(c.Request.Context(), claims.UserID, actorRole(c), idParam(c))
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
	claims, ok := claimsFromContext(c)
	if !ok {
		writeError(c, domain.ErrInvalidCredentials)
		return
	}
	var request struct {
		Delta *int `json:"delta" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.Delta == nil {
		writeError(c, domain.ErrInvalidInput)
		return
	}
	user, err := h.service.AdminAdjustPoints(c.Request.Context(), claims.UserID, idParam(c), *request.Delta)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

// AdminOAuthAttempts is the 设置 page's 「最近 NodeLoc 登录记录」: the last round
// trips, newest first, with the step each one stopped at. A shop owner cannot
// read a container log from the storefront, and 「登录不了」 without a step attached
// is not something they can fix.
func (h *Handler) AdminOAuthAttempts(c *gin.Context) {
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit 要是一个数字。", "code": "invalid_query"})
			return
		}
		limit = parsed
	}
	attempts, err := h.service.OAuthAttempts(c.Request.Context(), limit)
	if err != nil {
		writeError(c, err)
		return
	}
	if attempts == nil {
		attempts = []domain.OAuthAttempt{}
	}
	c.JSON(http.StatusOK, gin.H{"data": attempts})
}

// RefreshToken trades a refresh token for a new access token, so a session
// lasts as long as the refresh TTL instead of dropping the buyer every two
// hours.
func (h *Handler) RefreshToken(c *gin.Context) {
	var request struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, domain.ErrInvalidCredentials)
		return
	}
	result, err := h.service.Refresh(c.Request.Context(), request.RefreshToken)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// CheckIn pays out today's 签到.
func (h *Handler) CheckIn(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		writeError(c, domain.ErrInvalidCredentials)
		return
	}
	checkin, user, err := h.service.CheckIn(c.Request.Context(), claims.UserID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"reward":           checkin.RewardPoints,
		"consecutive_days": checkin.ConsecutiveDays,
		"total_checkins":   user.TotalCheckins,
		"points":           user.Points,
		"user":             user,
	})
}

func (h *Handler) CheckinStatus(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		writeError(c, domain.ErrInvalidCredentials)
		return
	}
	status, err := h.service.CheckinStatus(c.Request.Context(), claims.UserID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, status)
}

func (h *Handler) CheckinHistory(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		writeError(c, domain.ErrInvalidCredentials)
		return
	}
	checkins, err := h.service.Checkins(c.Request.Context(), claims.UserID, positiveParam(c.DefaultQuery("limit", "30"), 30))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": checkins})
}

func (h *Handler) MyPoints(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		writeError(c, domain.ErrInvalidCredentials)
		return
	}
	limit := positiveParam(c.DefaultQuery("limit", "20"), 20)
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		offset = 0
	}
	entries, total, err := h.service.Points(c.Request.Context(), claims.UserID, limit, offset)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": entries, "total": total, "limit": limit, "offset": offset})
}

// UpdateProfile keeps the account's own nickname, avatar and bio in step with
// what NodeLoc knows, so 个人中心 does not have to stay all-or-nothing.
func (h *Handler) UpdateProfile(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		writeError(c, domain.ErrInvalidCredentials)
		return
	}
	var request struct {
		Nickname  *string `json:"nickname"`
		AvatarURL *string `json:"avatar_url"`
		Bio       *string `json:"bio"`
		Email     *string `json:"email"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, domain.ErrInvalidInput)
		return
	}
	user, err := h.service.UpdateProfile(c.Request.Context(), claims.UserID, domain.ProfileEdit{
		Nickname:  request.Nickname,
		AvatarURL: request.AvatarURL,
		Bio:       request.Bio,
		Email:     request.Email,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (h *Handler) SyncOAuthProfile(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		writeError(c, domain.ErrInvalidCredentials)
		return
	}
	user, err := h.service.SyncOAuthProfile(c.Request.Context(), claims.UserID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

// MyPermissions tells the storefront and the back office what the signed-in
// account may actually see, so navigation can be built from the server's answer
// rather than a second copy of the rules in the client.
func (h *Handler) MyPermissions(c *gin.Context) {
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
	permissions := authz.PermissionsOf(user.Role)
	if user.Role == "super_admin" {
		permissions = []string{"*:*"}
	}
	if permissions == nil {
		permissions = []string{}
	}
	c.JSON(http.StatusOK, gin.H{
		"role":        user.Role,
		"is_staff":    domain.IsStaff(user.Role),
		"permissions": permissions,
	})
}

// actorRole is the role the permission guard just verified against the store.
func actorRole(c *gin.Context) string {
	return c.GetString(middleware.UserRoleKey)
}

func positiveParam(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
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
	// A cookie is only dropped when the clearing names the same Path it was set
	// with; the round trip's pair lives under /api/v1/auth, so clearing at "/"
	// left a half-finished 登录 able to be picked up by the next buyer's browser.
	for _, name := range oauthCookieNames(c) {
		c.SetCookie(name, "", -1, oauthCookiePath, "", h.cookieSecure(c), true)
	}
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

func (h *Handler) InitiateOAuth(c *gin.Context) {
	binding := c.Query("bind") == "true"
	intent := "login"
	if binding {
		intent = "bind"
	}
	redirectURL, state, err := h.service.BeginOAuth(c.Request.Context(), "", intent, nil)
	if err != nil {
		if c.Query("redirect") != "true" {
			h.recordOAuthInitiateFailure(c, err)
			writeError(c, err)
			return
		}
		// This route is where a browser navigation starts, so a buyer who pressed
		// 用 NodeLoc 登录 on an unconfigured store must land back on the login page
		// with the reason, not on a wall of JSON.
		h.oauthFailure(c, oauthStepInitiate, oauthFailReason(err), err, err.Error(), binding)
		return
	}
	h.service.LogOAuthAttempt(c.Request.Context(), application.OAuthAttemptLog{
		Step:     oauthStepInitiate,
		Outcome:  "started",
		Redirect: callbackFromAuthorizeURL(redirectURL),
		Binding:  binding,
	})
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(oauthStateCookie(state), state, int((10 * time.Minute).Seconds()), oauthCookiePath, "", h.cookieSecure(c), true)
	// A bind intent tells the callback to hand the code back to the signed-in
	// SPA instead of logging the NodeLoc identity in.
	if binding {
		c.SetCookie(oauthBindCookie(state), "1", int((10 * time.Minute).Seconds()), oauthCookiePath, "", h.cookieSecure(c), true)
	} else {
		c.SetCookie(oauthBindCookie(state), "", -1, oauthCookiePath, "", h.cookieSecure(c), true)
	}
	if c.Query("redirect") == "true" {
		c.Redirect(http.StatusFound, redirectURL)
		return
	}
	c.JSON(http.StatusOK, gin.H{"authorization_url": redirectURL, "state": state})
}

func (h *Handler) OAuthCallback(c *gin.Context) {
	state := strings.TrimSpace(c.Query("state"))
	binding := false
	if state != "" {
		if value, err := c.Cookie(oauthBindCookie(state)); err == nil && value != "" {
			binding = true
		}
		c.SetCookie(oauthBindCookie(state), "", -1, oauthCookiePath, "", h.cookieSecure(c), true)
	}

	// Consume and validate the one-time state before handling either success or
	// provider error. NodeLoc includes state on denial too.
	stateCookie := ""
	var cookieErr error
	if state != "" {
		stateCookie, cookieErr = c.Cookie(oauthStateCookie(state))
	}
	h.clearOAuthState(c, state)
	if cookieErr != nil || stateCookie == "" {
		log.Printf("nodeloc oauth: no state cookie on the callback, so the round trip was interrupted (state_in_redirect=%v)", c.Query("state") != "")
		h.oauthFailure(c, oauthStepCallback, "expired", domain.ErrInvalidCredentials,
			"回调没有带回 state cookie；浏览器没存住、跨了域名回来、或登录超过 10 分钟都会这样。", binding)
		return
	}
	if c.Query("state") == "" || stateCookie != c.Query("state") {
		log.Printf("nodeloc oauth: state mismatch between cookie and callback query")
		h.oauthFailure(c, oauthStepCallback, "state", domain.ErrInvalidCredentials,
			"回调带回的 state 与本店发出的不是同一个。", binding)
		return
	}
	if _, err := h.service.ConsumeOAuth(c.Request.Context(), state); err != nil {
		log.Printf("nodeloc oauth: transaction already consumed or expired: %v", err)
		h.oauthFailure(c, oauthStepCallback, "transaction", domain.ErrOAuthTransaction,
			"这次 OAuth 授权已经处理过或已过期，请重新发起登录。", binding)
		return
	}

	// NodeLoc answers a rejected authorization with error + error_description +
	// state and no code. Without this branch that reason only showed up as a
	// generic "链接过期" on the login page.
	if providerError := strings.TrimSpace(c.Query("error")); providerError != "" {
		description := strings.TrimSpace(c.Query("error_description"))
		log.Printf("nodeloc oauth: the provider refused the authorization: error=%s description=%q",
			providerError, description)
		h.oauthFailure(c, oauthStepCallback, "denied", domain.ErrInvalidCredentials,
			strings.Join([]string{providerError, description}, " "), binding)
		return
	}

	if binding {
		// The SPA redeems the code against POST /auth/bind-oauth with its own
		// bearer token; fragments never reach a server or log.
		h.clearOAuthState(c, state)
		h.service.LogOAuthAttempt(c.Request.Context(), application.OAuthAttemptLog{
			Step: oauthStepCallback, Outcome: "started", Reason: "bind", Binding: true,
		})
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
		h.oauthFailure(c, oauthStepCallback, oauthFailReason(err), err, err.Error(), false)
		return
	}
	h.clearOAuthState(c, state)
	h.service.LogOAuthAttempt(c.Request.Context(), application.OAuthAttemptLog{
		Step:     oauthStepCallback,
		Outcome:  "success",
		Username: oauthAccountName(result),
	})
	// XHR clients (SPA) get JSON; browser navigations are bounced back to the
	// SPA callback page with the token in the URL fragment (never logged).
	if acceptsJSON(c) {
		c.JSON(http.StatusOK, result)
		return
	}
	fragment := url.Values{}
	fragment.Set("access_token", result.Tokens.AccessToken)
	// This pair is the shop's own JWTs, not NodeLoc's: the forum's token endpoint
	// answers a refresh_token grant with 「Refresh tokens are not supported」, so the
	// buyer's session can only outlive the access TTL through the store's own
	// /auth/refresh. A fragment is neither logged nor sent to any server.
	if result.Tokens.RefreshToken != "" {
		fragment.Set("refresh_token", result.Tokens.RefreshToken)
	}
	c.Redirect(http.StatusFound, "/oauth/callback#"+fragment.Encode())
}

// oauthFailReason turns the exchange failure into the code the login page reads,
// so 「设置里没填」 and 「NodeLoc 拒绝了」 are not the same sentence on the storefront.
func oauthFailReason(err error) string {
	switch {
	case errors.Is(err, domain.ErrOAuthDisabled):
		return "disabled"
	case errors.Is(err, domain.ErrOAuthNotConfigured):
		return "not_configured"
	case errors.Is(err, domain.ErrOAuthTransaction):
		return "transaction"
	case errors.Is(err, domain.ErrOAuthRejected):
		return "rejected"
	case errors.Is(err, domain.ErrOAuthUnreachable):
		return "unreachable"
	default:
		return "provider"
	}
}

// oauthFailure answers the callback: JSON for the SPA, otherwise a redirect so
// a browser that NodeLoc bounced back here never sees a bare error document.
// The reason code travels to the SPA so the copy can name the actual cause.
//
// It is also the shop's record of the attempt. A buyer sees one sentence and is
// gone; the owner is left with 「登录不了」 and no way to tell a wrong setting from a
// dead network, because the answer only ever sat in the container log.
func oauthStateCookie(state string) string {
	return oauthStatePrefix + state
}

func oauthBindCookie(state string) string {
	return oauthBindPrefix + state
}

func (h *Handler) clearOAuthState(c *gin.Context, state string) {
	if state == "" {
		return
	}
	c.SetCookie(oauthStateCookie(state), "", -1, oauthCookiePath, "", h.cookieSecure(c), true)
}

func oauthCookieNames(c *gin.Context) []string {
	cookieNames := []string{}
	for _, cookie := range c.Request.Cookies() {
		if strings.HasPrefix(cookie.Name, oauthStatePrefix) || strings.HasPrefix(cookie.Name, oauthBindPrefix) {
			cookieNames = append(cookieNames, cookie.Name)
		}
	}
	return cookieNames
}

func (h *Handler) oauthFailure(c *gin.Context, step, reason string, err error, detail string, binding bool) {
	h.service.LogOAuthAttempt(c.Request.Context(), application.OAuthAttemptLog{
		Step:    step,
		Outcome: "failed",
		Reason:  reason,
		Detail:  detail,
		Binding: binding,
	})
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

// oauthAccountName is who came through the door, for the shop's own record. The
// tokens in the same result are never written down.
func oauthAccountName(result *application.OAuthResult) string {
	if result == nil || result.User == nil {
		return ""
	}
	return result.User.Username
}

// recordOAuthInitiateFailure is the same note for a login that never left the
// shop: the SPA asked for the authorization URL and was refused.
func (h *Handler) recordOAuthInitiateFailure(c *gin.Context, err error) {
	h.service.LogOAuthAttempt(c.Request.Context(), application.OAuthAttemptLog{
		Step:    oauthStepInitiate,
		Outcome: "failed",
		Reason:  oauthFailReason(err),
		Detail:  err.Error(),
		Binding: c.Query("bind") == "true",
	})
}

// callbackFromAuthorizeURL keeps just the callback address out of the
// authorization URL. The rest of that URL is client_id and state — good for one
// round trip, and noise the shop's records should not carry.
func callbackFromAuthorizeURL(authorizeURL string) string {
	parsed, err := url.Parse(authorizeURL)
	if err != nil {
		return ""
	}
	return parsed.Query().Get("redirect_uri")
}

func acceptsJSON(c *gin.Context) bool {
	return strings.Contains(c.GetHeader("Accept"), "application/json")
}

// AuthMiddleware validates JWT and sets identity claims in context.
func (h *Handler) AuthMiddleware() gin.HandlerFunc {
	rejected := func(c *gin.Context) {
		// Its own copy, not the login one: someone whose session timed out needs
		// "请重新登录", not "用户名或密码不正确".
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": "登录状态已失效，请重新登录",
			"code":  "invalid_credentials",
		})
	}
	return func(c *gin.Context) {
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		if len(header) < 8 || !strings.EqualFold(header[:7], "Bearer ") {
			rejected(c)
			return
		}
		claims, err := h.service.Authenticate(c.Request.Context(), strings.TrimSpace(header[7:]))
		if err != nil {
			rejected(c)
			return
		}
		c.Set(claimsKey, claims)
		c.Set("user_id", claims.UserID)
		// RequirePermission reads the shared context keys, so this middleware has
		// to populate them as well or every admin route is rejected as anonymous.
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

// errorCopy is the storefront-facing answer for an identity failure. The
// domain sentinels are English (they are log material), so the buyer-facing
// wording lives here, keyed off the sentinel, with the specific reason a
// validation error carries kept visible.
func errorCopy(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		return http.StatusBadRequest, "invalid_input", detail(err, "信息填得不对，请检查后重试")
	case errors.Is(err, domain.ErrInvalidCredentials):
		return http.StatusUnauthorized, "invalid_credentials", detail(err, "用户名或密码不正确，或登录状态已过期")
	case errors.Is(err, domain.ErrInactiveUser):
		return http.StatusForbidden, "account_disabled", "账号已被停用，请联系管理员"
	case errors.Is(err, domain.ErrCheckinDisabled):
		return http.StatusForbidden, "checkin_disabled", "签到功能当前未开启"
	case errors.Is(err, domain.ErrRegistrationDisabled):
		return http.StatusForbidden, "registration_disabled", "本店已关闭注册，请使用 NodeLoc 账号登录，或联系店家开启"
	case errors.Is(err, domain.ErrUserNotFound):
		return http.StatusNotFound, "user_not_found", "用户不存在"
	case errors.Is(err, domain.ErrIdentityNotFound):
		return http.StatusNotFound, "identity_not_found", "还没有绑定 NodeLoc 账号"
	case errors.Is(err, domain.ErrNotBound):
		return http.StatusConflict, "not_bound", "还没有绑定 NodeLoc 账号，请先完成绑定"
	// The OAuth round trip has three answers the login page has to tell apart:
	// the store is not configured (the owner fixes 设置), NodeLoc refused
	// (credentials or a used code), NodeLoc was unreachable (egress). All three
	// used to land on 「服务器开小差了」, which is why a broken login reads as bad
	// luck rather than a setting.
	case errors.Is(err, domain.ErrOAuthDisabled):
		return http.StatusServiceUnavailable, "oauth_disabled", err.Error()
	case errors.Is(err, domain.ErrOAuthNotConfigured):
		// This one names the store's own empty fields and no secret value, and the
		// person who can fix it is usually the one testing the login.
		return http.StatusServiceUnavailable, "oauth_not_configured", err.Error()
	case errors.Is(err, domain.ErrOAuthTransaction):
		return http.StatusConflict, "oauth_transaction_used", "这次 OAuth 授权已经处理过或已过期，请重新发起登录。"
	case errors.Is(err, domain.ErrOAuthRejected):
		// NodeLoc's own words (error_description, an HTTP body) stay in the log:
		// they name credentials, which a buyer cannot use and a storefront should
		// not echo.
		return http.StatusBadGateway, "oauth_rejected", "NodeLoc 拒绝了这次登录（授权码可能已经用过或过期，或后台凭据不对）。请回到登录页重新点一次；反复出现请把这句话发给店家。"
	case errors.Is(err, domain.ErrOAuthUnreachable):
		return http.StatusBadGateway, "oauth_unreachable", "现在联系不上 NodeLoc，请稍后再试；如果一直如此，请店家检查 OAuth 接口地址与服务器网络。"
	case errors.Is(err, domain.ErrUsernameTaken):
		return http.StatusConflict, "username_taken", "用户名已被占用，换一个试试"
	case errors.Is(err, domain.ErrEmailTaken):
		return http.StatusConflict, "email_taken", "这个邮箱已经被其他账号使用"
	case errors.Is(err, domain.ErrIdentityAlreadyBound):
		return http.StatusConflict, "identity_bound", "这个 NodeLoc 账号已经绑定了其他用户"
	case errors.Is(err, domain.ErrLastLoginMethod):
		return http.StatusConflict, "last_login_method", "这是仅剩的登录方式，无法解绑"
	case errors.Is(err, domain.ErrAlreadyCheckedIn):
		return http.StatusConflict, "checked_in_today", "今天已经签到过啦，明天再来"
	}
	return http.StatusInternalServerError, "internal_error", "服务器开小差了，请稍后再试"
}

// detail keeps the reason a wrapped sentinel carries ("invalid input: 昵称最多
// 32 个字符" → the part after the colon) and falls back when there is none.
func detail(err error, fallback string) string {
	_, tail, found := strings.Cut(err.Error(), ": ")
	if found && strings.TrimSpace(tail) != "" {
		return tail
	}
	return fallback
}

func writeError(c *gin.Context, err error) {
	status, code, message := errorCopy(err)
	if status == http.StatusInternalServerError {
		// The client only sees a generic message, so the real cause has to reach
		// the server log or unexplained 500s cannot be diagnosed.
		log.Printf("[identity] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
	}
	c.JSON(status, gin.H{"error": message, "code": code})
}
