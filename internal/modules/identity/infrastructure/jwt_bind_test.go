package infrastructure

import (
	"context"
	"testing"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/domain"
)

func newBindTestJWT(t *testing.T) *JWTService {
	t.Helper()
	service, err := NewJWTService(JWTConfig{Secret: "bind-test-secret", Issuer: "nodeloc-store-test"})
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}
	return service
}

// 绑定 starts with a top-level browser navigation, so the account has to ride on
// a token the redirect itself can carry. That token must name the account and must
// not be usable as a session.
func TestBindTokenNamesTheAccountAndIsNotASession(t *testing.T) {
	service := newBindTestJWT(t)
	user := &domain.User{ID: 7, Username: "buyer", Role: "user", IsActive: true}

	token, err := service.IssueBind(context.Background(), user)
	if err != nil {
		t.Fatalf("IssueBind: %v", err)
	}
	claims, err := service.ParseBind(context.Background(), token)
	if err != nil {
		t.Fatalf("ParseBind: %v", err)
	}
	if claims.UserID != 7 {
		t.Fatalf("bind claims UserID = %d, want 7", claims.UserID)
	}
	if claims.Type != "bind" {
		t.Fatalf("bind token type = %q, want bind", claims.Type)
	}
	// The session middleware only accepts "access", so a leaked bind token cannot
	// be replayed as a login.
	if _, err := service.Parse(context.Background(), token); err == nil {
		t.Fatal("a bind token was accepted as a session token")
	}
}

// And the reverse: an ordinary session token must not be able to start a binding
// for its account through the public bind route.
func TestSessionTokenCannotBeUsedAsABindToken(t *testing.T) {
	service := newBindTestJWT(t)
	user := &domain.User{ID: 7, Username: "buyer", Role: "user", IsActive: true}

	pair, err := service.Issue(context.Background(), user)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := service.ParseBind(context.Background(), pair.AccessToken); err == nil {
		t.Fatal("an access token was accepted as a bind token")
	}
	if _, err := service.ParseBind(context.Background(), pair.RefreshToken); err == nil {
		t.Fatal("a refresh token was accepted as a bind token")
	}
	if _, err := service.IssueBind(context.Background(), nil); err == nil {
		t.Fatal("a bind token was issued for no account")
	}
}

// The bind window is short on purpose; this pins that it is not accidentally a
// session-length window or already expired.
func TestBindWindowOutlivesTheConsentScreenButNotMuchElse(t *testing.T) {
	service := newBindTestJWT(t)
	if service.bindTTL < 5*time.Minute || service.bindTTL > 30*time.Minute {
		t.Fatalf("bind TTL = %s, want a short one-off window", service.bindTTL)
	}
}
