package infrastructure

import (
	"context"
	"errors"
	"strings"
	"time"

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
		Select("users.*").
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

func (r *GormUserRepo) CreateOAuthTransaction(ctx context.Context, transaction *domain.OAuthTransaction) error {
	if transaction == nil || transaction.StateHash == "" || transaction.Intent == "" {
		return domain.ErrInvalidInput
	}
	return r.db.WithContext(ctx).Create(transaction).Error
}

func (r *GormUserRepo) ConsumeOAuthTransaction(ctx context.Context, stateHash string, now time.Time) (*domain.OAuthTransaction, error) {
	var transaction domain.OAuthTransaction
	result := r.db.WithContext(ctx).Model(&transaction).
		Where("state_hash = ? AND consumed_at IS NULL AND status = ? AND expires_at > ?", stateHash, "pending", now).
		Updates(map[string]any{"consumed_at": now, "status": "consumed"})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		var existing domain.OAuthTransaction
		if err := r.db.WithContext(ctx).Where("state_hash = ?", stateHash).First(&existing).Error; err != nil {
			return nil, domain.ErrOAuthTransactionExpired
		}
		if existing.ConsumedAt != nil || existing.Status != "pending" {
			return nil, domain.ErrOAuthTransactionUsed
		}
		return nil, domain.ErrOAuthTransactionExpired
	}
	if err := r.db.WithContext(ctx).Where("state_hash = ?", stateHash).First(&transaction).Error; err != nil {
		return nil, err
	}
	return &transaction, nil
}

// oauthAttemptKeep is how far back the shop remembers 登录 attempts. The trail is
// written to be read after a complaint, not to be a history of who ever signed
// in: it names buyers, so it has to stop growing at some point.
const oauthAttemptKeep = 500

func (r *GormUserRepo) RecordOAuthAttempt(ctx context.Context, attempt *domain.OAuthAttempt) error {
	if err := r.db.WithContext(ctx).Create(attempt).Error; err != nil {
		return err
	}
	// Best-effort trim, in the same write path so it cannot be forgotten: a
	// scheduled sweep would need the container to stay up long enough to run one.
	r.db.WithContext(ctx).Exec(
		"DELETE FROM oauth_attempts WHERE id <= (SELECT COALESCE(MAX(id), 0) - ? FROM oauth_attempts)",
		oauthAttemptKeep,
	)
	return nil
}

func (r *GormUserRepo) ListOAuthAttempts(ctx context.Context, limit int) ([]domain.OAuthAttempt, error) {
	switch {
	case limit <= 0:
		limit = 20
	case limit > 200:
		limit = 200
	}
	var attempts []domain.OAuthAttempt
	err := r.db.WithContext(ctx).Order("id DESC").Limit(limit).Find(&attempts).Error
	return attempts, err
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

func (r *GormUserRepo) List(ctx context.Context, limit, offset int, search, role string) ([]*domain.User, int64, error) {
	var users []*domain.User
	var total int64
	query := r.db.WithContext(ctx).Model(&domain.User{})
	if pattern := strings.TrimSpace(search); pattern != "" {
		like := "%" + pattern + "%"
		query = query.Where("username LIKE ? OR email LIKE ? OR nickname LIKE ?", like, like, like)
	}
	if role = strings.TrimSpace(role); role != "" {
		switch role {
		case "staff":
			// 历史角色仍在库中，但界面上统一称作「管理员」；筛选员工时
			// 不能只匹配 role=admin，否则升级前创建的客服/运营账号会漏掉。
			query = query.Where("role <> ?", "user")
		default:
			query = query.Where("role = ?", role)
		}
	}
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

func (r *GormUserRepo) RecordCheckin(ctx context.Context, user *domain.User, checkin *domain.CheckIn, entry *domain.PointEntry) error {
	if user == nil || checkin == nil || entry == nil {
		return domain.ErrInvalidInput
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// The user's own counters usually already answer "today?", but two taps
		// can both read a stale row. This check inside the transaction plus the
		// ledger's unique reference is what makes the second one pay nothing.
		var already int64
		if err := tx.Model(&domain.CheckIn{}).
			Where("user_id = ? AND checkin_date >= ? AND checkin_date < ?", checkin.UserID, checkin.CheckinDate, checkin.CheckinDate.AddDate(0, 0, 1)).
			Count(&already).Error; err != nil {
			return err
		}
		if already > 0 {
			return domain.ErrAlreadyCheckedIn
		}
		if err := tx.Create(checkin).Error; err != nil {
			return err
		}
		if err := tx.Create(entry).Error; err != nil {
			return err
		}
		return tx.Save(user).Error
	})
	if err != nil {
		return translateGormError(err)
	}
	return nil
}

func (r *GormUserRepo) AdjustPoints(ctx context.Context, user *domain.User, entry *domain.PointEntry) error {
	if user == nil || user.ID == 0 || entry == nil {
		return domain.ErrInvalidInput
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(entry).Error; err != nil {
			return err
		}
		return tx.Save(user).Error
	})
	if err != nil {
		return translateGormError(err)
	}
	return nil
}

func (r *GormUserRepo) ListPoints(ctx context.Context, userID uint, limit, offset int) ([]domain.PointEntry, int64, error) {
	var entries []domain.PointEntry
	var total int64
	query := r.db.WithContext(ctx).Model(&domain.PointEntry{}).Where("user_id = ?", userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&entries).Error
	return entries, total, err
}

func (r *GormUserRepo) ListCheckins(ctx context.Context, userID uint, limit int) ([]domain.CheckIn, error) {
	if limit <= 0 || limit > 60 {
		limit = 30
	}
	var checkins []domain.CheckIn
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("checkin_date DESC").
		Limit(limit).
		Find(&checkins).Error
	return checkins, err
}
