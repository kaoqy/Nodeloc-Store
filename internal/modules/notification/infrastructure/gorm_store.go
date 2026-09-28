package infrastructure

import (
	"context"
	"errors"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"gorm.io/gorm"
)

var ErrNotificationNotFound = errors.New("notification not found")

type GormStore struct {
	db *gorm.DB
}

func NewGormStore(db *gorm.DB) *GormStore { return &GormStore{db: db} }

func (s *GormStore) Create(ctx context.Context, notification *models.Notification) error {
	return s.db.WithContext(ctx).Create(notification).Error
}

func (s *GormStore) CreateBatch(ctx context.Context, notifications []*models.Notification) error {
	if len(notifications) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).CreateInBatches(notifications, 500).Error
}

func (s *GormStore) ListByUser(ctx context.Context, userID uint, limit, offset int) ([]models.Notification, int64, error) {
	var notifications []models.Notification
	var total int64
	query := s.db.WithContext(ctx).Model(&models.Notification{}).Where("user_id = ?", userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&notifications).Error; err != nil {
		return nil, 0, err
	}
	return notifications, total, nil
}

func (s *GormStore) MarkAsRead(ctx context.Context, id, userID uint) error {
	result := s.db.WithContext(ctx).Model(&models.Notification{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("is_read", true)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	// MySQL reports the rows it changed rather than the rows it matched, so
	// marking an already-read notification lands here. That is not a missing
	// notification, and the buyer shouldn't get an error for clicking twice.
	var matches int64
	if err := s.db.WithContext(ctx).Model(&models.Notification{}).
		Where("id = ? AND user_id = ?", id, userID).Count(&matches).Error; err != nil {
		return err
	}
	if matches == 0 {
		return ErrNotificationNotFound
	}
	return nil
}

// CountUnread answers 「有没有新消息」 without pulling the list.
func (s *GormStore) CountUnread(ctx context.Context, userID uint) (int64, error) {
	var unread int64
	err := s.db.WithContext(ctx).Model(&models.Notification{}).
		Where("user_id = ? AND is_read = ?", userID, false).Count(&unread).Error
	return unread, err
}

// MarkAllRead clears the badge in one click and reports how many it changed,
// so the page can say 「已标为已读 N 条」 instead of a bare success.
func (s *GormStore) MarkAllRead(ctx context.Context, userID uint) (int64, error) {
	result := s.db.WithContext(ctx).Model(&models.Notification{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Update("is_read", true)
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

func (s *GormStore) ListActiveUserIDs(ctx context.Context) ([]uint, error) {
	var ids []uint
	err := s.db.WithContext(ctx).Model(&models.User{}).Where("is_active = ?", true).Pluck("id", &ids).Error
	return ids, err
}
