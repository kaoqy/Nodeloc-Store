package infrastructure

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
)

func newQuotaDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Product{}, &models.Order{}, &models.Card{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// A 限量 code used to be spent by every 立即购买 that was clicked and then
// abandoned, while the back office's 已使用 counter still read 0 — the shop could
// not see why the next buyer was told 已达使用上限. These counts are the rule the
// promo shelf, 总量限用 and 每人限用 all share.
func TestCouponQuotaCountsOnlyCheckoutsStillStanding(t *testing.T) {
	db := newQuotaDB(t)
	repo := NewCouponRepo(db)
	ctx := context.Background()

	const (
		ours   = uint(7)
		other  = uint(8)
		buyer  = uint(101)
		late   = uint(103)
		cutoff = -2 * time.Hour
	)
	since := time.Now().Add(cutoff)

	insert := func(orderNo string, couponID, userID uint, status string, age time.Duration) *models.Order {
		id := couponID
		touch := time.Now().Add(age)
		order := &models.Order{
			Base:        models.Base{CreatedAt: touch},
			OrderNo:     orderNo,
			UserID:      userID,
			ProductID:   1,
			Quantity:    1,
			UnitPrice:   100,
			TotalAmount: 100,
			Status:      status,
			CouponID:    &id,
		}
		if err := db.Create(order).Error; err != nil {
			t.Fatalf("create %s: %v", orderNo, err)
		}
		return order
	}

	insert("fresh-checkout", ours, buyer, "pending", -30*time.Minute)
	insert("paid-long-ago", ours, 102, "paid", -30*24*time.Hour)
	insert("abandoned", ours, late, "pending", 3*cutoff)
	insert("cancelled", ours, 104, "cancelled", 0)
	insert("someone-elses-code", other, 105, "pending", 0)
	deleted := insert("deleted", ours, 106, "pending", 0)
	if err := db.Delete(&models.Order{}, deleted.ID).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	total, err := repo.UsedTotal(ctx, ours, since)
	if err != nil {
		t.Fatalf("UsedTotal: %v", err)
	}
	// Only the fresh checkout and the paid order stand: an old unpaid one is not
	// a sale, and neither a cancelled nor a deleted order ever was.
	if total != 2 {
		t.Errorf("UsedTotal = %d, want 2 (fresh pending + paid)", total)
	}

	byBuyer, err := repo.UsedBy(ctx, ours, buyer, since)
	if err != nil {
		t.Fatalf("UsedBy: %v", err)
	}
	if byBuyer != 1 {
		t.Errorf("UsedBy(fresh buyer) = %d, want 1", byBuyer)
	}
	// The account that walked away outlives its hold on 每人限用.
	byLeaver, err := repo.UsedBy(ctx, ours, late, since)
	if err != nil {
		t.Fatalf("UsedBy: %v", err)
	}
	if byLeaver != 0 {
		t.Errorf("UsedBy(buyer who abandoned) = %d, want 0", byLeaver)
	}
	// A code nobody has used is not short because another code is popular.
	byStranger, err := repo.UsedBy(ctx, other, 105, since)
	if err != nil {
		t.Fatalf("UsedBy: %v", err)
	}
	if byStranger != 1 {
		t.Errorf("UsedBy(other coupon) = %d, want 1", byStranger)
	}

	// The promo table asks for every code in one pass; that pass has to agree
	// with the per-code count the checkout enforces, or the back office shows a
	// different quota than the one buyers are actually refused against.
	all, err := repo.HeldByCoupons(ctx, since)
	if err != nil {
		t.Fatalf("HeldByCoupons: %v", err)
	}
	if all[ours] != total || all[other] != 1 {
		t.Errorf("HeldByCoupons = %v, want {%d:%d, %d:1}", all, ours, total, other)
	}
}
