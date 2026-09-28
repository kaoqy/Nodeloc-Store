package contract

import (
	"context"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
)

type NotificationRepo interface {
	Create(ctx context.Context, notification *models.Notification) error
	CreateBatch(ctx context.Context, notifications []*models.Notification) error
	ListByUser(ctx context.Context, userID uint, limit, offset int) ([]models.Notification, int64, error)
	MarkAsRead(ctx context.Context, id, userID uint) error
	// CountUnread and MarkAllRead back the inbox header: a buyer needs to see
	// whether anything arrived without paging through the list, and one click
	// should be enough to clear it.
	CountUnread(ctx context.Context, userID uint) (int64, error)
	MarkAllRead(ctx context.Context, userID uint) (int64, error)
	ListActiveUserIDs(ctx context.Context) ([]uint, error)
}

type Notifier interface {
	Notify(ctx context.Context, notification models.Notification) error
}
