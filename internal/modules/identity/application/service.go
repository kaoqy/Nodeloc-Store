package application

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/domain"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo       contract.UserRepo
	oauth      contract.OAuthProvider
	tokens     contract.TokenService
	features   config.FeaturesConfig
	bcryptCost int
	now        func() time.Time
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

func NewService(repo contract.UserRepo, oauth contract.OAuthProvider, tokens contract.TokenService, features config.FeaturesConfig) (*Service, error) {
	if repo == nil || oauth == nil || tokens == nil {
		return nil, errors.New("identity service dependencies are required")
	}
	return &Service{repo: repo, oauth: oauth, tokens: tokens, features: features, bcryptCost: bcrypt.DefaultCost, now: time.Now}, nil
}

// CheckinEnabled reports whether 签到 is switched on for this shop.
func (s *Service) CheckinEnabled() bool { return s.features.CheckinOn() }

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

// ProbeOAuth asks NodeLoc's token endpoint whether the stored credentials are a pair
// it knows. The 设置 page reads the code, so it never has to re-interpret an English
// error string — and never has to pretend that a well-formed authorization URL means
// the login will work.
func (s *Service) ProbeOAuth(ctx context.Context) contract.OAuthProbe {
	return s.oauth.Probe(ctx)
}

func (s *Service) InitiateOAuth(state string) (string, string, error) {
	if !s.features.OAuthOn() {
		return "", "", domain.ErrOAuthDisabled
	}
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

// Refresh trades a refresh token for a new pair. The account is re-read rather
// than replayed from the old claims: a demotion has to shorten what the next
// access token says, and a disabled account must not renew at all.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (*OAuthResult, error) {
	claims, err := s.tokens.ParseRefresh(ctx, refreshToken)
	if err != nil {
		return nil, domain.ErrInvalidCredentials
	}
	user, err := s.repo.FindByID(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil, domain.ErrInvalidCredentials
		}
		return nil, err
	}
	if !user.IsActive {
		return nil, domain.ErrInactiveUser
	}
	tokens, err := s.tokens.Issue(ctx, user)
	if err != nil {
		return nil, err
	}
	return &OAuthResult{User: user, Tokens: tokens}, nil
}

// AdminListUsers pages the user directory for the back office.
func (s *Service) AdminListUsers(ctx context.Context, limit, offset int, search string) ([]*domain.User, int64, error) {
	return s.repo.List(ctx, limit, offset, search)
}

func (s *Service) AdminGetUser(ctx context.Context, userID uint) (*domain.User, error) {
	if userID == 0 {
		return nil, domain.ErrInvalidInput
	}
	return s.repo.FindByID(ctx, userID)
}

