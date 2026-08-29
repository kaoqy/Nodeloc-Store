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
	if result.RowsAffected == 0 {
		return ErrNotificationNotFound
	}
	return nil
}

func (s *GormStore) ListActiveUserIDs(ctx context.Context) ([]uint, error) {
	var ids []uint
	err := s.db.WithContext(ctx).Model(&models.User{}).Where("is_active = ?", true).Pluck("id", &ids).Error
	return ids, err
}
