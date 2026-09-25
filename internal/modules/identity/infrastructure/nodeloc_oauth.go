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
}

func NewNodeLocOAuth(config NodeLocOAuthConfig, client *http.Client) (*NodeLocOAuth, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" || strings.TrimSpace(config.ClientID) == "" || strings.TrimSpace(config.ClientSecret) == "" || strings.TrimSpace(config.RedirectURI) == "" {
		return nil, errors.New("nodeloc oauth configuration is incomplete")
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("invalid nodeloc base URL: %w", err)
	}
	if _, err := url.ParseRequestURI(config.RedirectURI); err != nil {
		return nil, fmt.Errorf("invalid nodeloc redirect URI: %w", err)
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &NodeLocOAuth{
		baseURL:      baseURL,
		clientID:     strings.TrimSpace(config.ClientID),
		clientSecret: strings.TrimSpace(config.ClientSecret),
		redirectURI:  strings.TrimSpace(config.RedirectURI),
		scopes:       strings.TrimSpace(config.Scopes),
		httpClient:   client,
	}, nil
}

func (n *NodeLocOAuth) Name() string { return nodeLocProviderName }

// AuthorizationURL builds the standard authorization-code request:
// GET {base}/oauth-provider/authorize
func (n *NodeLocOAuth) AuthorizationURL(state string) (string, error) {
	state = strings.TrimSpace(state)
	if state == "" {
		return "", errors.New("oauth state is required")
	}
	params := url.Values{}
	params.Set("client_id", n.clientID)
	params.Set("redirect_uri", n.redirectURI)
	params.Set("response_type", "code")
	params.Set("state", state)
	if n.scopes != "" {
		params.Set("scope", n.scopes)
	}
	return n.baseURL + "/oauth-provider/authorize?" + params.Encode(), nil
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
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := n.postForm(ctx, "/oauth-provider/token", form, &tokenResponse); err != nil {
		return nil, fmt.Errorf("obtain nodeloc access token: %w", err)
	}
	accessToken := strings.TrimSpace(tokenResponse.AccessToken)
	if accessToken == "" {
		return nil, errors.New("nodeloc token response did not contain access_token")
	}

	var userResponse nodeLocUserResponse
	if err := n.getBearing(ctx, "/oauth-provider/userinfo", accessToken, &userResponse); err != nil {
		return nil, fmt.Errorf("retrieve nodeloc userinfo: %w", err)
	}

	uid := firstNonEmpty(userResponse.ID, userResponse.UID, userResponse.UserID)
	if uid == "" {
		return nil, errors.New("nodeloc userinfo response did not contain a user ID")
	}
	email := optionalString(userResponse.Email)
	trustLevel := parseOptionalInt(userResponse.TrustLevel)
	scope := firstNonEmpty(tokenResponse.Scope, n.scopes)

	return &domain.OAuthProfile{
		Provider:     nodeLocProviderName,
		ProviderUID:  uid,
		Username:     firstNonEmpty(userResponse.Username, userResponse.Name, "nodeloc-"+uid),
		DisplayName:  firstNonEmpty(userResponse.Name, userResponse.DisplayName, userResponse.Username),
		Email:        email,
		AvatarURL:    firstNonEmpty(userResponse.AvatarURL, userResponse.Avatar),
		TrustLevel:   trustLevel,
		Scope:        scope,
		AccessToken:  accessToken,
		RefreshToken: tokenResponse.RefreshToken,
	}, nil
}

type nodeLocUserResponse struct {
	ID          string          `json:"id"`
	UID         string          `json:"uid"`
	UserID      string          `json:"user_id"`
	Username    string          `json:"username"`
	Name        string          `json:"name"`
	DisplayName string          `json:"display_name"`
	Email       string          `json:"email"`
	Avatar      string          `json:"avatar"`
	AvatarURL   string          `json:"avatar_url"`
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
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("nodeloc returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode nodeloc response: %w", err)
	}
	return nil
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
