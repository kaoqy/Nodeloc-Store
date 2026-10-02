package contract

import (
	"context"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/domain"
)

// UserRepo is the persistence port for users and their OAuth identities.
type UserRepo interface {
	Create(ctx context.Context, user *domain.User) error
	Update(ctx context.Context, user *domain.User) error
	FindByID(ctx context.Context, id uint) (*domain.User, error)
	FindByUsername(ctx context.Context, username string) (*domain.User, error)
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
	FindByOAuth(ctx context.Context, provider, providerUID string) (*domain.User, error)
	UsernameExists(ctx context.Context, username string) (bool, error)
	EmailExists(ctx context.Context, email string) (bool, error)
	List(ctx context.Context, limit, offset int, search string) ([]*domain.User, int64, error)

	CreateOAuthIdentity(ctx context.Context, identity *domain.OAuthIdentity) error
	UpdateOAuthIdentity(ctx context.Context, identity *domain.OAuthIdentity) error
	FindOAuthIdentity(ctx context.Context, provider, providerUID string) (*domain.OAuthIdentity, error)
	FindOAuthIdentityByUser(ctx context.Context, userID uint, provider string) (*domain.OAuthIdentity, error)
	DeleteOAuthIdentity(ctx context.Context, userID uint, provider string) error
	CountOAuthIdentities(ctx context.Context, userID uint) (int64, error)
	CreateOAuthTransaction(ctx context.Context, transaction *domain.OAuthTransaction) error
	ConsumeOAuthTransaction(ctx context.Context, stateHash string, now time.Time) (*domain.OAuthTransaction, error)
	// RecordOAuthAttempt keeps one NodeLoc 登录 round trip readable after the
	// fact. The shop's own log is out of reach for the owner of a Docker
	// container, and 「登录不了」 with no step attached is not a bug report.
	RecordOAuthAttempt(ctx context.Context, attempt *domain.OAuthAttempt) error
	// ListOAuthAttempts reads the most recent round trips back, newest first.
	ListOAuthAttempts(ctx context.Context, limit int) ([]domain.OAuthAttempt, error)

	// RecordCheckin writes the day's check-in, its ledger row and the user's
	// counters in one transaction. The ledger's unique reference is what stops a
	// double-tapped 签到 from paying twice.
	RecordCheckin(ctx context.Context, user *domain.User, checkin *domain.CheckIn, entry *domain.PointEntry) error
	// AdjustPoints moves the balance and appends its ledger row atomically, so
	// the history always explains the current number.
	AdjustPoints(ctx context.Context, user *domain.User, entry *domain.PointEntry) error
	ListPoints(ctx context.Context, userID uint, limit, offset int) ([]domain.PointEntry, int64, error)
	ListCheckins(ctx context.Context, userID uint, limit int) ([]domain.CheckIn, error)
}

// OAuthProvider abstracts authorization URL generation, NodeLoc callback
// validation, authorization-code exchange, and profile retrieval.
type OAuthProvider interface {
	Name() string
	AuthorizationURL(state string) (string, error)
	VerifyCallback(params map[string]string) bool
	ExchangeCode(ctx context.Context, code string) (*domain.OAuthProfile, error)
	// FetchProfile re-reads the account behind an access token, so a buyer can
	// pull a fresh 头像/邮箱 from NodeLoc without signing out and back in.
	FetchProfile(ctx context.Context, accessToken string) (*domain.OAuthProfile, error)
	// Probe asks the token endpoint one question that costs nobody a login: are
	// these Client ID / Client Secret a pair this application knows?
	Probe(ctx context.Context) OAuthProbe
}

// OAuthProbe is one 「测试 NodeLoc 登录」 answer, classified by the module that
// spoke to NodeLoc instead of by the settings page re-reading English error
// strings. Code is empty when NodeLoc accepted the credentials.
type OAuthProbe struct {
	Code    string
	Message string
	Detail  string
}

// TokenService creates and validates application access and refresh tokens.
type TokenService interface {
	Issue(ctx context.Context, user *domain.User) (*domain.TokenPair, error)
	Parse(ctx context.Context, token string) (*domain.TokenClaims, error)
	// ParseRefresh validates a refresh token and returns its claims. Issuing
	// the new pair is left to the caller, which re-reads the account first so a
	// disabled or demoted one cannot keep renewing on its old claims.
	ParseRefresh(ctx context.Context, token string) (*domain.TokenClaims, error)
}
