package infrastructure

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit/domain"
)

func newAuditTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.AuditLog{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// seed writes n rows of one action, oldest first, all at minute spacing so the
// export's created_at DESC ordering is predictable.
func seed(t *testing.T, db *gorm.DB, action string, actorID *uint, n int) {
	t.Helper()
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	for i := 0; i < n; i++ {
		detail := fmt.Sprintf("%s detail %d", action, i)
		target := fmt.Sprintf("order-%d", i)
		entry := &models.AuditLog{
			Action:  action,
			ActorID: actorID,
			Target:  &target,
			Detail:  &detail,
		}
		entry.CreatedAt = base.Add(time.Duration(i) * time.Minute)
		if err := db.Create(entry).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func ptr[T any](v T) *T { return &v }

// The download exists because the page does not: a shop owner investigating an
// incident needs every matching row, not the twenty the screen fits.
func TestExportWalksPastOnePage(t *testing.T) {
	db := newAuditTestDB(t)
	store := NewGormStore(db)
	ctx := context.Background()

	const rows = exportBatch*2 + 100
	seed(t, db, "order.refund", ptr(uint(7)), rows)

	logs, truncated, err := store.Export(ctx, domain.LogFilter{}, rows+1000)
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Fatalf("export of %d rows under a larger cap reported truncation", rows)
	}
	if len(logs) != rows {
		t.Fatalf("got %d rows, want %d", len(logs), rows)
	}
	// Same newest-first order the list page uses, so the top of the file is what
	// the owner saw at the top of the page.
	if !logs[0].CreatedAt.After(logs[1].CreatedAt) {
		t.Fatalf("first two rows are not newest-first: %v then %v", logs[0].CreatedAt, logs[1].CreatedAt)
	}
	if last := logs[len(logs)-1]; !last.CreatedAt.Before(logs[0].CreatedAt) {
		t.Fatalf("last row %v is not older than the first %v", last.CreatedAt, logs[0].CreatedAt)
	}
}

// Crossing the batch boundary has to neither repeat a row nor drop one, and an
// export that had to stop has to say so instead of looking complete.
func TestExportCapsAndFlagsTruncation(t *testing.T) {
	db := newAuditTestDB(t)
	store := NewGormStore(db)
	ctx := context.Background()

	seed(t, db, "admin.login", nil, exportBatch+25)

	logs, truncated, err := store.Export(ctx, domain.LogFilter{}, exportBatch)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != exportBatch {
		t.Fatalf("got %d rows, want the cap of %d", len(logs), exportBatch)
	}
	if !truncated {
		t.Fatal("a cap that cut off 25 rows reported a complete export")
	}

	seen := make(map[uint]bool, len(logs))
	for _, entry := range logs {
		if seen[entry.ID] {
			t.Fatalf("row %d came out twice", entry.ID)
		}
		seen[entry.ID] = true
	}

	whole, again, err := store.Export(ctx, domain.LogFilter{}, exportBatch+25)
	if err != nil {
		t.Fatal(err)
	}
	if len(whole) != exportBatch+25 || again {
		t.Fatalf("got %d rows truncated=%v, want %d rows untruncated", len(whole), again, exportBatch+25)
	}
}

// The filters are what make a log download worth archiving, so the export has to
// agree with the page row for row.
func TestExportSharesThePageFilters(t *testing.T) {
	db := newAuditTestDB(t)
	store := NewGormStore(db)
	ctx := context.Background()

	seed(t, db, "order.refund", ptr(uint(3)), 610)
	seed(t, db, "product.update", ptr(uint(4)), 20)
	seed(t, db, "system.sweep", nil, 15)

	cases := []struct {
		name   string
		filter domain.LogFilter
		want   int
	}{
		{"action prefix", domain.LogFilter{Action: "order"}, 610},
		{"by actor", domain.LogFilter{ActorID: ptr(uint(4))}, 20},
		{"system only", domain.LogFilter{SystemOnly: true}, 15},
		{"search", domain.LogFilter{Search: "product.update detail 5"}, 1},
	}
	for _, tc := range cases {
		logs, truncated, err := store.Export(ctx, tc.filter, 5000)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(logs) != tc.want || truncated {
			t.Fatalf("%s: got %d rows truncated=%v, want %d", tc.name, len(logs), truncated, tc.want)
		}
		for _, entry := range logs {
			switch tc.name {
			case "action prefix":
				if entry.Action != "order.refund" {
					t.Fatalf("prefix export carried %s", entry.Action)
				}
			case "by actor":
				if entry.ActorID == nil || *entry.ActorID != 4 {
					t.Fatalf("actor export carried %+v", entry.ActorID)
				}
			case "system only":
				if entry.ActorID != nil {
					t.Fatalf("system export carries actor %d", *entry.ActorID)
				}
			}
		}
	}

	// A date window narrower than the seed range must not pull the whole table.
	filter := domain.LogFilter{
		Action: "order.refund",
		Since:  ptr(time.Date(2026, 9, 1, 0, 5, 0, 0, time.Local)),
		Before: ptr(time.Date(2026, 9, 1, 0, 8, 0, 0, time.Local)),
	}
	logs, _, err := store.Export(ctx, filter, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 3 {
		t.Fatalf("date window returned %d rows, want the three minutes inside it", len(logs))
	}
}

// A log row that says who acted is the difference between an audit trail and a
// list of numbers, and the download is where it gets read.
func TestExportResolvesActorNames(t *testing.T) {
	db := newAuditTestDB(t)
	store := NewGormStore(db)
	ctx := context.Background()

	owner := &models.User{Username: "kaoqy", Role: "super_admin"}
	if err := db.Create(owner).Error; err != nil {
		t.Fatal(err)
	}
	seed(t, db, "order.refund", ptr(owner.ID), exportBatch+7)

	logs, _, err := store.Export(ctx, domain.LogFilter{}, 5000)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range logs {
		if entry.ActorName != "kaoqy" {
			t.Fatalf("row %d carries actor name %q, want kaoqy", entry.ID, entry.ActorName)
		}
	}
}
