package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
)

var (
	ErrUserNotFound         = errors.New("user not found")
	ErrIdentityNotFound     = errors.New("oauth identity not found")
	ErrUsernameTaken        = errors.New("username is already in use")
	ErrEmailTaken           = errors.New("email is already in use")
	ErrIdentityAlreadyBound = errors.New("oauth identity is already bound")
	ErrInvalidCredentials   = errors.New("invalid credentials")
	ErrInactiveUser         = errors.New("user account is inactive")
	ErrInvalidInput         = errors.New("invalid input")
	ErrLastLoginMethod      = errors.New("cannot remove the last login method")
	ErrAlreadyCheckedIn     = errors.New("already checked in today")
	ErrCheckinDisabled      = errors.New("check-in is disabled")
	ErrNotBound             = errors.New("no oauth account is bound")
	// ErrOAuthNotConfigured is the store's own answer, not NodeLoc's: the 设置 page
	// is missing a field the login round trip needs. It names them, so the shop
	// owner fixes one setting instead of watching 登录 do nothing.
	ErrOAuthNotConfigured = errors.New("NodeLoc 登录还没有配置完整")
	// ErrOAuthDisabled is the owner's 「启用 NodeLoc OAuth 登录」 switch turned off.
	// Distinct from 没配置: the credentials are there, the shop just closed that
	// door, and the login page has to say so instead of blaming the buyer.
	ErrOAuthDisabled = errors.New("本店已暂停 NodeLoc 登录，请改用账号密码登录")
	// ErrOAuthRejected means NodeLoc answered and refused; ErrOAuthUnreachable
	// means it never answered. They are two different fixes — wrong credentials
	// versus no egress — and the login page has to say which.
	ErrOAuthRejected    = errors.New("NodeLoc 拒绝了这次登录请求")
	ErrOAuthUnreachable = errors.New("连不上 NodeLoc")
)

// User is the identity module's user aggregate.
type User struct {
	ID              uint           `gorm:"primarykey" json:"id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Username        string         `gorm:"size:64;uniqueIndex;not null" json:"username"`
	Email           *string        `gorm:"size:190;uniqueIndex" json:"email,omitempty"`
	PasswordHash    *string        `gorm:"size:255" json:"-"`
	IsAdmin         bool           `gorm:"default:false;not null" json:"is_admin"`
	IsActive        bool           `gorm:"column:is_active;default:true;not null" json:"is_active"`
	Role            string         `gorm:"size:32;default:'user';not null;index" json:"role"`
	Points          int            `gorm:"default:0;not null" json:"points"`
	ConsecutiveDays int            `gorm:"default:0;not null" json:"consecutive_days"`
	TotalCheckins   int            `gorm:"default:0;not null" json:"total_checkins"`
	LastCheckinDate *time.Time     `json:"last_checkin_date,omitempty"`
	Nickname        string         `gorm:"size:64" json:"nickname"`
	AvatarURL       string         `gorm:"size:255" json:"avatar_url"`
	Bio             string         `gorm:"type:text" json:"bio"`
	OAuthProvider   *string        `gorm:"size:32;index" json:"oauth_provider,omitempty"`
	OAuthUID        *string        `gorm:"size:190;index" json:"oauth_uid,omitempty"`
	OAuthUsername   *string        `gorm:"size:64" json:"oauth_username,omitempty"`
	OAuthName       *string        `gorm:"size:64" json:"oauth_name,omitempty"`
	OAuthAvatar     *string        `gorm:"size:255" json:"oauth_avatar,omitempty"`
	OAuthTrustLevel *int           `json:"oauth_trust_level,omitempty"`
	OAuthScope      *string        `gorm:"size:255" json:"oauth_scope,omitempty"`
	OAuthHasEmail   bool           `gorm:"default:false" json:"oauth_has_email"`
	LastLoginIP     string         `gorm:"size:45" json:"-"`
	LastLoginAt     *time.Time     `json:"last_login_at,omitempty"`
}

// CheckIn is one day's check-in. The row is the receipt; the user's counters are
// the summary kept on the account.
type CheckIn struct {
	ID              uint           `gorm:"primarykey" json:"id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	UserID          uint           `gorm:"not null;index" json:"user_id"`
	CheckinDate     time.Time      `gorm:"type:date;not null;index" json:"checkin_date"`
	RewardPoints    int            `gorm:"default:0;not null" json:"reward_points"`
	ConsecutiveDays int            `gorm:"default:1;not null" json:"consecutive_days"`
}

func (CheckIn) TableName() string { return "check_ins" }

