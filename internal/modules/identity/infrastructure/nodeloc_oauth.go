package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/domain"
	"github.com/kaoqy/Nodeloc-Store/internal/shared"
)

const nodeLocProviderName = "nodeloc"

// NodeLocOAuthConfig configures the NodeLoc OAuth2 provider.
// See https://docs.nodeloc.com/api-reference/introduction
type NodeLocOAuthConfig struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Scopes       string
}

// NodeLocOAuth implements contract.OAuthProvider with the official
// NodeLoc OAuth Provider endpoints under /oauth-provider.
type NodeLocOAuth struct {
	baseURL      string
	clientID     string
	clientSecret string
	redirectURI  string
	scopes       string
	httpClient   *http.Client
	// missing names the settings that would make a login round trip impossible.
	// It is computed once, because a store that cannot offer NodeLoc login still
	// has to serve everything else.
	missing []string
}

// NewNodeLocOAuth builds the provider from the stored settings and never fails.
//
// This constructor runs inside the container rebuild that follows every settings
// save, and it used to return an error there — which identity.Wire turned into a
// panic, taking the whole storefront down over one empty field. An incomplete
// OAuth configuration is a missing feature, not a broken process: it now surfaces
// as an answer that names the field on 用 NodeLoc 登录.
func NewNodeLocOAuth(config NodeLocOAuthConfig, client *http.Client) *NodeLocOAuth {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	oauth := &NodeLocOAuth{
		baseURL:      strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"),
		clientID:     strings.TrimSpace(config.ClientID),
		clientSecret: strings.TrimSpace(config.ClientSecret),
		redirectURI:  strings.TrimSpace(config.RedirectURI),
		scopes:       strings.TrimSpace(config.Scopes),
		httpClient:   client,
	}
	oauth.missing = oauth.incomplete()
	return oauth
}

// incomplete lists the settings by the names the 设置 page uses for them, so the
// shop owner reads the diagnosis and finds the field.
func (n *NodeLocOAuth) incomplete() []string {
	missing := make([]string, 0, 4)
	if n.baseURL == "" {
		missing = append(missing, "OAuth 接口地址")
	} else if _, err := url.ParseRequestURI(n.baseURL); err != nil {
		missing = append(missing, "OAuth 接口地址（不是合法的 URL）")
	}
	if n.clientID == "" {
		missing = append(missing, "Client ID")
	}
	if n.clientSecret == "" {
		missing = append(missing, "Client Secret")
	}
	if n.redirectURI == "" {
		// The callback address is derived from 站点域名 when it is blank, so an
		// empty one here means the store has no domain to name either.
		missing = append(missing, "回调地址（请先在设置里填写站点域名）")
	} else if _, err := url.ParseRequestURI(n.redirectURI); err != nil {
		missing = append(missing, "回调地址（不是合法的 URL）")
	}
	return missing
}

// guard is the refusal every call to an unconfigured provider returns.
func (n *NodeLocOAuth) guard() error {
	if len(n.missing) == 0 {
		return nil
	}
	return fmt.Errorf("%w：缺少 %s", domain.ErrOAuthNotConfigured, strings.Join(n.missing, "、"))
}

func (n *NodeLocOAuth) Name() string { return nodeLocProviderName }

// AuthorizationURL builds the standard authorization-code request:
// GET {base}/oauth-provider/authorize
func (n *NodeLocOAuth) AuthorizationURL(state string) (string, error) {
	if err := n.guard(); err != nil {
		return "", err
	}
	state = strings.TrimSpace(state)
	if state == "" {
		return "", errors.New("oauth state is required")
	}
	params := url.Values{}
	params.Set("client_id", n.clientID)
	params.Set("redirect_uri", n.redirectURI)
	params.Set("response_type", "code")
	params.Set("state", state)
	// openid is mandatory on NodeLoc and cannot be removed from the
	// application, so sending a scope list without it is always wrong.
	if scope := normalizeScopes(n.scopes); scope != "" {
		params.Set("scope", scope)
	}
	return n.baseURL + "/oauth-provider/authorize?" + params.Encode(), nil
}

// normalizeScopes keeps the configured order, drops duplicates and guarantees
// openid is present.
func normalizeScopes(scopes string) string {
	seen := map[string]bool{}
	ordered := make([]string, 0, 4)
	for _, field := range strings.Fields(scopes) {
		field = strings.ToLower(field)
		if field == "" || seen[field] {
			continue
		}
		seen[field] = true
		ordered = append(ordered, field)
	}
	if !seen["openid"] {
		ordered = append([]string{"openid"}, ordered...)
	}
	return strings.Join(ordered, " ")
}

