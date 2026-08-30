package infrastructure

import (
	"context"
	"errors"
	"strings"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/domain"
	"gorm.io/gorm"
)

// GormUserRepo implements identity persistence using GORM.
type GormUserRepo struct {
	db *gorm.DB
}

func NewGormUserRepo(db *gorm.DB) *GormUserRepo {
	return &GormUserRepo{db: db}
}

func (r *GormUserRepo) Create(ctx context.Context, user *domain.User) error {
	if user == nil {
		return domain.ErrInvalidInput
	}
	return translateGormError(r.db.WithContext(ctx).Create(user).Error)
}

func (r *GormUserRepo) Update(ctx context.Context, user *domain.User) error {
	if user == nil || user.ID == 0 {
		return domain.ErrInvalidInput
	}
	result := r.db.WithContext(ctx).Save(user)
	if result.Error != nil {
		return translateGormError(result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}

func (r *GormUserRepo) FindByID(ctx context.Context, id uint) (*domain.User, error) {
	if id == 0 {
		return nil, domain.ErrUserNotFound
	}
	var user domain.User
	err := r.db.WithContext(ctx).First(&user, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *GormUserRepo) FindByUsername(ctx context.Context, username string) (*domain.User, error) {
	var user domain.User
	err := r.db.WithContext(ctx).Where("LOWER(username) = ?", strings.ToLower(strings.TrimSpace(username))).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *GormUserRepo) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user domain.User
	err := r.db.WithContext(ctx).Where("LOWER(email) = ?", strings.ToLower(strings.TrimSpace(email))).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *GormUserRepo) FindByOAuth(ctx context.Context, provider, providerUID string) (*domain.User, error) {
	var user domain.User
	err := r.db.WithContext(ctx).
		Table("users").
		Joins("JOIN oauth_identities ON oauth_identities.user_id = users.id AND oauth_identities.deleted_at IS NULL").
		Where("oauth_identities.provider = ? AND oauth_identities.provider_uid = ?", strings.TrimSpace(provider), strings.TrimSpace(providerUID)).
		First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *GormUserRepo) UsernameExists(ctx context.Context, username string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.User{}).
		Where("LOWER(username) = ?", strings.ToLower(strings.TrimSpace(username))).
		Count(&count).Error
	return count > 0, err
}

func (r *GormUserRepo) EmailExists(ctx context.Context, email string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.User{}).
		Where("LOWER(email) = ?", strings.ToLower(strings.TrimSpace(email))).
		Count(&count).Error
	return count > 0, err
}

func (r *GormUserRepo) CreateOAuthIdentity(ctx context.Context, identity *domain.OAuthIdentity) error {
	if identity == nil || identity.UserID == 0 || strings.TrimSpace(identity.Provider) == "" || strings.TrimSpace(identity.ProviderUID) == "" {
		return domain.ErrInvalidInput
	}
	return translateGormError(r.db.WithContext(ctx).Create(identity).Error)
}

func (r *GormUserRepo) UpdateOAuthIdentity(ctx context.Context, identity *domain.OAuthIdentity) error {
	if identity == nil || identity.ID == 0 {
		return domain.ErrInvalidInput
	}
	result := r.db.WithContext(ctx).Save(identity)
	if result.Error != nil {
		return translateGormError(result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrIdentityNotFound
	}
	return nil
}

func (r *GormUserRepo) FindOAuthIdentity(ctx context.Context, provider, providerUID string) (*domain.OAuthIdentity, error) {
	var identity domain.OAuthIdentity
	err := r.db.WithContext(ctx).
		Where("provider = ? AND provider_uid = ?", strings.TrimSpace(provider), strings.TrimSpace(providerUID)).
		First(&identity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrIdentityNotFound
	}
	if err != nil {
		return nil, err
	}
	return &identity, nil
}

func (r *GormUserRepo) FindOAuthIdentityByUser(ctx context.Context, userID uint, provider string) (*domain.OAuthIdentity, error) {
	var identity domain.OAuthIdentity
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND provider = ?", userID, strings.TrimSpace(provider)).
		First(&identity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrIdentityNotFound
	}
	if err != nil {
		return nil, err
	}
	return &identity, nil
}

func (r *GormUserRepo) DeleteOAuthIdentity(ctx context.Context, userID uint, provider string) error {
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND provider = ?", userID, strings.TrimSpace(provider)).
		Delete(&domain.OAuthIdentity{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrIdentityNotFound
	}
	return nil
}

func (r *GormUserRepo) CountOAuthIdentities(ctx context.Context, userID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.OAuthIdentity{}).Where("user_id = ?", userID).Count(&count).Error
	return count, err
}

func translateGormError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return domain.ErrIdentityAlreadyBound
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique") || strings.Contains(message, "duplicate") {
		switch {
		case strings.Contains(message, "username"):
			return domain.ErrUsernameTaken
		case strings.Contains(message, "email"):
			return domain.ErrEmailTaken
		case strings.Contains(message, "provider") || strings.Contains(message, "oauth"):
			return domain.ErrIdentityAlreadyBound
		}
	}
	return err
}

func (r *GormUserRepo) List(ctx context.Context, limit, offset int) ([]*domain.User, int64, error) {
	var users []*domain.User
	var total int64
	query := r.db.WithContext(ctx).Model(&domain.User{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, total, nil
}
