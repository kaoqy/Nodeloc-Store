package contract

import (
	"context"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
)

type NotificationRepo interface {
	Create(ctx context.Context, notification *models.Notification) error
	CreateBatch(ctx context.Context, notifications []*models.Notification) error
	// HasRecent answers whether this exact message already reached this account at
	// or after the given moment, so a warning the shop re-checks every few minutes
	// can go out once a day instead of once a pass.
	HasRecent(ctx context.Context, userID uint, notificationType, link string, since time.Time) (bool, error)
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

// MailAddressReader resolves the inbox address for one account. It is a port
// rather than a store call so the notification module never has to know how
// accounts are kept; a missing address is not an error, it just means this
// message has no 站外 copy to send.
type MailAddressReader interface {
	EmailFor(ctx context.Context, userID uint) (string, error)
}

// MailConfigReader supplies the shop's SMTP settings. The notification module
// asks on every send so a settings change applies without a restart, and a shop
// that has not configured SMTP simply reads as 「off」.
type MailConfigReader interface {
	MailConfig(ctx context.Context) (MailConfig, bool)
}

// MailConfig is the subset of the shop's SMTP settings a mail needs. It is the
// notification module's own shape so it does not import the system module.
type MailConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	Secure   string
	From     string
	SiteName string
	// BaseURL is the shop's own origin (https://shop.example.com), used to turn a
	// relative notification link into one a mail client can open.
	BaseURL string
}

// MailSender delivers one email. A failure is the caller's to log, never the
// buyer's to see: the 站内 notification has already been written by then.
type MailSender interface {
	SendMail(config MailConfig, to, subject, body string) error
}