// PointEntry is a row of the points ledger. ReferenceType+ReferenceID are
// unique, which is what makes an award idempotent: re-running the same reward
// fails on the constraint instead of paying twice.
type PointEntry struct {
	ID            uint           `gorm:"primarykey" json:"id"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	UserID        uint           `gorm:"not null;index" json:"user_id"`
	Delta         int            `gorm:"not null" json:"delta"`
	BalanceAfter  int            `gorm:"not null" json:"balance_after"`
	Reason        string         `gorm:"size:120;not null" json:"reason"`
	ReferenceType string         `gorm:"size:32;not null;index:idx_reference,unique" json:"reference_type"`
	ReferenceID   string         `gorm:"size:128;not null;index:idx_reference,unique" json:"reference_id"`
	ActorID       *uint          `json:"actor_id,omitempty"`
}

func (PointEntry) TableName() string { return "point_ledgers" }

// OAuthIdentity links a local User to an OAuth provider identity.
type OAuthIdentity struct {
	ID           uint           `gorm:"primarykey" json:"id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	UserID       uint           `gorm:"uniqueIndex:uniq_oauth_identities_user;not null" json:"user_id"`
	Provider     string         `gorm:"size:32;uniqueIndex:uniq_oauth_identities_provider_uid;not null" json:"provider"`
	ProviderUID  string         `gorm:"size:190;uniqueIndex:uniq_oauth_identities_provider_uid;not null" json:"provider_uid"`
	Username     *string        `gorm:"size:64" json:"username,omitempty"`
	DisplayName  *string        `gorm:"size:64" json:"display_name,omitempty"`
	AvatarURL    *string        `gorm:"size:255" json:"avatar_url,omitempty"`
	Scope        *string        `gorm:"size:255" json:"scope,omitempty"`
	AccessToken  *string        `gorm:"type:text" json:"-"`
	RefreshToken *string        `gorm:"type:text" json:"-"`
}

func (OAuthIdentity) TableName() string { return "oauth_identities" }

// OAuthAttempt is one NodeLoc 登录 round trip as the shop recorded it — the
// step it reached, the reason it stopped, and the provider's own words. It is
// the shared table because the store migrates it with the rest; the identity
// module decides what goes in and keeps credentials out of it.
type OAuthAttempt = models.OAuthAttempt

// OAuthProfile is the normalized identity returned by an OAuth provider.
type OAuthProfile struct {
	Provider     string  `json:"provider"`
	ProviderUID  string  `json:"provider_uid"`
	Username     string  `json:"username"`
	DisplayName  string  `json:"display_name"`
	Email        *string `json:"email,omitempty"`
	AvatarURL    string  `json:"avatar_url"`
	TrustLevel   *int    `json:"trust_level,omitempty"`
	Scope        string  `json:"scope"`
	AccessToken  string  `json:"-"`
	RefreshToken string  `json:"-"`
}

// TokenPair contains a short-lived access token and a refresh token.
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// TokenClaims are the identity claims authenticated by TokenService.
type TokenClaims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	IsAdmin  bool   `json:"is_admin"`
	Type     string `json:"type"`
}

func NewUser(username string, email *string, passwordHash *string) (*User, error) {
	username = strings.TrimSpace(username)
	if username == "" || len(username) > 64 {
		return nil, ErrInvalidInput
	}
	if email != nil {
		normalized := strings.ToLower(strings.TrimSpace(*email))
		if normalized == "" || len(normalized) > 190 {
			return nil, ErrInvalidInput
		}
		email = &normalized
	}
	return &User{
		Username:     username,
		Email:        email,
		PasswordHash: passwordHash,
		IsActive:     true,
		Role:         "user",
	}, nil
}

func NewOAuthIdentity(userID uint, profile OAuthProfile) (*OAuthIdentity, error) {
	profile.Provider = strings.TrimSpace(profile.Provider)
	profile.ProviderUID = strings.TrimSpace(profile.ProviderUID)
	if userID == 0 || profile.Provider == "" || profile.ProviderUID == "" {
		return nil, ErrInvalidInput
	}
	identity := &OAuthIdentity{
		UserID:      userID,
		Provider:    profile.Provider,
		ProviderUID: profile.ProviderUID,
	}
	identity.ApplyProfile(profile)
	return identity, nil
}

func (i *OAuthIdentity) ApplyProfile(profile OAuthProfile) {
	i.Username = stringPointer(profile.Username)
	i.DisplayName = stringPointer(profile.DisplayName)
	i.AvatarURL = stringPointer(profile.AvatarURL)
	i.Scope = stringPointer(profile.Scope)
	i.AccessToken = stringPointer(profile.AccessToken)
	i.RefreshToken = stringPointer(profile.RefreshToken)
}

func (u *User) ApplyOAuthProfile(profile OAuthProfile) {
	u.OAuthProvider = stringPointer(profile.Provider)
	u.OAuthUID = stringPointer(profile.ProviderUID)
	u.OAuthUsername = stringPointer(profile.Username)
	u.OAuthName = stringPointer(profile.DisplayName)
	u.OAuthAvatar = stringPointer(profile.AvatarURL)
	u.OAuthTrustLevel = profile.TrustLevel
	u.OAuthScope = stringPointer(profile.Scope)
	u.OAuthHasEmail = profile.Email != nil && strings.TrimSpace(*profile.Email) != ""
}