// VerifyCallback validates the browser redirect back from NodeLoc.
// The official callback carries only code + state (CSRF state is checked
// against a server-set cookie by the HTTP handler); no signed parameters.
func (n *NodeLocOAuth) VerifyCallback(params map[string]string) bool {
	if strings.TrimSpace(params["code"]) == "" || strings.TrimSpace(params["state"]) == "" {
		return false
	}
	// If a signature is ever present (custom/signed deployments) enforce it.
	if sig, ok := params["signature"]; ok && sig != "" {
		return shared.VerifyCallback(cloneParams(params), n.clientSecret)
	}
	return true
}

// ExchangeCode redeems the authorization code at /oauth-provider/token
// (client_secret_post) and then loads the profile from /oauth-provider/userinfo.
func (n *NodeLocOAuth) ExchangeCode(ctx context.Context, code string) (*domain.OAuthProfile, error) {
	if err := n.guard(); err != nil {
		return nil, err
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, errors.New("authorization code is required")
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", n.redirectURI)
	form.Set("client_id", n.clientID)
	form.Set("client_secret", n.clientSecret)

	var tokenResponse struct {
		AccessToken      string `json:"access_token"`
		RefreshToken     string `json:"refresh_token"`
		TokenType        string `json:"token_type"`
		ExpiresIn        int    `json:"expires_in"`
		Scope            string `json:"scope"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := n.postForm(ctx, "/oauth-provider/token", form, &tokenResponse); err != nil {
		return nil, fmt.Errorf("obtain nodeloc access token: %w", err)
	}
	accessToken := strings.TrimSpace(tokenResponse.AccessToken)
	if accessToken == "" {
		// An authorization code is single-use and expires quickly, so a buyer who
		// pressed 登录 twice gets this answer. Naming OAuth's own error word is the
		// difference between "重新点一次" and an unexplainable failure.
		reason := firstNonEmpty(tokenResponse.ErrorDescription, tokenResponse.Error)
		if reason == "" {
			reason = "响应里没有 access_token"
		}
		return nil, fmt.Errorf("%w：换取令牌失败（%s）", domain.ErrOAuthRejected, reason)
	}

	var userResponse nodeLocUserResponse
	if err := n.getBearing(ctx, "/oauth-provider/userinfo", accessToken, &userResponse); err != nil {
		return nil, fmt.Errorf("retrieve nodeloc userinfo: %w", err)
	}

	uid := firstNonEmpty(userResponse.ID, userResponse.UID, userResponse.UserID, userResponse.Subject)
	if uid == "" {
		return nil, fmt.Errorf("%w：NodeLoc 的用户资料里没有账号标识（id / uid / sub）", domain.ErrOAuthRejected)
	}
	scope := firstNonEmpty(tokenResponse.Scope, n.scopes)

	return n.profile(userResponse, uid, accessToken, tokenResponse.RefreshToken, scope), nil
}

// FetchProfile re-reads the account behind a stored access token. It is the
// 同步资料 path: a buyer who changes their 头像 or confirms their email on
// NodeLoc gets the new values here without a sign-out round trip.
func (n *NodeLocOAuth) FetchProfile(ctx context.Context, accessToken string) (*domain.OAuthProfile, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil, errors.New("access token is required")
	}
	var userResponse nodeLocUserResponse
	if err := n.getBearing(ctx, "/oauth-provider/userinfo", accessToken, &userResponse); err != nil {
		return nil, fmt.Errorf("retrieve nodeloc userinfo: %w", err)
	}
	uid := firstNonEmpty(userResponse.ID, userResponse.UID, userResponse.UserID, userResponse.Subject)
	if uid == "" {
		return nil, fmt.Errorf("%w：NodeLoc 的用户资料里没有账号标识（id / uid / sub）", domain.ErrOAuthRejected)
	}
	return n.profile(userResponse, uid, accessToken, "", n.scopes), nil
}

// profile reads one NodeLoc userinfo payload into the store's profile. The
// claim names differ between a forum-shaped answer and an OIDC one (id/uid/sub,
// username/preferred_username, avatar/avatar_url/picture), so all of them are
// consulted; the login worked for accounts whose reply happened to use the first
// spelling and failed for the rest.
func (n *NodeLocOAuth) profile(userResponse nodeLocUserResponse, uid, accessToken, refreshToken, scope string) *domain.OAuthProfile {
	return &domain.OAuthProfile{
		Provider:     nodeLocProviderName,
		ProviderUID:  uid,
		Username:     firstNonEmpty(userResponse.Username, userResponse.Preferred, userResponse.Name, "nodeloc-"+uid),
		DisplayName:  firstNonEmpty(userResponse.Name, userResponse.DisplayName, userResponse.Nickname, userResponse.Username),
		Email:        optionalString(userResponse.Email),
		AvatarURL:    firstNonEmpty(userResponse.AvatarURL, userResponse.Avatar, userResponse.Picture),
		TrustLevel:   parseOptionalInt(userResponse.TrustLevel),
		Scope:        scope,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}
}

type nodeLocUserResponse struct {
	ID          string          `json:"id"`
	UID         string          `json:"uid"`
	UserID      string          `json:"user_id"`
	Subject     string          `json:"sub"`
	Username    string          `json:"username"`
	Preferred   string          `json:"preferred_username"`
	Name        string          `json:"name"`
	Nickname    string          `json:"nickname"`
	DisplayName string          `json:"display_name"`
	Email       string          `json:"email"`
	Avatar      string          `json:"avatar"`
	AvatarURL   string          `json:"avatar_url"`
	Picture     string          `json:"picture"`
	TrustLevel  json.RawMessage `json:"trust_level"`
}

func (n *NodeLocOAuth) postForm(ctx context.Context, path string, form url.Values, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	return n.execute(req, target)
}

func (n *NodeLocOAuth) getBearing(ctx context.Context, path, accessToken string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, n.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	return n.execute(req, target)
}

func (n *NodeLocOAuth) execute(req *http.Request, target any) error {
	response, err := n.httpClient.Do(req)
	if err != nil {
		// The buyer cannot act on a dial failure, but the shop owner can: this is
		// egress, DNS, TLS or the host setting.
		return fmt.Errorf("%w：%v", domain.ErrOAuthUnreachable, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("%w：读取 NodeLoc 响应失败: %v", domain.ErrOAuthUnreachable, err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w（HTTP %d）：%s", domain.ErrOAuthRejected, response.StatusCode, summarizeBody(body))
	}

	// NodeLoc answers some endpoints with the payload under `data` and some with
	// the claims at the top level; the payment gateway has long accepted both. The
	// envelope may also carry {"success": false, "message": …} with a 200, which
	// used to read as an empty profile and fail as "资料里没有账号标识".
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("%w：NodeLoc 的返回不是可解析的 JSON：%s", domain.ErrOAuthRejected, summarizeBody(body))
	}
	if success, ok := envelope["success"]; ok && string(success) == "false" {
		var message struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		_ = json.Unmarshal(body, &message)
		return fmt.Errorf("%w：%s", domain.ErrOAuthRejected, firstNonEmpty(message.Message, message.Error, "NodeLoc 拒绝了请求"))
	}
	payload := body
	if inner, ok := envelope["data"]; ok {
		var nested map[string]json.RawMessage
		if json.Unmarshal(inner, &nested) == nil {
			for key, value := range envelope {
				if _, exists := nested[key]; !exists && key != "data" {
					nested[key] = value
				}
			}
			if rewritten, err := json.Marshal(nested); err == nil {
				payload = rewritten
			}
		}
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return fmt.Errorf("%w：decode nodeloc response: %v", domain.ErrOAuthRejected, err)
	}
	return nil
}

// summarizeBody keeps a provider body short enough to show a shop owner.
func summarizeBody(body []byte) string {
	text := strings.Join(strings.Fields(string(body)), " ")
	const limit = 180
	if len(text) > limit {
		return text[:limit] + "…"
	}
	if text == "" {
		return "空响应"
	}
	return text
}

func cloneParams(params map[string]string) map[string]string {
	copyOfParams := make(map[string]string, len(params))
	for key, value := range params {
		copyOfParams[key] = value
	}
	return copyOfParams
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func parseOptionalInt(raw json.RawMessage) *int {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var value int
	if err := json.Unmarshal(raw, &value); err == nil {
		return &value
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return nil
	}
	return &parsed
}
