package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/notification/contract"
)

var ErrInvalidNotification = errors.New("user_id, type and title are required")

type Service struct {
	repo     contract.NotificationRepo
	notifier contract.Notifier
}

func NewService(repo contract.NotificationRepo, notifier contract.Notifier) *Service {
	return &Service{repo: repo, notifier: notifier}
}

func (s *Service) Send(ctx context.Context, notification *models.Notification) error {
	if notification == nil || notification.UserID == 0 || strings.TrimSpace(notification.Type) == "" || strings.TrimSpace(notification.Title) == "" {
		return ErrInvalidNotification
	}
	notification.Type = strings.TrimSpace(notification.Type)
	notification.Title = strings.TrimSpace(notification.Title)
	notification.IsRead = false
	if err := s.repo.Create(ctx, notification); err != nil {
		return err
	}
	if s.notifier != nil {
		return s.notifier.Notify(ctx, *notification)
	}
	return nil
}

// SendOnce writes the message only when the very same one has not reached this
// account since `since`, and reports whether it landed. A shop that re-checks a
// shelf every few minutes gets one warning a day out of it, and the caller can
// still count the messages it really sent.
func (s *Service) SendOnce(ctx context.Context, notification *models.Notification, since time.Time) (bool, error) {
	if notification == nil || notification.UserID == 0 || strings.TrimSpace(notification.Type) == "" {
		return false, ErrInvalidNotification
	}
	link := ""
	if notification.Link != nil {
		link = *notification.Link
	}
	recent, err := s.repo.HasRecent(ctx, notification.UserID, strings.TrimSpace(notification.Type), link, since)
	if err != nil {
		return false, err
	}
	if recent {
		return false, nil
	}
	return true, s.Send(ctx, notification)
}

func (s *Service) Broadcast(ctx context.Context, notificationType, title string, content, link *string) (int, error) {
	notificationType = strings.TrimSpace(notificationType)
	title = strings.TrimSpace(title)
	if notificationType == "" || title == "" {
		return 0, ErrInvalidNotification
	}
	userIDs, err := s.repo.ListActiveUserIDs(ctx)
	if err != nil {
		return 0, err
	}
	notifications := make([]*models.Notification, 0, len(userIDs))
	for _, userID := range userIDs {
		notifications = append(notifications, &models.Notification{UserID: userID, Type: notificationType, Title: title, Content: content, Link: link})
	}
	if err := s.repo.CreateBatch(ctx, notifications); err != nil {
		return 0, err
	}
	if s.notifier != nil {
		for _, notification := range notifications {
			if err := s.notifier.Notify(ctx, *notification); err != nil {
				return 0, err
			}
		}
	}
	return len(notifications), nil
}

// InboxPaging clamps what a caller asked for to what the inbox serves: page 1 is
// the floor and one page holds at most 100 rows. Both the query and the response
// run through it, so the API never echoes back a page size it quietly ignored.
func InboxPaging(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

// List reads one page of a buyer's own inbox. The filter narrows it to one kind
// of message or to the unread ones; the total it reports is the filtered one, so
// the page never counts rows it cannot show.
func (s *Service) List(ctx context.Context, userID uint, page, pageSize int, filter contract.InboxFilter) ([]models.Notification, int64, error) {
	page, pageSize = InboxPaging(page, pageSize)
	filter.Type = strings.TrimSpace(filter.Type)
	return s.repo.ListByUser(ctx, userID, pageSize, (page-1)*pageSize, filter)
}

// Facets describes the whole inbox by kind, so the page can offer a 订单 tab only
// to a buyer who has order messages and show how much of each is unread.
func (s *Service) Facets(ctx context.Context, userID uint) ([]contract.InboxFacet, error) {
	if userID == 0 {
		return nil, ErrInvalidNotification
	}
	return s.repo.ListFacets(ctx, userID)
}

func (s *Service) MarkAsRead(ctx context.Context, id, userID uint) error {
	if id == 0 || userID == 0 {
		return ErrInvalidNotification
	}
	return s.repo.MarkAsRead(ctx, id, userID)
}

// CountUnread is the badge on the storefront header. A failure here is only
// ever a missing number, so it stays the caller's problem to swallow.
func (s *Service) CountUnread(ctx context.Context, userID uint) (int64, error) {
	if userID == 0 {
		return 0, ErrInvalidNotification
	}
	return s.repo.CountUnread(ctx, userID)
}

// MarkAllRead reads the whole inbox in one go and reports how many rows
// actually flipped, so the page can confirm the count it cleared.
func (s *Service) MarkAllRead(ctx context.Context, userID uint) (int64, error) {
	if userID == 0 {
		return 0, ErrInvalidNotification
	}
	return s.repo.MarkAllRead(ctx, userID)
}
