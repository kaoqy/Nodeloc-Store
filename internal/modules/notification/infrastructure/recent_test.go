package infrastructure

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
)

func newNotificationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Notification{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// seedMessage writes one row with its own timestamp, because the question under
// test is exactly when the message landed.
func seedMessage(t *testing.T, db *gorm.DB, userID uint, kind, link string, at time.Time) {
	t.Helper()
	value := link
	row := models.Notification{UserID: userID, Type: kind, Title: "库存预警", Link: &value}
	row.CreatedAt = at
	row.UpdatedAt = at
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("seed %s for user %d: %v", kind, userID, err)
	}
}

// The restock sweep asks this question every few minutes, so the day boundary is
// the whole feature: today's copy silences today, and nothing else does.
func TestHasRecentLooksInsideTheGivenWindow(t *testing.T) {
	db := newNotificationTestDB(t)
	repo := &GormStore{db: db}
	dayStart := time.Date(2026, 3, 5, 0, 0, 0, 0, time.Local)
	morning := time.Date(2026, 3, 5, 8, 30, 0, 0, time.Local)
	lastNight := time.Date(2026, 3, 4, 23, 30, 0, 0, time.Local)

	seedMessage(t, db, 7, "stock", "/cards/11", morning)
	seedMessage(t, db, 7, "stock", "/cards/12", lastNight)
	seedMessage(t, db, 7, "order", "/cards/13", morning)

	cases := []struct {
		name   string
		userID uint
		kind   string
		link   string
		since  time.Time
		want   bool
	}{
		{"today's copy counts", 7, "stock", "/cards/11", dayStart, true},
		{"a warning from before the window is yesterday's", 7, "stock", "/cards/12", dayStart, false},
		{"another account has its own inbox", 8, "stock", "/cards/11", dayStart, false},
		{"another shelf is another warning", 7, "stock", "/cards/99", dayStart, false},
		{"another kind of message is not this one", 7, "stock", "/cards/13", dayStart, false},
		{"a window starting after the send is empty", 7, "stock", "/cards/11", morning.Add(time.Minute), false},
		{"a window opening on the send counts it", 7, "stock", "/cards/11", morning, true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := repo.HasRecent(context.Background(), testCase.userID, testCase.kind, testCase.link, testCase.since)
			if err != nil {
				t.Fatalf("HasRecent: %v", err)
			}
			if got != testCase.want {
				t.Errorf("HasRecent(user %d, %s, %s, since %v) = %v, want %v",
					testCase.userID, testCase.kind, testCase.link, testCase.since, got, testCase.want)
			}
		})
	}
}

// A message written without a link is still identifiable, and asking with an
// empty link must not match a message that carries one.
func TestHasRecentKeepsLinksWithAndWithout(t *testing.T) {
	db := newNotificationTestDB(t)
	repo := &GormStore{db: db}
	now := time.Date(2026, 3, 5, 9, 0, 0, 0, time.Local)
	row := models.Notification{UserID: 3, Type: "system", Title: "维护公告"}
	row.CreatedAt = now
	row.UpdatedAt = now
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	linked, err := repo.HasRecent(context.Background(), 3, "system", "/maintenance", now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("HasRecent: %v", err)
	}
	if linked {
		t.Errorf("a message that carries no link answered for a message that names one")
	}
	plain, err := repo.HasRecent(context.Background(), 3, "system", "", now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("HasRecent: %v", err)
	}
	if !plain {
		t.Errorf("the message that was written has no link to match on")
	}
}
