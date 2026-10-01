package infrastructure

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/contract"
)

func newRestockDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Product{}, &models.Card{}, &models.Order{}, &models.DeliveryRecord{}); err != nil {
		t.Fatal(err)
	}
	return db
}

type restockNotifier struct {
	events []contract.BuyerEvent
}

func (n *restockNotifier) Publish(_ context.Context, event contract.BuyerEvent) {
	n.events = append(n.events, event)
}

type restockGateway struct{}

func (restockGateway) CreatePayment(context.Context, contract.CreatePaymentRequest) (*contract.CreatePaymentResult, error) {
	return nil, nil
}
func (restockGateway) QueryPayment(context.Context, contract.QueryPaymentRequest) (*contract.QueryPaymentResult, error) {
	return nil, nil
}
func (restockGateway) SigningStyle() string { return "" }
func (restockGateway) Transfer(context.Context, contract.TransferRequest) (*contract.TransferResult, error) {
	return nil, nil
}
func (restockGateway) VerifyCallback(map[string]string) bool { return false }

type restockLookup struct{}

func (restockLookup) FindByID(context.Context, uint) (*contract.UserInfo, error) { return nil, nil }

type restockPricing struct{}

func (restockPricing) DiscountFor(context.Context, uint, uint, int, int, string) (int, uint, error) {
	return 0, 0, nil
}

// stock puts keys on the shelf and recounts it the way the catalogue's SyncStock
// would, so the delivery path sees a consistent product row.
func stock(t *testing.T, db *gorm.DB, productID uint, contents ...string) {
	t.Helper()
	for _, content := range contents {
		card := &models.Card{ProductID: productID, Content: content, Status: "available"}
		if err := db.Create(card).Error; err != nil {
			t.Fatalf("stock %s: %v", content, err)
		}
	}
	var available int64
	if err := db.Model(&models.Card{}).Where("product_id = ? AND status = ?", productID, "available").
		Count(&available).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.Product{}).Where("id = ?", productID).
		UpdateColumn("stock_count", available).Error; err != nil {
		t.Fatalf("sync stock for product %d: %v", productID, err)
	}
}

func loadOrder(t *testing.T, db *gorm.DB, orderNo string) *models.Order {
	t.Helper()
	var order models.Order
	if err := db.Where("order_no = ?", orderNo).First(&order).Error; err != nil {
		t.Fatalf("load %s: %v", orderNo, err)
	}
	return &order
}

