package application

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/domain"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo     contract.UserRepo
	oauth    contract.OAuthProvider
	tokens   contract.TokenService
	bcryptCost int
	now      func() time.Time
}

type RegisterInput struct {
	Username string
	Email    *string
	Password string
}

type LoginInput struct {
	Identifier string
	Password   string
}

type OAuthResult struct {
	User   *domain.User      `json:"user"`
	Tokens *domain.TokenPair `json:"tokens"`
}

func NewService(repo contract.UserRepo, oauth contract.OAuthProvider, tokens contract.TokenService) (*Service, error) {
	if repo == nil || oauth == nil || tokens == nil {
		return nil, errors.New("identity service dependencies are required")
	}
	return &Service{repo: repo, oauth: oauth, tokens: tokens, bcryptCost: bcrypt.DefaultCost, now: time.Now}, nil
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (*OAuthResult, error) {
	username := strings.TrimSpace(input.Username)
	password := input.Password
	if username == "" || len(username) > 64 || len(password) < 8 || len(password) > 72 {
		return nil, domain.ErrInvalidInput
	}

	exists, err := s.repo.UsernameExists(ctx, username)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, domain.ErrUsernameTaken
	}

	if input.Email != nil {
		email := strings.ToLower(strings.TrimSpace(*input.Email))
		if email == "" || len(email) > 190 || !strings.Contains(email, "@") {
			return nil, domain.ErrInvalidInput
		}
		input.Email = &email
		exists, err = s.repo.EmailExists(ctx, email)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, domain.ErrEmailTaken
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	hashString := string(hash)
	user, err := domain.NewUser(username, input.Email, &hashString)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}
	tokens, err := s.tokens.Issue(ctx, user)
	if err != nil {
		return nil, err
	}
	return &OAuthResult{User: user, Tokens: tokens}, nil
}

func (s *Service) Login(ctx context.Context, input LoginInput) (*OAuthResult, error) {
	identifier := strings.TrimSpace(input.Identifier)
	if identifier == "" || input.Password == "" {
		return nil, domain.ErrInvalidCredentials
	}

	var user *domain.User
	var err error
	if strings.Contains(identifier, "@") {
		user, err = s.repo.FindByEmail(ctx, identifier)
	} else {
		user, err = s.repo.FindByUsername(ctx, identifier)
	}
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil, domain.ErrInvalidCredentials
		}
		return nil, err
	}
	if !user.IsActive {
		return nil, domain.ErrInactiveUser
	}
	if user.PasswordHash == nil || bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(input.Password)) != nil {
		return nil, domain.ErrInvalidCredentials
	}

	now := s.now().UTC()
	user.LastLoginAt = &now
	if err := s.repo.Update(ctx, user); err != nil {
		return nil, err
	}
	tokens, err := s.tokens.Issue(ctx, user)
	if err != nil {
		return nil, err
	}
	return &OAuthResult{User: user, Tokens: tokens}, nil
}

func (s *Service) InitiateOAuth(state string) (string, string, error) {
	state = strings.TrimSpace(state)
	if state == "" {
		buffer := make([]byte, 32)
		if _, err := rand.Read(buffer); err != nil {
			return "", "", fmt.Errorf("generate oauth state: %w", err)
		}
		state = base64.RawURLEncoding.EncodeToString(buffer)
	}
	url, err := s.oauth.AuthorizationURL(state)
	if err != nil {
		return "", "", err
	}
	return url, state, nil
}

