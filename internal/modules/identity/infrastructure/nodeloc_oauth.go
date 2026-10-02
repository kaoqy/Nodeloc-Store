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

	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/contract"
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

	userResponse, err := n.userinfo(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	scope := firstNonEmpty(tokenResponse.Scope, n.scopes)

	profile, err := n.profile(userResponse, accessToken, tokenResponse.RefreshToken, scope)
	if err != nil {
		return nil, err
	}
	return profile, nil
}

// FetchProfile re-reads the account behind a stored access token. It is the
// 同步资料 path: a buyer who changes their 头像 or confirms their email on
// NodeLoc gets the new values here without a sign-out round trip.
func (n *NodeLocOAuth) FetchProfile(ctx context.Context, accessToken string) (*domain.OAuthProfile, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil, errors.New("access token is required")
	}
	userResponse, err := n.userinfo(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	profile, err := n.profile(userResponse, accessToken, "", n.scopes)
	if err != nil {
		return nil, err
	}
	return profile, nil
}

// userinfo loads the account behind an access token and hands back the raw payload.
// The account id is deliberately not resolved here: the documented reply, the
// OIDC-shaped one and the forum-shaped one each spell it differently, and profile()
// is where that spelling is decided.
func (n *NodeLocOAuth) userinfo(ctx context.Context, accessToken string) (nodeLocUserResponse, error) {
	var userResponse nodeLocUserResponse
	if err := n.getBearing(ctx, "/oauth-provider/userinfo", accessToken, &userResponse); err != nil {
		return nodeLocUserResponse{}, fmt.Errorf("retrieve nodeloc userinfo: %w", err)
	}
	return userResponse, nil
}

// profile reads one NodeLoc userinfo payload into the store's profile.
//
// The account id is the field that decides whether a login can bind at all. The
// official reply (see https://docs.nodeloc.com/api-reference/introduction) documents it as
// an integer — `{"id": 123, "username": "user1", …}` — so decoding it as a Go string
// failed outright on the documented answer and every buyer saw 「资料里没有账号标识」
// after NodeLoc had already accepted their code. The string spellings OIDC and some
// forum releases use (`uid`, `sub`, `user_id`) are still read through nodeLocIdentifier.
func (n *NodeLocOAuth) profile(userResponse nodeLocUserResponse, accessToken, refreshToken, scope string) (*domain.OAuthProfile, error) {
	uid := nodeLocIdentifier(userResponse)
	if uid == "" {
		// No account id means no identity to bind. Creating a user keyed on an
		// empty string would merge every such buyer into one account, so this is
		// a refusal that names the claim the forum left out.
		return nil, fmt.Errorf("%w：NodeLoc 的用户资料里没有账号标识（id / uid / sub）", domain.ErrOAuthRejected)
	}
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
	}, nil
}