// TestRestockDeliversWhatWasWaitingForIt is the whole point of the feature: a
// buyer who paid into an empty shelf gets their key the moment the shop restocks
// it, not whenever the background sweep next runs. It also checks the two ways
// that can go wrong — releasing another product's queue, and delivering an order
// nobody paid for.
func TestRestockDeliversWhatWasWaitingForIt(t *testing.T) {
	db := newRestockDB(t)
	ctx := context.Background()
	store := NewGormStore(db)
	notifier := &restockNotifier{}
	service := application.NewService(store, restockGateway{}, store, restockLookup{}, restockPricing{}, notifier, "pay")

	keyProduct := &models.Product{Slug: "keys", Name: "卡密商品", Price: 30, ProductType: "card", AutoDeliver: true, IsPublished: true}
	otherProduct := &models.Product{Slug: "other", Name: "另一件商品", Price: 30, ProductType: "card", AutoDeliver: true, IsPublished: true}
	for _, product := range []*models.Product{keyProduct, otherProduct} {
		if err := db.Create(product).Error; err != nil {
			t.Fatalf("create product %s: %v", product.Slug, err)
		}
	}

	paidAt := time.Now().UTC().Add(-2 * time.Hour)
	orders := []models.Order{
		{OrderNo: "WAIT-1", UserID: 5, ProductID: keyProduct.ID, Quantity: 1, UnitPrice: 30, TotalAmount: 30, Status: "paid", PaidAt: &paidAt},
		{OrderNo: "WAIT-2", UserID: 6, ProductID: keyProduct.ID, Quantity: 1, UnitPrice: 30, TotalAmount: 30, Status: "paid", PaidAt: &paidAt},
		{OrderNo: "OTHER-1", UserID: 7, ProductID: otherProduct.ID, Quantity: 1, UnitPrice: 30, TotalAmount: 30, Status: "paid", PaidAt: &paidAt},
		// Nobody paid for this one: 待支付 must never be delivered, however empty
		// the shelf is and however long it has sat there.
		{OrderNo: "UNPAID-1", UserID: 8, ProductID: keyProduct.ID, Quantity: 1, UnitPrice: 30, TotalAmount: 30},
	}
	for i := range orders {
		if err := db.Create(&orders[i]).Error; err != nil {
			t.Fatalf("create order %s: %v", orders[i].OrderNo, err)
		}
	}

	// Three paid orders meet an empty shelf and are parked in it, the way the
	// checkout path parks them.
	for _, orderNo := range []string{"WAIT-1", "WAIT-2", "OTHER-1"} {
		order := loadOrder(t, db, orderNo)
		if err := store.Fulfill(ctx, order); err != nil {
			t.Fatalf("fulfill %s: %v", orderNo, err)
		}
		if order.FulfillmentStatus != "waiting_stock" {
			t.Fatalf("%s landed in %q, want waiting_stock", orderNo, order.FulfillmentStatus)
		}
	}

	counts, err := service.WaitingOrdersByProduct(ctx)
	if err != nil {
		t.Fatalf("WaitingOrdersByProduct: %v", err)
	}
	if counts[keyProduct.ID] != 2 || counts[otherProduct.ID] != 1 || len(counts) != 2 {
		t.Errorf("waiting counts = %v, want 2 for the empty shelf and 1 for the other product", counts)
	}

	// One key arrives. The oldest payment takes it, the second keeps waiting, and
	// the other product's queue must not move at all.
	stock(t, db, keyProduct.ID, "KEY-1")
	released, err := service.ReleaseProductBacklog(ctx, keyProduct.ID)
	if err != nil {
		t.Fatalf("ReleaseProductBacklog: %v", err)
	}
	if released != 1 {
		t.Fatalf("released = %d, want the one key to go to the one oldest order", released)
	}

	first := loadOrder(t, db, "WAIT-1")
	if first.FulfillmentStatus != "delivered" {
		t.Errorf("WAIT-1 = %q, want delivered by the restock", first.FulfillmentStatus)
	}
	if first.DeliveryContent == nil || *first.DeliveryContent != "KEY-1" {
		t.Errorf("WAIT-1 content = %v, want the key that was just stocked", first.DeliveryContent)
	}
	if status := loadOrder(t, db, "WAIT-2").FulfillmentStatus; status != "waiting_stock" {
		t.Errorf("WAIT-2 = %q, want it still waiting while one key covers one order", status)
	}
	if status := loadOrder(t, db, "OTHER-1").FulfillmentStatus; status != "waiting_stock" {
		t.Errorf("OTHER-1 = %q, want another product's queue left alone", status)
	}
	unpaid := loadOrder(t, db, "UNPAID-1")
	if unpaid.Status != "pending" || unpaid.FulfillmentStatus != "pending" {
		t.Errorf("UNPAID-1 = %s/%s, want an unpaid order untouched", unpaid.Status, unpaid.FulfillmentStatus)
	}

	sold := &models.Card{}
	if err := db.Where("content = ?", "KEY-1").First(sold).Error; err != nil {
		t.Fatal(err)
	}
	if sold.Status != "sold" || sold.OrderID == nil || *sold.OrderID != first.ID {
		t.Errorf("KEY-1 = %s/%v, want it sold to WAIT-1", sold.Status, sold.OrderID)
	}
	var restocked models.Product
	if err := db.First(&restocked, keyProduct.ID).Error; err != nil {
		t.Fatal(err)
	}
	if restocked.StockCount != 0 || restocked.SoldCount != 1 {
		t.Errorf("product stock/sold = %d/%d, want the delivered key taken out of stock and counted as a sale",
			restocked.StockCount, restocked.SoldCount)
	}

	// The buyer of WAIT-1 is told the goods arrived, not only that the money did.
	if len(notifier.events) != 1 || notifier.events[0].UserID != first.UserID || notifier.events[0].Title != "商品已交付" {
		t.Fatalf("events = %+v, want one 商品已交付 addressed to WAIT-1's buyer", notifier.events)
	}

	counts, err = service.WaitingOrdersByProduct(ctx)
	if err != nil {
		t.Fatalf("WaitingOrdersByProduct after the first delivery: %v", err)
	}
	if counts[keyProduct.ID] != 1 {
		t.Errorf("waiting for the key product = %d, want the one order still short", counts[keyProduct.ID])
	}

	// Restocking again releases what remains, and a release over an empty queue
	// is simply nothing to do.
	stock(t, db, keyProduct.ID, "KEY-2", "KEY-3")
	released, err = service.ReleaseProductBacklog(ctx, keyProduct.ID)
	if err != nil {
		t.Fatalf("second ReleaseProductBacklog: %v", err)
	}
	if released != 1 {
		t.Errorf("released = %d, want WAIT-2 delivered now", released)
	}
	if status := loadOrder(t, db, "WAIT-2").FulfillmentStatus; status != "delivered" {
		t.Errorf("WAIT-2 = %q, want delivered", status)
	}
	if released, err = service.ReleaseProductBacklog(ctx, keyProduct.ID); err != nil || released != 0 {
		t.Errorf("empty queue release = %d/%v, want nothing left to release", released, err)
	}
	if released, err := service.ReleaseProductBacklog(ctx, 0); err != nil || released != 0 {
		t.Errorf("release without a product = %d/%v, want a harmless no-op", released, err)
	}
}

