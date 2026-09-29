package infrastructure

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/notification/contract"
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

// HasRecent looks for one exact message in one inbox since a moment. The link
// belongs to that identity: the same wording about another product is another
// warning, and a message that carries no link matches only messages with none,
// since an empty column and a written empty string are the same absence.
func (s *GormStore) HasRecent(ctx context.Context, userID uint, notificationType, link string, since time.Time) (bool, error) {
	query := s.db.WithContext(ctx).Model(&models.Notification{}).
		Where("user_id = ? AND type = ? AND created_at >= ?", userID, notificationType, since)
	if link == "" {
		query = query.Where("link IS NULL OR link = ''")
	} else {
		query = query.Where("link = ?", link)
	}
	var matches int64
	if err := query.Count(&matches).Error; err != nil {
		return false, err
	}
	return matches > 0, nil
}

func (s *GormStore) ListByUser(ctx context.Context, userID uint, limit, offset int, filter contract.InboxFilter) ([]models.Notification, int64, error) {
	var notifications []models.Notification
	var total int64
	query := s.db.WithContext(ctx).Model(&models.Notification{}).Where("user_id = ?", userID)
	if kind := strings.TrimSpace(filter.Type); kind != "" {
		query = query.Where("type = ?", kind)
	}
	if filter.UnreadOnly {
		query = query.Where("is_read = ?", false)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	// created_at only reaches the second on some databases and two messages can
	// land in it together; without the id tiebreaker the same row can show up on
	// two pages, or on none.
	if err := query.Order("created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&notifications).Error; err != nil {
		return nil, 0, err
	}
	return notifications, total, nil
}

func (s *GormStore) ListFacets(ctx context.Context, userID uint) ([]contract.InboxFacet, error) {
	var facets []contract.InboxFacet
	err := s.db.WithContext(ctx).Model(&models.Notification{}).
		Select("type, COUNT(*) AS total, SUM(CASE WHEN is_read = 1 THEN 0 ELSE 1 END) AS unread").
		Where("user_id = ?", userID).
		Group("type").
		Order("type ASC").
		Scan(&facets).Error
	return facets, err
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