// AdminSetRole changes a user's role. IsAdmin is kept in sync because both the
// JWT claims and the bootstrap check read it. Self-demotion is refused so an
// admin can never lock themselves out of the back office.
//
// A plain admin may only hand out 客服/运营 or send someone back to 普通用户;
// granting admin or super_admin belongs to super_admin, otherwise one leaked
// admin session is enough to mint a new owner.
func (s *Service) AdminSetRole(ctx context.Context, actorID uint, actorRole string, userID uint, role string) (*domain.User, error) {
	role = strings.TrimSpace(role)
	if actorID == userID {
		return nil, fmt.Errorf("%w: 不能修改自己的角色，请让另一位超级管理员来调整", domain.ErrInvalidInput)
	}
	if !domain.ValidRole(role) {
		return nil, fmt.Errorf("%w: 未知的角色 %q", domain.ErrInvalidInput, role)
	}
	if actorRole != "super_admin" && (role == "admin" || role == "super_admin") {
		return nil, fmt.Errorf("%w: 只有超级管理员才能授予该角色", domain.ErrInvalidInput)
	}
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if actorRole != "super_admin" && domain.IsStaff(user.Role) {
		return nil, fmt.Errorf("%w: 只有超级管理员才能调整后台账号的角色", domain.ErrInvalidInput)
	}
	if err := user.SetRole(role); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// AdminToggleAdmin flips an account between 普通用户 and 管理员. The finer
// staff roles are set through AdminSetRole from the role picker.
func (s *Service) AdminToggleAdmin(ctx context.Context, actorID uint, actorRole string, userID uint) (*domain.User, error) {
	next := "admin"
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.IsAdmin || user.Role != "user" {
		next = "user"
	}
	return s.AdminSetRole(ctx, actorID, actorRole, userID, next)
}

func (s *Service) AdminToggleActive(ctx context.Context, actorID, userID uint) (*domain.User, error) {
	if actorID == userID {
		return nil, fmt.Errorf("%w: 不能停用自己的账号", domain.ErrInvalidInput)
	}
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	user.IsActive = !user.IsActive
	if err := s.repo.Update(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// AdminAdjustPoints moves a user's point balance, never below zero, and writes
// the reason to the ledger so the balance always explains itself.
func (s *Service) AdminAdjustPoints(ctx context.Context, actorID, userID uint, delta int) (*domain.User, error) {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.adjustPoints(ctx, user, delta, "管理员调整", &actorID)
}

func (s *Service) adjustPoints(ctx context.Context, user *domain.User, delta int, reason string, actorID *uint) (*domain.User, error) {
	if delta == 0 {
		return user, nil
	}
	balance := user.Points + delta
	if balance < 0 {
		return nil, fmt.Errorf("%w: 要扣的积分比账号现有的还多，当前余额 %d 分", domain.ErrInvalidInput, user.Points)
	}
	user.Points = balance
	entry := &domain.PointEntry{
		UserID:        user.ID,
		Delta:         delta,
		BalanceAfter:  balance,
		Reason:        reason,
		ReferenceType: "admin_adjust",
		// The nonce keeps every deliberate adjustment a distinct ledger row;
		// retrying the same click is a new change, not a duplicate to suppress.
		ReferenceID: fmt.Sprintf("%d:%d:%d", user.ID, s.now().UnixNano(), delta),
		ActorID:     actorID,
	}
	if err := s.repo.AdjustPoints(ctx, user, entry); err != nil {
		return nil, err
	}
	return user, nil
}

// CheckIn awards today's 签到 points. It answers with the day's row and the
// updated account so the storefront can show the streak without a second call.
func (s *Service) CheckIn(ctx context.Context, userID uint) (*domain.CheckIn, *domain.User, error) {
	if !s.features.CheckinOn() {
		return nil, nil, domain.ErrCheckinDisabled
	}
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	checkin, err := user.Checkin(s.now())
	if err != nil {
		return nil, nil, err
	}
	entry := &domain.PointEntry{
		UserID:        user.ID,
		Delta:         checkin.RewardPoints,
		BalanceAfter:  user.Points,
		Reason:        "每日签到",
		ReferenceType: "checkin",
		// One reference per account per calendar day: the unique index on the
		// ledger makes a racing second request pay nothing.
		ReferenceID: fmt.Sprintf("%d:%s", user.ID, checkin.CheckinDate.Format("2006-01-02")),
	}
	if err := s.repo.RecordCheckin(ctx, user, checkin, entry); err != nil {
		return nil, nil, err
	}
	return checkin, user, nil
}

// CheckinStatus is what the profile page shows before anyone taps the button.
type CheckinStatus struct {
	Enabled         bool `json:"enabled"`
	CheckedInToday  bool `json:"checked_in_today"`
	ConsecutiveDays int  `json:"consecutive_days"`
	TotalCheckins   int  `json:"total_checkins"`
	Points          int  `json:"points"`
}

func (s *Service) CheckinStatus(ctx context.Context, userID uint) (*CheckinStatus, error) {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &CheckinStatus{
		Enabled:         s.features.CheckinOn(),
		CheckedInToday:  user.HasCheckedInToday(s.now()),
		ConsecutiveDays: user.ConsecutiveDays,
		TotalCheckins:   user.TotalCheckins,
		Points:          user.Points,
	}, nil
}

// Points pages the ledger, newest first.
func (s *Service) Points(ctx context.Context, userID uint, limit, offset int) ([]domain.PointEntry, int64, error) {
	return s.repo.ListPoints(ctx, userID, limit, offset)
}

// Checkins returns the recent check-in history for the streak strip.
func (s *Service) Checkins(ctx context.Context, userID uint, limit int) ([]domain.CheckIn, error) {
	return s.repo.ListCheckins(ctx, userID, limit)
}

// UpdateProfile applies the fields a buyer may edit themselves. Email changes
// are refused while a NodeLoc login owns the address, so the identity synced
// from the forum stays the one source of truth.
func (s *Service) UpdateProfile(ctx context.Context, userID uint, edit domain.ProfileEdit) (*domain.User, error) {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if edit.Email != nil {
		if user.OAuthProvider != nil && user.OAuthHasEmail {
			return nil, fmt.Errorf("%w: 邮箱来自 NodeLoc 登录，请在论坛修改后重新同步", domain.ErrInvalidInput)
		}
		email := strings.ToLower(strings.TrimSpace(*edit.Email))
		if email == "" {
			return nil, fmt.Errorf("%w: 邮箱不能为空", domain.ErrInvalidInput)
		}
		exists, err := s.repo.EmailExists(ctx, email)
		if err != nil {
			return nil, err
		}
		if exists && (user.Email == nil || !strings.EqualFold(*user.Email, email)) {
			return nil, domain.ErrEmailTaken
		}
	}
	if err := user.ApplyProfile(edit); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// SyncOAuthProfile pulls the current NodeLoc profile onto the bound identity,
// so 头像/昵称/邮箱/论坛 ID stay in step without a re-login. The stored access
// token is what authorizes the read; when the provider rejects it the caller
// has to bind again, which ErrInvalidCredentials tells the SPA.
func (s *Service) SyncOAuthProfile(ctx context.Context, userID uint) (*domain.User, error) {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.OAuthProvider == nil || user.OAuthUID == nil {
		return nil, domain.ErrNotBound
	}
	identity, err := s.repo.FindOAuthIdentityByUser(ctx, userID, *user.OAuthProvider)
	if err != nil {
		return nil, err
	}
	if identity.AccessToken == nil || strings.TrimSpace(*identity.AccessToken) == "" {
		return nil, fmt.Errorf("%w: 这次绑定没有保存访问令牌，请解绑后重新绑定", domain.ErrInvalidCredentials)
	}
	profile, err := s.oauth.FetchProfile(ctx, *identity.AccessToken)
	if err != nil {
		return nil, err
	}
	// The provider could answer for a different account if a token were ever
	// swapped; refuse to move one person's profile onto another.
	if profile.ProviderUID != *user.OAuthUID {
		return nil, fmt.Errorf("%w: 令牌对应的论坛账号与当前绑定不一致", domain.ErrInvalidCredentials)
	}
	identity.ApplyProfile(*profile)
	if err := s.repo.UpdateOAuthIdentity(ctx, identity); err != nil {
		return nil, err
	}
	user.ApplyOAuthProfile(*profile)
	if err := s.repo.Update(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
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
