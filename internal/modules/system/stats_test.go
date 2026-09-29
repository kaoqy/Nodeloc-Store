package system

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
)

func newStatsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Product{}, &models.Card{}, &models.Order{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func stockProduct(t *testing.T, db *gorm.DB, slug string, available, sold int) *models.Product {
	t.Helper()
	product := &models.Product{
		Slug: slug, Name: slug, Price: 10, ProductType: "card",
		IsPublished: true, AutoDeliver: true,
	}
	if err := db.Create(product).Error; err != nil {
		t.Fatal(err)
	}
	content := 0
	add := func(status string, n int) {
		for i := 0; i < n; i++ {
			content++
			card := &models.Card{ProductID: product.ID, Content: slug + "-" + string(rune('a'+content)), Status: status}
			if status == "sold" {
				now := time.Now()
				card.SoldAt = &now
			}
			if err := db.Create(card).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	add("available", available)
	add("sold", sold)
	return product
}

func paidOrder(db *gorm.DB, productID uint, orderNo, fulfillment string) *models.Order {
	order := &models.Order{
		OrderNo: orderNo, UserID: 1, ProductID: productID, Quantity: 1,
		UnitPrice: 10, TotalAmount: 10, Status: "paid", FulfillmentStatus: fulfillment,
	}
	order.CreatedAt = time.Now()
	return order
}

// The panel only has room for six products, so the queue that owes buyers has to
// beat the shelf that merely looks empty. Before the waiting count existed the
// ordering was purely by remaining keys, which is why this is guarded here.
func TestStockAlertsPutOwedOrdersFirst(t *testing.T) {
	db := newStatsTestDB(t)
	ctx := context.Background()

	owes := stockProduct(t, db, "owes", 2, 1)
	empty := stockProduct(t, db, "empty", 0, 4)
	quiet := stockProduct(t, db, "quiet", 1, 0)

	if err := db.Create([]*models.Order{
		paidOrder(db, owes.ID, "OWES-1", "waiting_stock"),
		paidOrder(db, owes.ID, "OWES-2", "waiting_stock"),
		paidOrder(db, owes.ID, "OWES-3", "pending"),
		// Already served, and one that never paid: neither is a restock debt.
		paidOrder(db, owes.ID, "OWES-4", "delivered"),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Order{
		OrderNo: "UNPAID-1", UserID: 1, ProductID: owes.ID, Quantity: 1,
		UnitPrice: 10, TotalAmount: 10, Status: "pending", FulfillmentStatus: "pending",
	}).Error; err != nil {
		t.Fatal(err)
	}

	stats := &DashboardStats{StockAlertThreshold: 3}
	if err := statStockAlerts(ctx, db, stats, time.Now()); err != nil {
		t.Fatal(err)
	}

	if len(stats.StockAlerts) != 3 {
		t.Fatalf("got %d alerts, want the three products under the threshold: %+v", len(stats.StockAlerts), stats.StockAlerts)
	}
	first := stats.StockAlerts[0]
	if first.ProductID != owes.ID || first.Waiting != 3 {
		t.Fatalf("first alert is %+v, want the product with three paid orders waiting", first)
	}
	if first.Available != 2 || first.Sold != 1 {
		t.Fatalf("stock numbers are %+v, want 2 available and 1 sold", first)
	}
	if got := stats.StockAlerts[1].ProductID; got != empty.ID {
		t.Fatalf("second alert is product %d, want the empty shelf %d", got, empty.ID)
	}
	if got := stats.StockAlerts[1].Waiting; got != 0 {
		t.Fatalf("empty shelf reports %d waiting, want 0", got)
	}
	if got := stats.StockAlerts[2].ProductID; got != quiet.ID {
		t.Fatalf("third alert is product %d, want %d", got, quiet.ID)
	}
}
