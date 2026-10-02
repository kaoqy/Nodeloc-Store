package application

import (
	"context"
	"strings"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/domain"
)

// attemptSpy is the one half of the identity store a login record touches. The
// rest of UserRepo comes along by embedding the interface: this test never calls
// it, and a call would panic loudly rather than read a fake's default.
type attemptSpy struct {
	contract.UserRepo
	recorded []domain.OAuthAttempt
}

func (s *attemptSpy) RecordOAuthAttempt(_ context.Context, attempt *domain.OAuthAttempt) error {
	s.recorded = append(s.recorded, *attempt)
	return nil
}

func (s *attemptSpy) ListOAuthAttempts(context.Context, int) ([]domain.OAuthAttempt, error) {
	return s.recorded, nil
}

// stubProvider and stubTokens exist to pass NewService's dependency check.
// Writing a login record never walks the OAuth round trip, and a call into
// either one panics instead of quietly returning a zero value.
type (
	stubProvider struct{ contract.OAuthProvider }
	stubTokens   struct{ contract.TokenService }
)

func newAttemptService(t *testing.T, spy *attemptSpy) *Service {
	t.Helper()
	service, err := NewService(spy, stubProvider{}, stubTokens{}, config.FeaturesConfig{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

func TestLogOAuthAttemptKeepsCredentialsOutOfTheShopRecords(t *testing.T) {
	spy := &attemptSpy{}
	service := newAttemptService(t, spy)

	service.LogOAuthAttempt(context.Background(), OAuthAttemptLog{
		Step:    "callback",
		Outcome: "failed",
		Reason:  "rejected",
		Detail:  "换取令牌失败（error=invalid_grant, error_description=authorization code expired, code=abc123secret, access_token=AT, client_secret=CS）",
	})
	if len(spy.recorded) != 1 {
		t.Fatalf("recorded %d attempts, want one", len(spy.recorded))
	}
	detail := spy.recorded[0].Detail
	for _, leaked := range []string{"abc123secret", "access_token=AT", "client_secret=CS"} {
		if strings.Contains(detail, leaked) {
			t.Errorf("detail kept %q readable, i.e. unredacted: %q", leaked, detail)
		}
	}
	if !strings.Contains(detail, "error=invalid_grant") || !strings.Contains(detail, "error_description=authorization code expired") {
		t.Errorf("detail lost provider diagnostics: %q", detail)
	}
	if !strings.Contains(detail, "code=[已隐藏]") || !strings.Contains(detail, "access_token=[已隐藏]") || !strings.Contains(detail, "client_secret=[已隐藏]") {
		t.Errorf("detail was not scrubbed: %q", detail)
	}
	if !strings.Contains(detail, "invalid_grant") {
		t.Errorf("detail lost the part the owner needs to read: %q", detail)
	}
}

// A credential-bearing authorization URL must not reach the table either, and a
// long provider sentence is cut instead of overflowing the column.
func TestLogOAuthAttemptClipsTheCallbackAndTheLongDetail(t *testing.T) {
	spy := &attemptSpy{}
	service := newAttemptService(t, spy)

	service.LogOAuthAttempt(context.Background(), OAuthAttemptLog{
		Step:     "initiate",
		Outcome:  "started",
		Redirect: "https://shop.example.com/api/v1/auth/oauth/callback",
		Detail:   strings.Repeat("链", 900),
	})

	recorded := spy.recorded[0]
	if recorded.RedirectURI != "https://shop.example.com/api/v1/auth/oauth/callback" {
		t.Errorf("redirect = %q, want the callback the shop sent", recorded.RedirectURI)
	}
	if runes := []rune(recorded.Detail); len(runes) != 500 {
		t.Errorf("detail kept %d runes, want it clipped to 500", len(runes))
	}
}
