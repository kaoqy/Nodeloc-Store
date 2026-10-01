package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/catalog/domain"
)

// restockProducts is the slice of the catalogue a restock touches: the card
// product being stocked, the stock count the shop is told, and the low-stock
// queue the back office reads.
type restockProducts struct {
	product     *domain.Product
	lowStock    []domain.Product
	stockWrites []int
}

func (r *restockProducts) ListQuery(context.Context, domain.ProductQuery) ([]domain.Product, int64, error) {
	return nil, 0, nil
}
func (r *restockProducts) GetByID(context.Context, uint) (*domain.Product, error) {
	return r.product, nil
}
func (r *restockProducts) GetBySlug(context.Context, string, bool) (*domain.Product, error) {
	return nil, nil
}
func (r *restockProducts) Create(context.Context, *domain.Product) error { return nil }
func (r *restockProducts) Update(context.Context, *domain.Product) error { return nil }
func (r *restockProducts) Delete(context.Context, uint) error            { return nil }
func (r *restockProducts) UpdateStockCount(_ context.Context, _ uint, count int) error {
	r.stockWrites = append(r.stockWrites, count)
	return nil
}
func (r *restockProducts) CountByCategory(context.Context, bool) (map[uint]int64, error) {
	return nil, nil
}
func (r *restockProducts) Stats(context.Context, bool) (domain.StoreStats, error) {
	return domain.StoreStats{}, nil
}
func (r *restockProducts) ListLowStock(context.Context, int) ([]domain.Product, error) {
	return r.lowStock, nil
}

// restockCards holds what the shop imports, so the fake can answer the duplicate
// check and the stock recount the way GORM would.
type restockCards struct {
	available int64
	held      map[string]struct{}
	statuses  map[uint]string
	// created records every line the shop asked for, in order, because a repeat
	// that was stocked and a repeat that was dropped look the same in `held`.
	created []string
}

func (r *restockCards) ListByProduct(context.Context, uint) ([]domain.Card, error) {
	return nil, nil
}
func (r *restockCards) ListFiltered(context.Context, domain.CardFilter) ([]domain.Card, int64, error) {
	return nil, 0, nil
}
func (r *restockCards) GetByID(_ context.Context, id uint) (*domain.Card, error) {
	return &domain.Card{Base: models.Base{ID: id}, ProductID: 7, Content: "OLD", Status: domain.CardStatusDisabled}, nil
}
func (r *restockCards) Create(_ context.Context, card *domain.Card) error {
	r.held[card.Content] = struct{}{}
	r.created = append(r.created, card.Content)
	if card.Status == domain.CardStatusAvailable {
		r.available++
	}
	return nil
}
func (r *restockCards) CreateBatch(_ context.Context, cards []domain.Card) error {
	for i := range cards {
		if err := r.Create(context.Background(), &cards[i]); err != nil {
			return err
		}
	}
	return nil
}
func (r *restockCards) Update(_ context.Context, card *domain.Card) error {
	r.statuses[card.ID] = card.Status
	return nil
}
func (r *restockCards) Delete(context.Context, uint) error { return nil }
func (r *restockCards) CountByStatus(context.Context, uint, string) (int64, error) {
	return r.available, nil
}
func (r *restockCards) TakeAvailable(context.Context, uint, uint) (*domain.Card, error) {
	return nil, nil
}
func (r *restockCards) ExistingContents(context.Context, uint) (map[string]struct{}, error) {
	return r.held, nil
}
func (r *restockCards) BulkStatus(_ context.Context, _ uint, ids []uint, status string) (int64, error) {
	for _, id := range ids {
		r.statuses[id] = status
	}
	r.available += int64(len(ids))
	return int64(len(ids)), nil
}
func (r *restockCards) BulkDelete(context.Context, uint, []uint) (int64, error) { return 0, nil }

// wakeSpy is the payment side: it records which product's queue was released and
// can be made to fail, because a restock must survive that.
type wakeSpy struct {
	released  int
	err       error
	called    []uint
	counts    map[uint]int64
	countsErr error
}

func (w *wakeSpy) ReleaseProductBacklog(_ context.Context, productID uint) (int, error) {
	w.called = append(w.called, productID)
	return w.released, w.err
}

func (w *wakeSpy) WaitingOrdersByProduct(context.Context) (map[uint]int64, error) {
	return w.counts, w.countsErr
}

