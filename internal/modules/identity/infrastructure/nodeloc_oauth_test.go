package infrastructure

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/domain"
)

// The login asks for the openid scope, so an OIDC-shaped userinfo reply is one of
// the answers a NodeLoc release can give: the account id is `sub`, the avatar is
// `picture` and the handle is `preferred_username`. Reading only the forum-shaped
// names made the round trip succeed for some accounts and fail for the rest with
// a message about a missing user id.
func TestProfileSyncKeepsExistingRefreshToken(t *testing.T) {
	identity := &domain.OAuthIdentity{}
	identity.ApplyProfile(domain.OAuthProfile{AccessToken: "access-1", RefreshToken: "refresh-1"})
	identity.ApplyProfile(domain.OAuthProfile{AccessToken: "access-2"})
	if identity.RefreshToken == nil || *identity.RefreshToken != "refresh-1" {
		t.Fatalf("refresh token = %v, want existing token preserved", identity.RefreshToken)
	}
	if identity.AccessToken == nil || *identity.AccessToken != "access-2" {
		t.Fatalf("access token = %v, want latest token", identity.AccessToken)
	}
}

func TestLoginReadsAnOIDCShapedProfile(t *testing.T) {
	var tokenRequests int
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth-provider/token" {
			tokenRequests++
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token request: %v", err)
			}
			if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "code_1" ||
				r.Form.Get("redirect_uri") != server.URL+"/api/v1/auth/oauth/callback" ||
				r.Form.Get("client_id") != "ci" || r.Form.Get("client_secret") != "cs" {
				t.Fatalf("token form = %v", r.Form)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth-provider/token":
			_, _ = w.Write([]byte(`{"access_token":"at_1","refresh_token":"rt_1","scope":"openid profile"}`))
		case "/oauth-provider/userinfo":
			if r.Header.Get("Authorization") != "Bearer at_1" {
				http.Error(w, "missing bearer", http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"sub":"9001","preferred_username":"ada","name":"Ada","picture":"https://cdn/p.png","email":"ada@example.com"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	oauth := NewNodeLocOAuth(NodeLocOAuthConfig{
		BaseURL: server.URL, ClientID: "ci", ClientSecret: "cs", RedirectURI: server.URL + "/api/v1/auth/oauth/callback",
	}, server.Client())
	profile, err := oauth.ExchangeCode(context.Background(), "code_1")
	if err != nil {
		t.Fatalf("login with an OIDC-shaped profile: %v", err)
	}
	if profile.ProviderUID != "9001" || profile.Username != "ada" || profile.AvatarURL != "https://cdn/p.png" {
		t.Fatalf("profile = %+v", profile)
	}
	if profile.Email == nil || *profile.Email != "ada@example.com" {
		t.Fatalf("email = %+v", profile.Email)
	}
	if tokenRequests != 1 {
		t.Fatalf("token requests = %d, want exactly one", tokenRequests)
	}
}

// Some NodeLoc releases wrap the payload in `data` and answer a refusal with
// {"success": false} at HTTP 200. Both used to read as an empty profile, which
// the buyer saw as a login that silently did nothing.
func TestLoginUnwrapsDataAndReadsARefusal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth-provider/token" {
			_, _ = w.Write([]byte(`{"data": {"access_token":"at_1"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data": {"sub":"9001","picture":"https://cdn/p.png"}}`))
	}))
	defer server.Close()
	oauth := NewNodeLocOAuth(NodeLocOAuthConfig{
		BaseURL: server.URL, ClientID: "ci", ClientSecret: "cs", RedirectURI: server.URL + "/callback",
	}, server.Client())
	profile, err := oauth.ExchangeCode(context.Background(), "code_1")
	if err != nil {
		t.Fatalf("login through a data envelope: %v", err)
	}
	if profile.ProviderUID != "9001" || profile.AvatarURL != "https://cdn/p.png" {
		t.Fatalf("profile = %+v", profile)
	}

	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth-provider/token" {
			_, _ = w.Write([]byte(`{"access_token":"at_1"}`))
			return
		}
		_, _ = w.Write([]byte(`{"success": false, "message": "token expired"}`))
	}))
	defer refusing.Close()
	oauth = NewNodeLocOAuth(NodeLocOAuthConfig{
		BaseURL: refusing.URL, ClientID: "ci", ClientSecret: "cs", RedirectURI: refusing.URL + "/callback",
	}, refusing.Client())
	if _, err := oauth.ExchangeCode(context.Background(), "code_1"); !errors.Is(err, domain.ErrOAuthRejected) {
		t.Fatalf("a success:false userinfo answer = %v, want a refusal naming it", err)
	} else if !strings.Contains(err.Error(), "token expired") {
		t.Fatalf("refusal lost NodeLoc's reason: %v", err)
	}
}

