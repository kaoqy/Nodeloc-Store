package domain

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
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

// OAuthIdentity links a local User to an OAuth provider identity.
type OAuthIdentity struct {
	ID           uint           `gorm:"primarykey" json:"id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	UserID       uint           `gorm:"uniqueIndex:idx_user_provider;not null" json:"user_id"`
	Provider     string         `gorm:"size:32;uniqueIndex:idx_provider_uid;not null" json:"provider"`
	ProviderUID  string         `gorm:"size:190;uniqueIndex:idx_provider_uid;not null" json:"provider_uid"`
	Username     *string        `gorm:"size:64" json:"username,omitempty"`
	DisplayName  *string        `gorm:"size:64" json:"display_name,omitempty"`
	AvatarURL    *string        `gorm:"size:255" json:"avatar_url,omitempty"`
	Scope        *string        `gorm:"size:255" json:"scope,omitempty"`
	AccessToken  *string        `gorm:"type:text" json:"-"`
	RefreshToken *string        `gorm:"type:text" json:"-"`
}

func (OAuthIdentity) TableName() string { return "oauth_identities" }

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