func restockService(products *restockProducts, cards *restockCards, wake deliveryWake) *Service {
	service := NewService(products, cards, nil, nil, config.FeaturesConfig{})
	service.SetDeliveryWake(wake)
	return service
}

func cardProduct() *domain.Product {
	return &domain.Product{
		Base: models.Base{ID: 7}, Slug: "keys", Name: "卡密商品", Price: 30,
		ProductType: domain.ProductTypeCard, AutoDeliver: true, IsPublished: true,
	}
}

func newRestockFakes() (*restockProducts, *restockCards) {
	return &restockProducts{product: cardProduct()}, &restockCards{held: map[string]struct{}{}, statuses: map[uint]string{}}
}

func TestImportCardsReleasesTheWaitingOrders(t *testing.T) {
	products, cards := newRestockFakes()
	wake := &wakeSpy{released: 3, counts: map[uint]int64{7: 3}}
	service := restockService(products, cards, wake)

	result, err := service.ImportCards(context.Background(), 7, []string{"A", "B", ""})
	if err != nil {
		t.Fatalf("ImportCards: %v", err)
	}
	if result.Released != 3 {
		t.Errorf("released = %d, want the three orders the restock delivered", result.Released)
	}
	if len(wake.called) != 1 || wake.called[0] != 7 {
		t.Errorf("wake called for %v, want product 7 once", wake.called)
	}
}

// TestImportCardsKeepsRepeatedKeys is the shop that sells the same 激活码 to
// everyone: one line per unit sold, so pasting the file twice must stock twice
// and only tell the operator how many lines repeated.
func TestImportCardsKeepsRepeatedKeys(t *testing.T) {
	products, cards := newRestockFakes()
	cards.held["A"] = struct{}{}
	service := restockService(products, cards, &wakeSpy{})

	result, err := service.ImportCards(context.Background(), 7, []string{"A", "A", "B", "   "})
	if err != nil {
		t.Fatalf("ImportCards: %v", err)
	}
	if len(result.Created) != 3 {
		t.Fatalf("created = %d lines (%v), want every non-blank line stocked", len(result.Created), result.Created)
	}
	if strings.Join(cards.created, ",") != "A,A,B" {
		t.Errorf("stocked %v, want A twice and B once", cards.created)
	}
	if result.Duplicates != 2 {
		t.Errorf("duplicates = %d, want the shelf's existing A and the second pasted A", result.Duplicates)
	}
	if result.Blank != 1 {
		t.Errorf("blank = %d, want the whitespace-only line reported", result.Blank)
	}
	if len(products.stockWrites) != 1 || products.stockWrites[0] != 3 {
		t.Errorf("stock recount = %v, want the three lines counted", products.stockWrites)
	}
}

func TestGenerateCardsReleasesTheWaitingOrders(t *testing.T) {
	products, cards := newRestockFakes()
	wake := &wakeSpy{released: 2}
	service := restockService(products, cards, wake)

	result, err := service.GenerateCards(context.Background(), 7, 4, "")
	if err != nil {
		t.Fatalf("GenerateCards: %v", err)
	}
	if len(result.Created) != 4 || result.Released != 2 {
		t.Errorf("created/released = %d/%d, want four keys and two orders released", len(result.Created), result.Released)
	}
}

func TestAddCardReleasesAndSkipsKeysThatStayedOff(t *testing.T) {
	products, cards := newRestockFakes()
	wake := &wakeSpy{released: 1}
	service := restockService(products, cards, wake)

	released, err := service.AddCard(context.Background(), 7, &domain.Card{Content: "SOLO"})
	if err != nil {
		t.Fatalf("AddCard: %v", err)
	}
	if released != 1 || len(wake.called) != 1 {
		t.Errorf("released/calls = %d/%v, want the new key to release its product's queue", released, wake.called)
	}

	// A key the shop imports already disabled is not stock, so nobody waiting
	// should be told their goods arrived.
	released, err = service.AddCard(context.Background(), 7, &domain.Card{Content: "OFF", Status: domain.CardStatusDisabled})
	if err != nil {
		t.Fatalf("AddCard disabled: %v", err)
	}
	if released != 0 || len(wake.called) != 1 {
		t.Errorf("disabled key released %d after %v calls, want no wake at all", released, wake.called)
	}
}

