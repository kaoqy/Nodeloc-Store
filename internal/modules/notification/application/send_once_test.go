package application

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/notification/contract"
)

// fakeRepo is an inbox that has been asked one question before writing: has this
// exact message already arrived since a given moment?
type fakeRepo struct {
	created []*models.Notification
	// seen holds the answers to give, keyed the way the store would match.
	seen      map[string]bool
	lookups   []string
	failQuery error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{seen: map[string]bool{}, lookups: []string{}}
}

func lookupKey(userID uint, kind, link string, since time.Time) string {
	return strings.Join([]string{
		strconv.FormatUint(uint64(userID), 10), kind, link, since.UTC().Format(time.RFC3339Nano),
	}, "|")
}

func (f *fakeRepo) HasRecent(_ context.Context, userID uint, kind, link string, since time.Time) (bool, error) {
	key := lookupKey(userID, kind, link, since)
	f.lookups = append(f.lookups, key)
	if f.failQuery != nil {
		return false, f.failQuery
	}
	return f.seen[key], nil
}

func (f *fakeRepo) Create(_ context.Context, notification *models.Notification) error {
	f.created = append(f.created, notification)
	return nil
}

func (f *fakeRepo) CreateBatch(_ context.Context, notifications []*models.Notification) error {
	f.created = append(f.created, notifications...)
	return nil
}

func (f *fakeRepo) ListByUser(context.Context, uint, int, int, contract.InboxFilter) ([]models.Notification, int64, error) {
	return nil, 0, nil
}

func (f *fakeRepo) ListFacets(context.Context, uint) ([]contract.InboxFacet, error) { return nil, nil }

func (f *fakeRepo) MarkAsRead(context.Context, uint, uint) error { return nil }

func (f *fakeRepo) CountUnread(context.Context, uint) (int64, error) { return 0, nil }

func (f *fakeRepo) MarkAllRead(context.Context, uint) (int64, error) { return 0, nil }

func (f *fakeRepo) ListActiveUserIDs(context.Context) ([]uint, error) { return nil, nil }

type fakeNotifier struct {
	delivered []models.Notification
}

func (n *fakeNotifier) Notify(_ context.Context, notification models.Notification) error {
	n.delivered = append(n.delivered, notification)
	return nil
}

func warning(userID uint) *models.Notification {
	title := "库存预警：年付密钥 只剩 0 张"
	content := "补货阈值 5 张。"
	link := "/cards/11"
	return &models.Notification{UserID: userID, Type: " stock ", Title: title, Content: &content, Link: &link}
}

func TestSendOnceWritesWhatTheInboxHasNotSeen(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	service := NewService(repo, notifier)
	dayStart := time.Date(2026, 3, 5, 0, 0, 0, 0, time.Local)

	sent, err := service.SendOnce(context.Background(), warning(7), dayStart)
	if err != nil {
		t.Fatalf("SendOnce: %v", err)
	}
	if !sent {
		t.Errorf("sent = false, want an unseen warning to go out")
	}
	if len(repo.created) != 1 {
		t.Fatalf("inbox holds %+v, want exactly one message", repo.created)
	}
	if strings.TrimSpace(repo.created[0].Type) != "stock" || repo.created[0].Type != "stock" {
		t.Errorf("type = %q, want it stored trimmed", repo.created[0].Type)
	}
	if repo.created[0].IsRead {
		t.Errorf("a fresh warning stored as already read")
	}
	if len(notifier.delivered) != 1 {
		t.Errorf("notifier saw %d messages, want 1", len(notifier.delivered))
	}
	// The question it asked has to be the question the store can answer: this
	// account, this kind, this link, this day.
	want := lookupKey(7, "stock", "/cards/11", dayStart)
	if len(repo.lookups) != 1 || repo.lookups[0] != want {
		t.Errorf("lookups = %v, want the same %q the write would be matched by", repo.lookups, want)
	}
}

func TestSendOnceHoldsBackTodayCopy(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	service := NewService(repo, notifier)
	dayStart := time.Date(2026, 3, 5, 0, 0, 0, 0, time.Local)
	repo.seen[lookupKey(7, "stock", "/cards/11", dayStart)] = true

	sent, err := service.SendOnce(context.Background(), warning(7), dayStart)
	if err != nil {
		t.Fatalf("SendOnce: %v", err)
	}
	if sent {
		t.Errorf("sent = true, want the sweep told it added nothing")
	}
	if len(repo.created) != 0 || len(notifier.delivered) != 0 {
		t.Errorf("a warned-today shelf still wrote: %d rows, %d pushes", len(repo.created), len(notifier.delivered))
	}
}

func TestSendOnceRefusesAnAddresslessMessage(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, &fakeNotifier{})
	dayStart := time.Date(2026, 3, 5, 0, 0, 0, 0, time.Local)

	cases := map[string]*models.Notification{
		"nil message":  nil,
		"no recipient": warning(0),
		"no kind":      &models.Notification{UserID: 7, Title: "库存预警"},
		"blank kind":   &models.Notification{UserID: 7, Type: "   ", Title: "库存预警"},
	}
	for name, message := range cases {
		t.Run(name, func(t *testing.T) {
			sent, err := service.SendOnce(context.Background(), message, dayStart)
			if !errors.Is(err, ErrInvalidNotification) {
				t.Errorf("err = %v, want %v", err, ErrInvalidNotification)
			}
			if sent {
				t.Errorf("sent = true for a message nobody can receive")
			}
		})
	}
	if len(repo.lookups) != 0 || len(repo.created) != 0 {
		t.Errorf("an invalid message still reached the store: %d lookups, %d rows", len(repo.lookups), len(repo.created))
	}
}

// When the inbox cannot say whether it has seen the warning, guessing either
// duplicates it or drops it, so the failure goes back to the sweep instead.
func TestSendOnceHandsBackAFailedLookup(t *testing.T) {
	broken := errors.New("inbox is unreadable")
	repo := newFakeRepo()
	repo.failQuery = broken
	service := NewService(repo, &fakeNotifier{})

	sent, err := service.SendOnce(context.Background(), warning(7), time.Now())
	if !errors.Is(err, broken) {
		t.Errorf("err = %v, want the store's own failure", err)
	}
	if sent {
		t.Errorf("sent = true while nothing is known about the inbox")
	}
	if len(repo.created) != 0 {
		t.Errorf("a failed lookup still wrote %d rows", len(repo.created))
	}
}

// A message with no link is matched as a message with no link, not as every
// message of its kind.
func TestSendOnceMatchesTheMissingLink(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, &fakeNotifier{})
	dayStart := time.Date(2026, 3, 5, 0, 0, 0, 0, time.Local)
	message := &models.Notification{UserID: 4, Type: "system", Title: "维护公告"}

	sent, err := service.SendOnce(context.Background(), message, dayStart)
	if err != nil {
		t.Fatalf("SendOnce: %v", err)
	}
	if !sent {
		t.Fatalf("sent = false, want a fresh announcement out")
	}
	if len(repo.lookups) != 1 || repo.lookups[0] != lookupKey(4, "system", "", dayStart) {
		t.Errorf("lookups = %v, want the empty link asked about as itself", repo.lookups)
	}
}
