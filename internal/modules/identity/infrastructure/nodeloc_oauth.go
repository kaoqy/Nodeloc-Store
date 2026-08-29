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

// NodeLocOAuthConfig configures the NodeLoc OAuth2-compatible endpoints.
type NodeLocOAuthConfig struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Scopes       string
}

// NodeLocOAuth implements contract.OAuthProvider.
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

func (n *NodeLocOAuth) AuthorizationURL(state string) (string, error) {
	state = strings.TrimSpace(state)
	if state == "" {
		return "", errors.New("oauth state is required")
	}
	params := map[string]string{
		"client_id":     n.clientID,
		"redirect_uri":  n.redirectURI,
		"response_type": "code",
		"state":         state,
	}
	if n.scopes != "" {
		params["scope"] = n.scopes
	}
	params["signature"] = shared.SignWithHashedToken(cloneParams(params), n.clientSecret)

	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	return n.baseURL + "/oauth/authorize?" + values.Encode(), nil
}

func (n *NodeLocOAuth) VerifyCallback(params map[string]string) bool {
	return shared.VerifyCallback(cloneParams(params), n.clientSecret)
}

func (n *NodeLocOAuth) ExchangeCode(ctx context.Context, code string) (*domain.OAuthProfile, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, errors.New("authorization code is required")
	}

	// NodeLoc's flow first converts the callback code into an exchange code.
	exchangeRequest := map[string]string{
		"client_id":    n.clientID,
		"code":         code,
		"redirect_uri": n.redirectURI,
	}
	var exchangeResponse struct {
		ExchangeCode string `json:"exchange_code"`
		Code         string `json:"code"`
	}
	if err := n.postSignedForm(ctx, "/oauth/exchange_code", exchangeRequest, &exchangeResponse); err != nil {
		return nil, fmt.Errorf("exchange nodeloc authorization code: %w", err)
	}
	exchangeCode := strings.TrimSpace(exchangeResponse.ExchangeCode)
	if exchangeCode == "" {
		exchangeCode = strings.TrimSpace(exchangeResponse.Code)
	}
	if exchangeCode == "" {
		return nil, errors.New("nodeloc exchange response did not contain exchange_code")
	}

	// The exchange code is then redeemed for an access token.
	tokenRequest := map[string]string{
		"client_id":     n.clientID,
		"exchange_code": exchangeCode,
		"grant_type":    "authorization_code",
		"redirect_uri":  n.redirectURI,
	}
	var tokenResponse struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
	}
	if err := n.postSignedForm(ctx, "/oauth/token", tokenRequest, &tokenResponse); err != nil {
		return nil, fmt.Errorf("obtain nodeloc access token: %w", err)
	}
	if strings.TrimSpace(tokenResponse.AccessToken) == "" {
		return nil, errors.New("nodeloc token response did not contain access_token")
	}

	// Finally the access token is used to retrieve the NodeLoc user profile.
	userinfoRequest := map[string]string{
		"access_token": tokenResponse.AccessToken,
		"client_id":    n.clientID,
	}
	var userResponse nodeLocUserResponse
	if err := n.getSigned(ctx, "/oauth/userinfo", userinfoRequest, &userResponse); err != nil {
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
		DisplayName:  firstNonEmpty(userResponse.DisplayName, userResponse.Name, userResponse.Username),
		Email:        email,
		AvatarURL:    firstNonEmpty(userResponse.AvatarURL, userResponse.Avatar),
		TrustLevel:   trustLevel,
		Scope:        scope,
		AccessToken:  tokenResponse.AccessToken,
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

func (n *NodeLocOAuth) postSignedForm(ctx context.Context, path string, params map[string]string, target any) error {
	params = cloneParams(params)
	params["signature"] = shared.SignWithHashedToken(cloneParams(params), n.clientSecret)
	form := url.Values{}
	for key, value := range params {
		form.Set(key, value)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	return n.execute(req, target)
}

func (n *NodeLocOAuth) getSigned(ctx context.Context, path string, params map[string]string, target any) error {
	params = cloneParams(params)
	params["signature"] = shared.SignWithHashedToken(cloneParams(params), n.clientSecret)
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, n.baseURL+path+"?"+values.Encode(), nil)
	if err != nil {
		return err
	}
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