func TestPutBackOnTheShelfReleasesTheQueue(t *testing.T) {
	products, cards := newRestockFakes()
	wake := &wakeSpy{released: 1}
	service := restockService(products, cards, wake)

	if _, err := service.UpdateCard(context.Background(), 7, 11, &domain.Card{
		Content: "REVIVED", Status: domain.CardStatusAvailable,
	}); err != nil {
		t.Fatalf("UpdateCard: %v", err)
	}
	if len(wake.called) != 1 {
		t.Errorf("wake calls = %v, want a key put back on the shelf to release its product", wake.called)
	}

	if _, err := service.UpdateCard(context.Background(), 7, 11, &domain.Card{
		Content: "REVIVED", Status: domain.CardStatusDisabled,
	}); err != nil {
		t.Fatalf("UpdateCard disabled: %v", err)
	}
	if len(wake.called) != 1 {
		t.Errorf("wake calls = %v, want a key taken off the shelf to release nothing", wake.called)
	}

	moved, released, err := service.SetCardStatus(context.Background(), 7, []uint{11, 12}, domain.CardStatusAvailable)
	if err != nil {
		t.Fatalf("SetCardStatus: %v", err)
	}
	if moved != 2 || released != 1 || len(wake.called) != 2 {
		t.Errorf("moved/released/calls = %d/%d/%v, want the batch that went back on the shelf to release the queue",
			moved, released, wake.called)
	}

	if _, released, err = service.SetCardStatus(context.Background(), 7, []uint{11}, domain.CardStatusDisabled); err != nil {
		t.Fatalf("SetCardStatus disabled: %v", err)
	} else if released != 0 || len(wake.called) != 2 {
		t.Errorf("disabled batch released %d after %v calls, want nothing", released, wake.called)
	}
}

// A restock that cannot reach the payment side still restocks: the keys are on
// the shelf, the orders are intact, and the background sweep delivers them.
func TestRestockSurvivesAWakeThatFails(t *testing.T) {
	products, cards := newRestockFakes()
	wake := &wakeSpy{err: errors.New("database is locked")}
	service := restockService(products, cards, wake)

	result, err := service.ImportCards(context.Background(), 7, []string{"A"})
	if err != nil {
		t.Fatalf("ImportCards with a failing wake: %v", err)
	}
	if result.Released != 0 {
		t.Errorf("released = %d, want none counted when the wake failed", result.Released)
	}
	if len(products.stockWrites) == 0 {
		t.Error("stock was never recounted, want the import to finish before the wake")
	}

	if _, err := service.LowStockProducts(context.Background()); err != nil {
		t.Errorf("LowStockProducts with a failing count: %v", err)
	}
}

func TestLowStockCarriesTheWaitingCount(t *testing.T) {
	products, cards := newRestockFakes()
	// The store hands the queue over sorted by how empty each shelf is; buyers who
	// already paid have to reorder it, so the product with the longest queue is
	// listed last here on purpose.
	products.lowStock = []domain.Product{
		{Base: models.Base{ID: 8}, Slug: "other", Name: "另一件", Price: 20, ProductType: domain.ProductTypeCard},
		*cardProduct(),
	}
	wake := &wakeSpy{counts: map[uint]int64{7: 4, 8: 2}}
	service := restockService(products, cards, wake)

	rows, err := service.LowStockProducts(context.Background())
	if err != nil {
		t.Fatalf("LowStockProducts: %v", err)
	}
	if len(rows) != 2 || rows[0].ID != 7 || rows[0].WaitingOrders != 4 || rows[1].WaitingOrders != 2 {
		t.Errorf("rows = %+v, want product 7 (4 waiting) ahead of 8 (2 waiting)", rows)
	}
	if rows[0].Name != "卡密商品" {
		t.Errorf("row kept only the count, want the product itself: %+v", rows[0])
	}

	// Wired without the payment side, the queue still lists products — it just has
	// no numbers to sort by, so the store's own order stands.
	bare := restockService(products, cards, nil)
	rows, err = bare.LowStockProducts(context.Background())
	if err != nil {
		t.Fatalf("LowStockProducts without a wake: %v", err)
	}
	if len(rows) != 2 || rows[0].WaitingOrders != 0 || rows[0].ID != 8 {
		t.Errorf("rows = %+v, want the queue to list products with no waiting counts", rows)
	}
}