// A store whose OAuth fields are half-filled keeps serving the storefront: the
// constructor no longer fails, and 用 NodeLoc 登录 answers with the field to fix.
func TestUnconfiguredLoginNamesTheMissingField(t *testing.T) {
	oauth := NewNodeLocOAuth(NodeLocOAuthConfig{BaseURL: "https://www.nodeloc.com", ClientID: "", ClientSecret: "cs"}, nil)
	_, err := oauth.AuthorizationURL("state_1")
	if !errors.Is(err, domain.ErrOAuthNotConfigured) {
		t.Fatalf("unconfigured 登录 = %v", err)
	}
	if !strings.Contains(err.Error(), "Client ID") || !strings.Contains(err.Error(), "回调地址") {
		t.Fatalf("the answer does not name the fields: %v", err)
	}
	if strings.Contains(err.Error(), "cs") {
		t.Fatalf("the secret value leaked into the diagnosis: %v", err)
	}
}

// Probed against the live forum: POST /oauth-provider/token with credentials it does not
// know answers 400 {"error":"invalid_client","error_description":"Invalid client
// credentials"}, and a path it does not have answers with the 「找不到页面」 HTML page. The
// store used to append either reply to a Chinese sentence as raw JSON, so the shop owner
// had to recognise OAuth error names themselves — and an HTML page looks like a broken
// credential when it really means 「这个域上没有 OAuth 应用」.
func TestARejectedTokenExchangeNamesTheOAuthError(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		contentType string
		body        string
		wants       []string
		doesNotWant string
	}{
		{
			name: "client 不对", status: http.StatusBadRequest, contentType: "application/json",
			body:  `{"error":"invalid_client","error_description":"Invalid client credentials"}`,
			wants: []string{"Client ID", "Invalid client credentials"}, doesNotWant: "error_description",
		},
		{
			name: "授权码用过了", status: http.StatusBadRequest, contentType: "application/json",
			body:  `{"error":"invalid_grant","error_description":"Authorization code expired"}`,
			wants: []string{"重新点一次", "Authorization code expired"}, doesNotWant: "",
		},
		{
			name: "这个域上没有 OAuth 应用", status: http.StatusNotFound, contentType: "text/html; charset=utf-8",
			body:  `<!DOCTYPE html><html><head><title>找不到页面 - NodeLoc</title></head></html>`,
			wants: []string{"网页", "OAuth"}, doesNotWant: "DOCTYPE",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			oauth := NewNodeLocOAuth(NodeLocOAuthConfig{
				BaseURL: server.URL, ClientID: "ci", ClientSecret: "cs", RedirectURI: server.URL + "/api/v1/auth/oauth/callback",
			}, server.Client())

			_, err := oauth.ExchangeCode(context.Background(), "code_1")
			if !errors.Is(err, domain.ErrOAuthRejected) {
				t.Fatalf("%v, want the provider to have answered and refused", err)
			}
			for _, want := range tc.wants {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("diagnosis = %q, want it to say %q", err.Error(), want)
				}
			}
			if tc.doesNotWant != "" && strings.Contains(err.Error(), tc.doesNotWant) {
				t.Fatalf("diagnosis = %q, want the provider's words translated, not dumped", err.Error())
			}
		})
	}
}

// 「测试 NodeLoc 登录」 reads the token endpoint, because the authorize page is not
// evidence: probed against the live forum, GET /oauth-provider/authorize redirects to
// /login even for a client_id that has never existed. So a probe that only builds the
// URL says 「配置正确」 for a reset or mistyped Secret, and the shop learns the truth
// from each buyer who logs in and then lands back on the login page.
//
// The probe redeems a code that cannot exist, which touches no account. It must be one
// POST, must not go on to userinfo, and must answer 「这组凭据 NodeLoc 认」 only for the
// one reply that proves it.
func TestProbeReadsTheTokenEndpointOnce(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Authorization code not found"}`))
	}))
	defer server.Close()
	oauth := NewNodeLocOAuth(NodeLocOAuthConfig{
		BaseURL: server.URL, ClientID: "ci", ClientSecret: "cs", RedirectURI: server.URL + "/api/v1/auth/oauth/callback",
	}, server.Client())

	outcome := oauth.Probe(context.Background())
	if outcome.Code != "" {
		t.Fatalf("an accepted client with a refused code = %q, want the healthy answer", outcome.Code)
	}
	if len(paths) != 1 || paths[0] != "POST /oauth-provider/token" {
		t.Fatalf("the probe made %v, want exactly one token POST and no userinfo", paths)
	}
	// The secret is what this test is about, so it must not come back in the answer.
	if strings.Contains(outcome.Detail, "cs") || strings.Contains(outcome.Detail, "\"ci\"") {
		t.Fatalf("the probe echoed the credentials back: %q", outcome.Detail)
	}
}