// TestRestockDoesNotResellToAnOrderThatAlreadyHasItsCards covers the retry the
// shop owner cannot see: the same order released twice.
func TestRestockDoesNotResellToAnOrderThatAlreadyHasItsCards(t *testing.T) {
	db := newRestockDB(t)
	ctx := context.Background()
	store := NewGormStore(db)
	service := application.NewService(store, restockGateway{}, store, restockLookup{}, restockPricing{}, &restockNotifier{}, "pay")

	product := &models.Product{Slug: "keys", Name: "卡密商品", Price: 30, ProductType: "card", AutoDeliver: true, IsPublished: true}
	if err := db.Create(product).Error; err != nil {
		t.Fatal(err)
	}
	paidAt := time.Now().UTC().Add(-time.Hour)
	order := &models.Order{
		OrderNo: "TWICE", UserID: 5, ProductID: product.ID, Quantity: 2,
		UnitPrice: 30, TotalAmount: 60, Status: "paid", PaidAt: &paidAt,
	}
	if err := db.Create(order).Error; err != nil {
		t.Fatal(err)
	}
	stock(t, db, product.ID, "KEY-A", "KEY-B")
	if err := store.Fulfill(ctx, order); err != nil {
		t.Fatalf("first fulfill: %v", err)
	}

	stock(t, db, product.ID, "KEY-C")
	released, err := service.ReleaseProductBacklog(ctx, product.ID)
	if err != nil {
		t.Fatalf("ReleaseProductBacklog: %v", err)
	}
	if released != 0 {
		t.Errorf("released = %d, want a delivered order handed nothing again", released)
	}
	var sold int64
	if err := db.Model(&models.Card{}).Where("product_id = ? AND status = ?", product.ID, "sold").Count(&sold).Error; err != nil {
		t.Fatal(err)
	}
	if sold != 2 {
		t.Errorf("sold cards = %d, want the two this order paid for and no third", sold)
	}
}
