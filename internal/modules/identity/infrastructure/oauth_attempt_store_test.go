package infrastructure

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/domain"
)

func newOAuthTransactionDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.OAuthTransaction{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func newAttemptStoreDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.OAuthAttempt{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestConsumeOAuthTransactionIsOneTimeAndExpires(t *testing.T) {
	db := newOAuthTransactionDB(t)
	repo := NewGormUserRepo(db)
	ctx := context.Background()
	now := time.Now().UTC()
	active := &domain.OAuthTransaction{StateHash: "state-a", Intent: "login", ReturnURL: "/oauth/callback", Status: "pending", ExpiresAt: now.Add(time.Minute)}
	if err := repo.CreateOAuthTransaction(ctx, active); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ConsumeOAuthTransaction(ctx, "state-a", now); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if _, err := repo.ConsumeOAuthTransaction(ctx, "state-a", now); !errors.Is(err, domain.ErrOAuthTransactionUsed) {
		t.Fatalf("second consume = %v, want transaction error", err)
	}
	expired := &domain.OAuthTransaction{StateHash: "state-b", Intent: "login", ReturnURL: "/", Status: "pending", ExpiresAt: now.Add(-time.Second)}
	if err := repo.CreateOAuthTransaction(ctx, expired); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ConsumeOAuthTransaction(ctx, "state-b", now); !errors.Is(err, domain.ErrOAuthTransactionExpired) {
		t.Fatalf("expired consume = %v, want transaction error", err)
	}
}

// The trail names buyers, so it cannot be the shop's permanent record of who
// ever pressed 登录. It is trimmed where it is written rather than by a scheduled
// sweep, because a sweep only runs if the container stays up long enough.
func TestRecordOAuthAttemptKeepsOnlyTheRecentTrail(t *testing.T) {
	repo := NewGormUserRepo(newAttemptStoreDB(t))
	ctx := context.Background()

	for i := 0; i < oauthAttemptKeep+40; i++ {
		attempt := &domain.OAuthAttempt{Step: "callback", Outcome: "failed", Reason: "expired"}
		if err := repo.RecordOAuthAttempt(ctx, attempt); err != nil {
			t.Fatalf("record attempt %d: %v", i, err)
		}
	}

	var count int64
	if err := repo.db.Model(&domain.OAuthAttempt{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != oauthAttemptKeep {
		t.Errorf("%d rows left after %d logins, want the cap of %d", count, oauthAttemptKeep+40, oauthAttemptKeep)
	}

	kept, err := repo.ListOAuthAttempts(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 1 {
		t.Fatalf("read back %d rows with limit 1, want 1", len(kept))
	}
	if kept[0].ID != uint(oauthAttemptKeep+40) {
		t.Errorf("newest kept row is #%d, want the last one written (#%d)", kept[0].ID, oauthAttemptKeep+40)
	}

	var minID uint
	if err := repo.db.Model(&domain.OAuthAttempt{}).Select("MIN(id)").Scan(&minID).Error; err != nil {
		t.Fatal(err)
	}
	if minID != 41 {
		t.Errorf("oldest kept row is #%d, want #41 — the first 40 should have been trimmed", minID)
	}
}