func (s *Service) OAuthLogin(ctx context.Context, code string, callbackParams map[string]string) (*OAuthResult, error) {
	if !s.oauth.VerifyCallback(callbackParams) {
		return nil, domain.ErrInvalidCredentials
	}
	profile, err := s.oauth.ExchangeCode(ctx, code)
	if err != nil {
		return nil, err
	}

	user, err := s.repo.FindByOAuth(ctx, profile.Provider, profile.ProviderUID)
	if errors.Is(err, domain.ErrUserNotFound) {
		user, err = s.createOAuthUser(ctx, *profile)
	} else if err == nil {
		identity, findErr := s.repo.FindOAuthIdentity(ctx, profile.Provider, profile.ProviderUID)
		if findErr != nil {
			return nil, findErr
		}
		identity.ApplyProfile(*profile)
		if err = s.repo.UpdateOAuthIdentity(ctx, identity); err != nil {
			return nil, err
		}
		user.ApplyOAuthProfile(*profile)
	} else {
		return nil, err
	}
	if !user.IsActive {
		return nil, domain.ErrInactiveUser
	}
	now := s.now().UTC()
	user.LastLoginAt = &now
	if err := s.repo.Update(ctx, user); err != nil {
		return nil, err
	}
	tokens, err := s.tokens.Issue(ctx, user)
	if err != nil {
		return nil, err
	}
	return &OAuthResult{User: user, Tokens: tokens}, nil
}

func (s *Service) BindOAuth(ctx context.Context, userID uint, code string, callbackParams map[string]string) (*domain.User, error) {
	if userID == 0 || !s.oauth.VerifyCallback(callbackParams) {
		return nil, domain.ErrInvalidCredentials
	}
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	profile, err := s.oauth.ExchangeCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if existing, findErr := s.repo.FindOAuthIdentity(ctx, profile.Provider, profile.ProviderUID); findErr == nil {
		if existing.UserID != userID {
			return nil, domain.ErrIdentityAlreadyBound
		}
		existing.ApplyProfile(*profile)
		if err := s.repo.UpdateOAuthIdentity(ctx, existing); err != nil {
			return nil, err
		}
	} else if !errors.Is(findErr, domain.ErrIdentityNotFound) {
		return nil, findErr
	} else {
		identity, err := domain.NewOAuthIdentity(userID, *profile)
		if err != nil {
			return nil, err
		}
		if err := s.repo.CreateOAuthIdentity(ctx, identity); err != nil {
			return nil, err
		}
	}
	user.ApplyOAuthProfile(*profile)
	if err := s.repo.Update(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Service) UnbindOAuth(ctx context.Context, userID uint) (*domain.User, error) {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	count, err := s.repo.CountOAuthIdentities(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.PasswordHash == nil && count <= 1 {
		return nil, domain.ErrLastLoginMethod
	}
	if err := s.repo.DeleteOAuthIdentity(ctx, userID, s.oauth.Name()); err != nil {
		return nil, err
	}
	user.ClearOAuthProfile()
	if err := s.repo.Update(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Service) Me(ctx context.Context, userID uint) (*domain.User, error) {
	return s.repo.FindByID(ctx, userID)
}

func (s *Service) Authenticate(ctx context.Context, token string) (*domain.TokenClaims, error) {
	return s.tokens.Parse(ctx, token)
}

func (s *Service) createOAuthUser(ctx context.Context, profile domain.OAuthProfile) (*domain.User, error) {
	base := strings.TrimSpace(profile.Username)
	if base == "" {
		base = "nodeloc-" + profile.ProviderUID
	}
	if len(base) > 56 {
		base = base[:56]
	}
	username := base
	for suffix := 2; ; suffix++ {
		exists, err := s.repo.UsernameExists(ctx, username)
		if err != nil {
			return nil, err
		}
		if !exists {
			break
		}
		username = fmt.Sprintf("%s-%d", base, suffix)
		if len(username) > 64 {
			return nil, domain.ErrUsernameTaken
		}
	}

	email := profile.Email
	if email != nil {
		exists, err := s.repo.EmailExists(ctx, *email)
		if err != nil {
			return nil, err
		}
		if exists {
			email = nil
		}
	}
	user, err := domain.NewUser(username, email, nil)
	if err != nil {
		return nil, err
	}
	user.ApplyOAuthProfile(profile)
	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}
	identity, err := domain.NewOAuthIdentity(user.ID, profile)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateOAuthIdentity(ctx, identity); err != nil {
		return nil, err
	}
	return user, nil
}
