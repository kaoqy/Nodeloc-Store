package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/domain"
)

var ErrInvalidToken = errors.New("invalid token")

type JWTConfig struct {
	Secret     string
	Issuer     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

type JWTService struct {
	secret     []byte
	issuer     string
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time
}

type jwtClaims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	IsAdmin  bool   `json:"is_admin"`
	Type     string `json:"type"`
	jwt.RegisteredClaims
}

func NewJWTService(config JWTConfig) (*JWTService, error) {
	if strings.TrimSpace(config.Secret) == "" {
		return nil, errors.New("jwt secret is required")
	}
	if config.AccessTTL <= 0 {
		config.AccessTTL = 2 * time.Hour
	}
	if config.RefreshTTL <= 0 {
		config.RefreshTTL = 7 * 24 * time.Hour
	}
	if strings.TrimSpace(config.Issuer) == "" {
		config.Issuer = "nodeloc-store"
	}
	return &JWTService{
		secret:     []byte(config.Secret),
		issuer:     config.Issuer,
		accessTTL:  config.AccessTTL,
		refreshTTL: config.RefreshTTL,
		now:        time.Now,
	}, nil
}

func (s *JWTService) Issue(_ context.Context, user *domain.User) (*domain.TokenPair, error) {
	if user == nil || user.ID == 0 {
		return nil, domain.ErrInvalidInput
	}

	now := s.now().UTC()
	accessExpiresAt := now.Add(s.accessTTL)
	accessToken, err := s.sign(user, "access", now, accessExpiresAt)
	if err != nil {
		return nil, err
	}
	refreshToken, err := s.sign(user, "refresh", now, now.Add(s.refreshTTL))
	if err != nil {
		return nil, err
	}

	return &domain.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresAt:    accessExpiresAt,
	}, nil
}

func (s *JWTService) Parse(_ context.Context, tokenString string) (*domain.TokenClaims, error) {
	claims, err := s.parse(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.Type != "access" {
		return nil, ErrInvalidToken
	}
	return claimsToDomain(claims), nil
}

func (s *JWTService) Refresh(_ context.Context, refreshToken string) (*domain.TokenPair, error) {
	claims, err := s.parse(refreshToken)
	if err != nil {
		return nil, err
	}
	if claims.Type != "refresh" {
		return nil, ErrInvalidToken
	}

	user := &domain.User{
		ID:       claims.UserID,
		Username: claims.Username,
		Role:     claims.Role,
		IsAdmin:  claims.IsAdmin,
		IsActive: true,
	}
	return s.Issue(context.Background(), user)
}

func (s *JWTService) sign(user *domain.User, tokenType string, issuedAt, expiresAt time.Time) (string, error) {
	claims := jwtClaims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		IsAdmin:  user.IsAdmin,
		Type:     tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   fmt.Sprintf("%d", user.ID),
			ID:        uuid.NewString(),
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			NotBefore: jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

func (s *JWTService) parse(tokenString string) (*jwtClaims, error) {
	tokenString = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(tokenString), "Bearer "))
	if tokenString == "" {
		return nil, ErrInvalidToken
	}

	claims := &jwtClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidToken
		}
		return s.secret, nil
	}, jwt.WithIssuer(s.issuer), jwt.WithExpirationRequired(), jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !token.Valid || claims.UserID == 0 {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func claimsToDomain(claims *jwtClaims) *domain.TokenClaims {
	return &domain.TokenClaims{
		UserID:   claims.UserID,
		Username: claims.Username,
		Role:     claims.Role,
		IsAdmin:  claims.IsAdmin,
		Type:     claims.Type,
	}
}