type nodeLocUserResponse struct {
	// ID is the documented account identifier. The official reply carries it as an
	// integer (`{"id": 123, …}`); OIDC-shaped and older deployments answer a string.
	// flexibleText absorbs either without turning the documented spelling into a
	// decode error that throws the whole profile away.
	ID          flexibleText    `json:"id"`
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

// probeAuthorizationCode is the code 「测试 NodeLoc 登录」 tries to redeem. It cannot
// exist: NodeLoc only ever hands one out at the end of a buyer's browser redirect, and
// it is single-use. So the answer to this request is an answer about the credentials
// alone — no account is touched, nothing is written, and no buyer is logged in.
const probeAuthorizationCode = "nodeloc-store-oauth-connectivity-probe"

// Probe is the settings page's one server-side OAuth check.
//
// Building an authorization URL proves only that four fields are non-empty: the real
// forum answers /oauth-provider/authorize with a redirect to its own login page even
// for a client_id that has never existed, so a shop with a rebuilt, reset or mistyped
// Client Secret saw 「配置正确」 there and every buyer then hit a wall after logging in.
// The token endpoint is the one call that sorts the two apart without a browser: it
// looks the client pair up first, and only then looks at the code.
func (n *NodeLocOAuth) Probe(ctx context.Context) contract.OAuthProbe {
	if err := n.guard(); err != nil {
		return contract.OAuthProbe{Code: "not_configured", Detail: err.Error()}
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", probeAuthorizationCode)
	form.Set("redirect_uri", n.redirectURI)
	form.Set("client_id", n.clientID)
	form.Set("client_secret", n.clientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.baseURL+"/oauth-provider/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return contract.OAuthProbe{Code: "not_configured", Detail: err.Error()}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	response, err := n.httpClient.Do(req)
	if err != nil {
		return contract.OAuthProbe{Code: "unreachable", Detail: fmt.Sprintf("%v", err)}
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if readErr != nil {
		return contract.OAuthProbe{Code: "unreachable", Detail: fmt.Sprintf("读取 NodeLoc 响应失败: %v", readErr)}
	}
	return classifyProbe(response.StatusCode, body)
}

// classifyProbe reads the token endpoint's answer. The OAuth error name is the evidence,
// because everything else about the answer is locale- and release-dependent: an
// 「invalid_grant」 says this client pair is known and only the made-up code was refused,
// while an 「invalid_client」 is the pair itself.
//
// A web page is neither of those, however it arrived. The shop's own SPA answers unknown
// paths with a 200 and HTML, so 「HTTP 200」 alone is not proof of an OAuth endpoint —
// reading it that way is the same false green this probe exists to remove. Only a JSON
// answer with no error in it counts as NodeLoc having taken the credentials.
func classifyProbe(status int, body []byte) contract.OAuthProbe {
	name, description := oauthErrorName(body)
	page := isWebPage(body)
	accepted := status >= http.StatusOK && status < http.StatusMultipleChoices && !page
	if name == "" && !page && status >= http.StatusOK && status < http.StatusMultipleChoices {
		// A body that is JSON but carries no access_token either says nothing about
		// this probe: it is the forum answering in a shape the store cannot read.
		if looksLikeTokenAnswer(body) {
			return contract.OAuthProbe{}
		}
		return contract.OAuthProbe{Code: "rejected", Detail: probeQuote(status, body)}
	}
	if name == "" {
		if page || status == http.StatusNotFound {
			return contract.OAuthProbe{Code: "route_missing", Detail: probeQuote(status, body)}
		}
		return contract.OAuthProbe{Code: "rejected", Detail: probeQuote(status, body)}
	}
	switch name {
	case "invalid_client", "unauthorized_client":
		return contract.OAuthProbe{Code: "client_rejected", Detail: probeQuote(status, body)}
	case "invalid_grant":
		if mentionsRedirect(description) {
			return contract.OAuthProbe{Code: "redirect_mismatch", Detail: probeQuote(status, body)}
		}
		if accepted {
			// A 200 that still reports invalid_grant is a forum answering strangely;
			// the credentials were not the thing it refused, so it is not proof either.
			return contract.OAuthProbe{Code: "rejected", Detail: probeQuote(status, body)}
		}
		// The client was recognised; only this impossible code was refused. That is
		// the healthy answer.
		return contract.OAuthProbe{}
	case "unsupported_grant_type":
		return contract.OAuthProbe{Code: "grant_unsupported", Detail: probeQuote(status, body)}
	case "invalid_scope":
		return contract.OAuthProbe{Code: "scope_rejected", Detail: probeQuote(status, body)}
	}
	return contract.OAuthProbe{Code: "rejected", Detail: probeQuote(status, body)}
}

// isWebPage spots HTML where JSON was promised — the forum's own 「找不到页面」 page, a
// captive login page, or the shop's SPA catch-all.
func isWebPage(body []byte) bool {
	head := strings.TrimLeft(string(body), " \t\r\n")
	return strings.HasPrefix(strings.ToLower(head), "<!doctype html") || strings.HasPrefix(head, "<html")
}

// looksLikeTokenAnswer accepts both documented shapes (the claims at the top level and
// the payload under `data`), because the probe only needs to know that the endpoint spoke
// OAuth back.
func looksLikeTokenAnswer(body []byte) bool {
	var envelope struct {
		AccessToken string          `json:"access_token"`
		Error       string          `json:"error"`
		Data        json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return false
	}
	if strings.TrimSpace(envelope.AccessToken) != "" || strings.TrimSpace(envelope.Error) != "" {
		return true
	}
	if len(envelope.Data) == 0 {
		return false
	}
	var inner struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	return json.Unmarshal(envelope.Data, &inner) == nil
}

// oauthErrorName reads {"error":…,"error_description":…} out of the token endpoint's
// answer. A body that is not that shape yields an empty name, which is itself the
// diagnosis: the request never reached an OAuth endpoint.
func oauthErrorName(body []byte) (string, string) {
	var payload struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", ""
	}
	return strings.ToLower(strings.TrimSpace(payload.Error)), strings.TrimSpace(payload.ErrorDescription)
}

func mentionsRedirect(description string) bool {
	lowered := strings.ToLower(description)
	return strings.Contains(lowered, "redirect") || strings.Contains(description, "回调") ||
		strings.Contains(description, "重定向")
}

// probeQuote keeps NodeLoc's own words for the 设置 page, short enough to read in one
// line and never a whole 404 web page: the forum's HTML error page is evidence about the
// route, and pasting its markup at a shop owner proves nothing they can act on.
func probeQuote(status int, body []byte) string {
	if head := strings.TrimLeft(string(body), " \t\r\n"); strings.HasPrefix(head, "<") {
		return fmt.Sprintf("HTTP %d：这个地址回的是论坛网页而不是 OAuth 应答（页面标题：%s）", status, pageTitle(body))
	}
	summary := summarizeBody(body)
	if summary == "" {
		summary = "空响应"
	}
	return fmt.Sprintf("HTTP %d：%s", status, summary)
}

// pageTitle reads the <title> of an HTML answer, which is the only part of a forum error
// page that tells the owner anything.
func pageTitle(body []byte) string {
	lowered := strings.ToLower(string(body))
	start := strings.Index(lowered, "<title>")
	if start < 0 {
		return "找不到页面"
	}
	rest := string(body)[start+len("<title>"):]
	if end := strings.Index(strings.ToLower(rest), "</title>"); end >= 0 {
		if title := strings.TrimSpace(rest[:end]); title != "" {
			return title
		}
	}
	return "找不到页面"
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
		return fmt.Errorf("%w（HTTP %d）：%s", domain.ErrOAuthRejected, response.StatusCode, oauthRefusal(body))
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
// oauthCodes are the standard OAuth error names NodeLoc's token endpoint answers with.
// Verified against the live forum: POST /oauth-provider/token with an unknown client
// returns {"error":"invalid_client","error_description":"Invalid client credentials"},
// and GET /oauth-provider/userinfo without a token returns {"error":"invalid_token"}.
// Naming that code is the difference between a shop owner reading 「Client ID 与 Secret
// 对不上这个应用」 and reading an English JSON blob they have to interpret themselves.
var oauthCodes = []struct{ code, advice string }{
	{"invalid_client", "Client ID 与 Client Secret 对不上这个 OAuth 应用（两串填反、应用被重建或 Secret 被重置都会这样）"},
	{"invalid_grant", "授权码已经用过或已经过期，让买家重新点一次「NodeLoc 登录」就好"},
	{"unauthorized_client", "这个 OAuth 应用没有被允许使用商店走的授权方式"},
	{"unsupported_grant_type", "商店提交的授权方式不被这个 OAuth 应用支持"},
	{"invalid_scope", "应用申请的 scope（通常是 email）没有通过 NodeLoc 的审核"},
	{"invalid_token", "NodeLoc 不认这个访问令牌，重新登录一次即可"},
	{"access_denied", "买家在 NodeLoc 那一侧点了取消"},
	{"server_error", "NodeLoc 的 OAuth 服务自己报错了，稍后重试"},
	{"temporarily_unavailable", "NodeLoc 的 OAuth 服务暂时不可用，稍后重试"},
}

// oauthRefusal turns NodeLoc's OAuth answer into the sentence an owner can act on, and
// keeps the provider's own words along with it.
func oauthRefusal(body []byte) string {
	trimmed := strings.TrimLeft(string(body), " \t\r\n")
	if strings.HasPrefix(trimmed, "<") {
		// A page where JSON was promised: the request never reached the OAuth extension,
		// so this is not a credential problem.
		return "NodeLoc 回的是网页而不是 OAuth 应答，请求多半没走到论坛的 OAuth 扩展（这个域上没有 OAuth 应用，或被论坛前面的缓存/防火墙拦下）"
	}
	var payload struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		Message          string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		code := strings.ToLower(strings.TrimSpace(payload.Error))
		for _, known := range oauthCodes {
			if code == known.code {
				if description := strings.TrimSpace(payload.ErrorDescription); description != "" {
					return known.advice + "（NodeLoc 原文：" + description + "）"
				}
				return known.advice
			}
		}
		if sentence := firstNonEmpty(payload.ErrorDescription, payload.Message, payload.Error); sentence != "" {
			return sentence
		}
	}
	return summarizeBody(body)
}

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

// flexibleText is a JSON scalar that may arrive quoted or bare. NodeLoc documents
// the account id as an integer while some releases (and OIDC in general) send the
// same claim as a string; both are read into the same trimmed text.
type flexibleText string

func (value *flexibleText) UnmarshalJSON(raw []byte) error {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		*value = ""
		return nil
	}
	if text[0] == '"' {
		var decoded string
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return err
		}
		*value = flexibleText(strings.TrimSpace(decoded))
		return nil
	}
	// A bare scalar (123, 123.0, true) is kept verbatim after the JSON wrapper is
	// trimmed, which is what the store binds on.
	*value = flexibleText(text)
	return nil
}

// nodeLocIdentifier resolves the account id from every spelling NodeLoc has used.
func nodeLocIdentifier(response nodeLocUserResponse) string {
	return firstNonEmpty(string(response.ID), response.UID, response.UserID, response.Subject)
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
