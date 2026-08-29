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
	ListActiveUserIDs(ctx context.Context) ([]uint, error)
}

type Notifier interface {
	Notify(ctx context.Context, notification models.Notification) error
}
