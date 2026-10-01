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
func TestLoginReadsAnOIDCShapedProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