func (u *User) ClearOAuthProfile() {
	u.OAuthProvider = nil
	u.OAuthUID = nil
	u.OAuthUsername = nil
	u.OAuthName = nil
	u.OAuthAvatar = nil
	u.OAuthTrustLevel = nil
	u.OAuthScope = nil
	u.OAuthHasEmail = false
}

func stringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

// Check-in rewards: a flat daily base plus a streak bonus that tops out after a
// week, so coming back every day pays but the reward never runs away from the
// shop owner.
const (
	checkinBasePoints  = 5
	checkinStreakBonus = 2
	checkinStreakCap   = 6
)

// Checkin pays out today's check-in and moves the user's counters. The day is
// truncated to UTC because 签到 gates on the calendar day, not the timestamp:
// two checks a minute apart are still the same day, and one yesterday plus one
// today is a streak even if only an hour sits between them.
func (u *User) Checkin(now time.Time) (*CheckIn, error) {
	day := startOfUTCDay(now)
	if u.LastCheckinDate != nil && startOfUTCDay(*u.LastCheckinDate).Equal(day) {
		return nil, ErrAlreadyCheckedIn
	}
	streak := 1
	if u.LastCheckinDate != nil && startOfUTCDay(*u.LastCheckinDate).Equal(day.AddDate(0, 0, -1)) {
		streak = u.ConsecutiveDays + 1
	}
	reward := checkinBasePoints
	if bonus := (streak - 1) * checkinStreakBonus; bonus < checkinStreakCap*checkinStreakBonus {
		reward += bonus
	} else {
		reward += checkinStreakCap * checkinStreakBonus
	}

	u.ConsecutiveDays = streak
	u.TotalCheckins++
	u.LastCheckinDate = &day
	u.Points += reward

	return &CheckIn{UserID: u.ID, CheckinDate: day, RewardPoints: reward, ConsecutiveDays: streak}, nil
}

// HasCheckedInToday reports whether the current streak is still open.
func (u *User) HasCheckedInToday(now time.Time) bool {
	return u.LastCheckinDate != nil && startOfUTCDay(*u.LastCheckinDate).Equal(startOfUTCDay(now))
}

// Roles are every role the application issues, lowest privilege first. They
// live with the User aggregate because IsAdmin is derived from Role; the
// policy side of the same names is authz.Roles.
var Roles = []string{"user", "support", "operator", "admin", "super_admin"}

// ValidRole reports whether a name is one of those roles.
func ValidRole(role string) bool {
	for _, candidate := range Roles {
		if candidate == role {
			return true
		}
	}
	return false
}

// SetRole assigns a role and keeps IsAdmin in step with it. Both the JWT
// claims and the bootstrap check read IsAdmin, so letting the two drift is how
// an account ends up shown as staff in one place and refused in another.
func (u *User) SetRole(role string) error {
	if !ValidRole(role) {
		return fmt.Errorf("%w: 未知的角色 %q", ErrInvalidInput, role)
	}
	u.Role = role
	u.IsAdmin = IsStaff(role)
	return nil
}

// IsStaff reports whether a role belongs to the back office.
func IsStaff(role string) bool { return role != "" && role != "user" }

// ProfileEdit is the part of an account a buyer may change themselves. A nil
// field means "leave it alone", which keeps a partial PATCH from blanking the
// nickname or avatar.
type ProfileEdit struct {
	Nickname  *string
	AvatarURL *string
	Bio       *string
	Email     *string
}

// ApplyProfile validates and applies an edit in place, so an invalid request
// never leaves the aggregate half-changed.
func (u *User) ApplyProfile(edit ProfileEdit) error {
	if edit.Nickname != nil {
		nickname := strings.TrimSpace(*edit.Nickname)
		if len(nickname) > 32 {
			return fmt.Errorf("%w: 昵称最多 32 个字符", ErrInvalidInput)
		}
		u.Nickname = nickname
	}
	if edit.AvatarURL != nil {
		avatar := strings.TrimSpace(*edit.AvatarURL)
		if len(avatar) > 255 {
			return fmt.Errorf("%w: 头像链接过长", ErrInvalidInput)
		}
		if avatar != "" && !strings.HasPrefix(avatar, "https://") && !strings.HasPrefix(avatar, "http://") && !strings.HasPrefix(avatar, "/") {
			return fmt.Errorf("%w: 头像必须是 http(s) 链接或站内路径", ErrInvalidInput)
		}
		u.AvatarURL = avatar
	}
	if edit.Bio != nil {
		bio := strings.TrimSpace(*edit.Bio)
		if len(bio) > 500 {
			return fmt.Errorf("%w: 个人简介最多 500 个字符", ErrInvalidInput)
		}
		u.Bio = bio
	}
	if edit.Email != nil {
		email := strings.ToLower(strings.TrimSpace(*edit.Email))
		if email == "" || len(email) > 190 || !strings.Contains(email, "@") {
			return fmt.Errorf("%w: 邮箱格式不正确", ErrInvalidInput)
		}
		u.Email = &email
	}
	return nil
}

func startOfUTCDay(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}