func TestProbeSeparatesEachTokenEndpointAnswer(t *testing.T) {
	cases := []struct {
		name            string
		status          int
		body            string
		contentTypeHTML bool
		wantsCode       string
		contains        string
		doesNotWant     string
	}{
		{name: "凭据对，只是编号不存在", status: http.StatusBadRequest,
			body: `{"error":"invalid_grant","error_description":"Authorization code not found"}`, wantsCode: ""},
		{name: "配对不上", status: http.StatusBadRequest,
			body:      `{"error":"invalid_client","error_description":"Invalid client credentials"}`,
			wantsCode: "client_rejected", contains: "Invalid client credentials"},
		{name: "应用没被允许走授权码", status: http.StatusBadRequest,
			body:      `{"error":"unauthorized_client","error_description":"Client not allowed"}`,
			wantsCode: "client_rejected"},
		{name: "回调地址对不上", status: http.StatusBadRequest,
			body:      `{"error":"invalid_grant","error_description":"redirect_uri mismatch"}`,
			wantsCode: "redirect_mismatch", contains: "redirect_uri mismatch"},
		{name: "这个域没有 OAuth 接口", status: http.StatusNotFound, contentTypeHTML: true,
			body:      `<!DOCTYPE html><html><head><title>找不到页面 - NodeLoc</title></head></html>`,
			wantsCode: "route_missing", doesNotWant: "DOCTYPE"},
		// The shop's own SPA answers unknown paths with HTTP 200 and HTML. Reading a
		// bare 2xx as 「endpoint is healthy」 is the exact false green this probe was
		// written to remove, so a 200 that carries a web page is still route_missing.
		{name: "商店自己的 SPA 用 200 回了网页", status: http.StatusOK, contentTypeHTML: true,
			body:      `<!DOCTYPE html><html><head><title>NodeLoc Store</title></head><body>app</body></html>`,
			wantsCode: "route_missing", doesNotWant: "DOCTYPE"},
		// A 200 that still reports invalid_grant is the forum answering in a shape the
		// probe cannot treat as proof — the credentials were never confirmed.
		{name: "200 里报 invalid_grant", status: http.StatusOK,
			body:      `{"error":"invalid_grant","error_description":"Authorization code not found"}`,
			wantsCode: "rejected"},
		{name: "论坛不认这种授权方式", status: http.StatusBadRequest,
			body:      `{"error":"unsupported_grant_type","error_description":"Refresh tokens are not supported"}`,
			wantsCode: "grant_unsupported"},
		{name: "scope 没过审", status: http.StatusBadRequest,
			body: `{"error":"invalid_scope","error_description":"Scope email not allowed"}`, wantsCode: "scope_rejected"},
		{name: "读不懂的应答", status: http.StatusBadGateway, body: `gateway gave up`, wantsCode: "rejected",
			contains: "gateway gave up"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.contentTypeHTML {
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
				} else {
					w.Header().Set("Content-Type", "application/json")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			oauth := NewNodeLocOAuth(NodeLocOAuthConfig{
				BaseURL: server.URL, ClientID: "ci", ClientSecret: "cs", RedirectURI: server.URL + "/cb",
			}, server.Client())
			outcome := oauth.Probe(context.Background())
			if outcome.Code != tc.wantsCode {
				t.Fatalf("code = %q, want %q (%s)", outcome.Code, tc.wantsCode, outcome.Detail)
			}
			if tc.contains != "" && !strings.Contains(outcome.Detail, tc.contains) {
				t.Fatalf("the answer lost NodeLoc's own words: %q", outcome.Detail)
			}
			if tc.doesNotWant != "" && strings.Contains(outcome.Detail, tc.doesNotWant) {
				t.Fatalf("a whole web page was echoed: %q", outcome.Detail)
			}
		})
	}
}

// A store that never finished configuring OAuth gets its own answer, and an outage gets
// a different one from a wrong secret — 「连不上」 and 「凭据不对」 are fixed by two
// different people.
func TestProbeNamesTheStoreOwnFaultsApart(t *testing.T) {
	unconfigured := NewNodeLocOAuth(NodeLocOAuthConfig{BaseURL: "https://www.nodeloc.com", ClientID: "", ClientSecret: ""}, nil)
	if outcome := unconfigured.Probe(context.Background()); outcome.Code != "not_configured" {
		t.Fatalf("unconfigured probe = %q, want not_configured", outcome.Code)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	oauth := NewNodeLocOAuth(NodeLocOAuthConfig{
		BaseURL: server.URL, ClientID: "ci", ClientSecret: "cs", RedirectURI: server.URL + "/cb",
	}, server.Client())
	server.Close()
	if outcome := oauth.Probe(context.Background()); outcome.Code != "unreachable" {
		t.Fatalf("closed endpoint = %q, want unreachable (%s)", outcome.Code, outcome.Detail)
	}
}
