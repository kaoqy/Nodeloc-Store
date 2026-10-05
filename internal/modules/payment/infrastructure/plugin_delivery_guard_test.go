package infrastructure

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/domain"
)

func newGuardDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Order{}, &models.DeliveryRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// MarkOrderPluginDelivering is the concurrency gate for redemption creation:
// the first caller wins and moves the order into the generating state; a second
// concurrent caller must be refused so it cannot call the upstream again.
func TestMarkOrderPluginDeliveringIsExclusive(t *testing.T) {
	db := newGuardDB(t)
	store := NewGormStore(db)
	order := &models.Order{
		OrderNo:           "NL-GUARD-1",
		UserID:            7,
		ProductID:         3,
		Quantity:          1,
		UnitPrice:         5,
		TotalAmount:       5,
		Status:            "paid",
		FulfillmentStatus: "pending",
	}
	if err := db.Create(order).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}

	if err := store.MarkOrderPluginDelivering(context.Background(), order.OrderNo); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	var claimed models.Order
	if err := db.Where("order_no = ?", order.OrderNo).First(&claimed).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if claimed.FulfillmentStatus != "plugin_pending" {
		t.Fatalf("status = %q, want plugin_pending", claimed.FulfillmentStatus)
	}

	if err := store.MarkOrderPluginDelivering(context.Background(), order.OrderNo); !errors.Is(err, domain.ErrDeliveryInProgress) {
		t.Fatalf("second claim returned %v, want ErrDeliveryInProgress", err)
	}
}

// Once an order is parked for review, another automatic attempt must not claim
// it either: a human has to confirm before upstream is called again.
func TestMarkOrderPluginDeliveringRefusesReview(t *testing.T) {
	db := newGuardDB(t)
	store := NewGormStore(db)
	order := &models.Order{
		OrderNo:           "NL-GUARD-2",
		UserID:            7,
		ProductID:         3,
		Quantity:          1,
		UnitPrice:         5,
		TotalAmount:       5,
		Status:            "paid",
		FulfillmentStatus: "plugin_review",
	}
	if err := db.Create(order).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}
	if err := store.MarkOrderPluginDelivering(context.Background(), order.OrderNo); !errors.Is(err, domain.ErrDeliveryInProgress) {
		t.Fatalf("review order was re-claimed: %v", err)
	}
}
