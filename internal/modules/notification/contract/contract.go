package contract

import (
	"context"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
)

type NotificationRepo interface {
	Create(ctx context.Context, notification *models.Notification) error
	CreateBatch(ctx context.Context, notifications []*models.Notification) error
	ListByUser(ctx context.Context, userID uint, limit, offset int, filter InboxFilter) ([]models.Notification, int64, error)
	// ListFacets counts an inbox by message kind, unread included, so the page
	// offers only the tabs the buyer actually has.
	ListFacets(ctx context.Context, userID uint) ([]InboxFacet, error)
	MarkAsRead(ctx context.Context, id, userID uint) error
	// CountUnread and MarkAllRead back the inbox header: a buyer needs to see
	// whether anything arrived without paging through the list, and one click
	// should be enough to clear it.
	CountUnread(ctx context.Context, userID uint) (int64, error)
	MarkAllRead(ctx context.Context, userID uint) (int64, error)
	ListActiveUserIDs(ctx context.Context) ([]uint, error)
}

// InboxFilter narrows a buyer's own inbox to one kind of message, or to the ones
// still unread. An empty Type means "every kind".
type InboxFilter struct {
	Type       string
	UnreadOnly bool
}

// InboxFacet is one kind of message in an inbox and how much of it is unread.
type InboxFacet struct {
	Type   string `json:"type"`
	Total  int64  `json:"total"`
	Unread int64  `json:"unread"`
}

type Notifier interface {
	Notify(ctx context.Context, notification models.Notification) error
}
