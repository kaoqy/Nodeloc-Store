package contract

import (
	"context"

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
	List(ctx context.Context, limit, offset int) ([]*domain.User, int64, error)

	CreateOAuthIdentity(ctx context.Context, identity *domain.OAuthIdentity) error
	UpdateOAuthIdentity(ctx context.Context, identity *domain.OAuthIdentity) error
	FindOAuthIdentity(ctx context.Context, provider, providerUID string) (*domain.OAuthIdentity, error)
	FindOAuthIdentityByUser(ctx context.Context, userID uint, provider string) (*domain.OAuthIdentity, error)
	DeleteOAuthIdentity(ctx context.Context, userID uint, provider string) error
	CountOAuthIdentities(ctx context.Context, userID uint) (int64, error)
}

// OAuthProvider abstracts authorization URL generation, NodeLoc callback
// validation, authorization-code exchange, and profile retrieval.
type OAuthProvider interface {
	Name() string
	AuthorizationURL(state string) (string, error)
	VerifyCallback(params map[string]string) bool
	ExchangeCode(ctx context.Context, code string) (*domain.OAuthProfile, error)
}

// TokenService creates and validates application access and refresh tokens.
type TokenService interface {
	Issue(ctx context.Context, user *domain.User) (*domain.TokenPair, error)
	Parse(ctx context.Context, token string) (*domain.TokenClaims, error)
	Refresh(ctx context.Context, refreshToken string) (*domain.TokenPair, error)
}
