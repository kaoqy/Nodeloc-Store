package application

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/notification/contract"
)

var ErrInvalidNotification = errors.New("user_id, type and title are required")

type Service struct {
	repo     contract.NotificationRepo
	notifier contract.Notifier
	// mail is the 站外 half. Every piece is optional: a shop with no SMTP
	// configured, or a buyer with no email address, still gets the 站内 message,
	// which is the one the badge counts.
	addresses contract.MailAddressReader
	mailConf  contract.MailConfigReader
	sender    contract.MailSender
}

func NewService(repo contract.NotificationRepo, notifier contract.Notifier) *Service {
	return &Service{repo: repo, notifier: notifier}
}

// EnableMail attaches the outbound half of a notification. It is a setter rather
// than a constructor argument because the pieces live in other modules (accounts
// and settings) and the notification module has to stay usable without them.
func (s *Service) EnableMail(addresses contract.MailAddressReader, config contract.MailConfigReader, sender contract.MailSender) {
	s.addresses = addresses
	s.mailConf = config
	s.sender = sender
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
		if err := s.notifier.Notify(ctx, *notification); err != nil {
			return err
		}
	}
	s.sendMail(ctx, *notification)
	return nil
}

// sendMail mirrors one written notification to the account's email address, when
// the shop has SMTP switched on and that account has an address.
//
// It is deliberately best-effort and silent: the 站内 message is already stored
// by the time this runs, so a mail server that is down must not turn a
// successful notification into a failed one. Every outcome is logged instead.
func (s *Service) sendMail(ctx context.Context, notification models.Notification) {
	if s.sender == nil || s.addresses == nil || s.mailConf == nil {
		return
	}
	config, on := s.mailConf.MailConfig(ctx)
	if !on {
		return
	}
	address, err := s.addresses.EmailFor(ctx, notification.UserID)
	if err != nil {
		log.Printf("[notification] mail: could not read the address for user %d: %v", notification.UserID, err)
		return
	}
	address = strings.TrimSpace(address)
	if address == "" {
		// A buyer who signed in with NodeLoc without the email scope, or who never
		// set one, simply has no 站外 inbox here.
		return
	}
	if err := s.sender.SendMail(config, address, mailSubject(config, notification), mailBody(config, notification)); err != nil {
		log.Printf("[notification] mail: user %d (%s): %v", notification.UserID, notification.Title, err)
	}
}

// mailSubject prefixes the shop's own name so a buyer can tell at a glance which
// store the message is from.
func mailSubject(config contract.MailConfig, notification models.Notification) string {
	name := strings.TrimSpace(config.SiteName)
	if name == "" {
		return notification.Title
	}
	return "【" + name + "】" + notification.Title
}

// mailBody is the plain-text version of the message. The link is written as an
// absolute address when the shop knows its own domain, so a mail client's link
// goes somewhere; otherwise the relative path is still readable.
// absoluteLink turns a storefront-relative notification link into one a mail
// client can open. The scheme is never guessed from the visitor's request: an
// email outlives the request that produced it, so only the shop's own setting is
// authoritative here.
func absoluteLink(base, link string) string {
	link = strings.TrimSpace(link)
	if strings.HasPrefix(link, "//") {
		// A protocol-relative address is not something this shop writes, and
		// following it would send the buyer to whatever host it names.
		return ""
	}
	if strings.HasPrefix(link, "/") {
		base = strings.TrimRight(strings.TrimSpace(base), "/")
		if base == "" {
			return link
		}
		return base + link
	}
	if strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") {
		return link
	}
	return ""
}

func mailBody(config contract.MailConfig, notification models.Notification) string {
	var b strings.Builder
	b.WriteString(notification.Title)
	b.WriteString("\r\n\r\n")
	if notification.Content != nil && strings.TrimSpace(*notification.Content) != "" {
		b.WriteString(strings.TrimSpace(*notification.Content))
		b.WriteString("\r\n\r\n")
	}
	if notification.Link != nil && strings.TrimSpace(*notification.Link) != "" {
		b.WriteString("查看详情：")
		b.WriteString(absoluteLink(config.BaseURL, *notification.Link))
		b.WriteString("\r\n\r\n")
	}
	b.WriteString("这是一封由 ")
	b.WriteString(strings.TrimSpace(config.SiteName))
	b.WriteString(" 自动发出的通知邮件，请勿直接回复。")
	return b.String()
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
