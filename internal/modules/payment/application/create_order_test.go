package application

import (
	"context"
	"errors"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/domain"
)

// orderProductRepo implements only the product lookup and order write paths the
// checkout contract uses. The fake makes the selected row observable so a
// regression back to slug-only lookup fails here rather than in production.
type orderProductRepo struct {
	contract.OrderRepo
	byID    map[uint]*models.Product
	bySlug  map[string]*models.Product
	created []*models.Order
}

func (r *orderProductRepo) GetPurchasableProductByID(_ context.Context, id uint) (*models.Product, error) {
	if product := r.byID[id]; product != nil {
		return product, nil
	}
	return nil, domain.ErrProductNotPurchasable
}

func (r *orderProductRepo) GetPurchasableProductBySlug(_ context.Context, slug string) (*models.Product, error) {
	if product := r.bySlug[slug]; product != nil {
		return product, nil
	}
	return nil, domain.ErrProductNotPurchasable
}

func (r *orderProductRepo) CountAvailableCards(context.Context, uint) (int64, error) {
	return 999, nil
}

func (r *orderProductRepo) CreateOrder(_ context.Context, order *models.Order) error {
	r.created = append(r.created, order)
	return nil
}

type orderUserDirectory struct {
	contract.UserLookup
}

func (orderUserDirectory) FindByID(context.Context, uint) (*contract.UserInfo, error) {
	return &contract.UserInfo{ID: 7, Username: "buyer", IsActive: true}, nil
}

func checkoutService() (*Service, *orderProductRepo) {
	productA := &models.Product{
		Base:        models.Base{ID: 11},
		Slug:        "item-a",
		Name:        "商品 A",
		Price:       100,
		ProductType: "manual",
		IsPublished: true,
	}
	productB := &models.Product{
		Base:        models.Base{ID: 22},
		Slug:        "item-b",
		Name:        "商品 B",
		Price:       200,
		ProductType: "manual",
		IsPublished: true,
	}
	repo := &orderProductRepo{
		byID:   map[uint]*models.Product{11: productA, 22: productB},
		bySlug: map[string]*models.Product{"item-a": productA, "item-b": productB},
	}
	return &Service{
		orders:   repo,
		users:    orderUserDirectory{},
		features: config.FeaturesConfig{},
	}, repo
}

func TestCreateOrderUsesTheSelectedProductID(t *testing.T) {
	service, repo := checkoutService()
	order, err := service.CreateOrder(context.Background(), CreateOrderInput{
		UserID:    7,
		ProductID: 22,
		Slug:      "item-b",
		Quantity:  1,
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if order.ProductID != 22 || order.UnitPrice != 200 {
		t.Fatalf("order selected product %d at %d, want 22 at 200", order.ProductID, order.UnitPrice)
	}
	if len(repo.created) != 1 || repo.created[0].ProductID != 22 {
		t.Fatalf("created orders = %+v, want the selected product", repo.created)
	}
}

func TestCreateOrderRejectsAMismatchedIDAndSlug(t *testing.T) {
	service, _ := checkoutService()
	_, err := service.CreateOrder(context.Background(), CreateOrderInput{
		UserID:    7,
		ProductID: 11,
		Slug:      "item-b",
		Quantity:  1,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("mismatched identity returned %v, want ErrInvalidInput", err)
	}
}

func TestCreateOrderKeepsSlugCompatibility(t *testing.T) {
	service, _ := checkoutService()
	order, err := service.CreateOrder(context.Background(), CreateOrderInput{
		UserID:   7,
		Slug:     "item-a",
		Quantity: 1,
	})
	if err != nil {
		t.Fatalf("legacy slug checkout: %v", err)
	}
	if order.ProductID != 11 {
		t.Fatalf("legacy slug checkout selected product %d, want 11", order.ProductID)
	}
}

func TestCreateOrderDoesNotLetSlugOverrideAnExplicitID(t *testing.T) {
	service, repo := checkoutService()
	_, err := service.CreateOrder(context.Background(), CreateOrderInput{
		UserID:    7,
		ProductID: 11,
		Slug:      "item-b",
		Quantity:  1,
	})
	if err == nil {
		t.Fatal("a stale slug overrode the selected product ID")
	}
	if len(repo.created) != 0 {
		t.Fatalf("a mismatched request created %d orders, want none", len(repo.created))
	}
}

// newAPICheckoutService wires one New-API product with a 10–100 top-up window.
func newAPICheckoutService() (*Service, *orderProductRepo) {
	product := &models.Product{
		Base:            models.Base{ID: 33},
		Slug:            "topup",
		Name:            "New-API 充值",
		Price:           5,
		ProductType:     "manual",
		DeliveryChannel: "new_api",
		MinTopupAmount:  10,
		MaxTopupAmount:  100,
		// In production this field is contributed by the New-API plugin's form
		// schema; the checkout validates product-and-plugin fields together, so
		// mirroring it here exercises the same bounds gate.
		FormSchema:  `[{"key":"nl_amount","label":"本次充值额度","type":"number","required":true}]`,
		IsPublished: true,
	}
	repo := &orderProductRepo{
		byID:   map[uint]*models.Product{33: product},
		bySlug: map[string]*models.Product{"topup": product},
	}
	return &Service{
		orders:   repo,
		users:    orderUserDirectory{},
		features: config.FeaturesConfig{},
	}, repo
}

func TestNewAPIOrderStoresTopupAmountAndProductID(t *testing.T) {
	service, repo := newAPICheckoutService()
	order, err := service.CreateOrder(context.Background(), CreateOrderInput{
		UserID:    7,
		ProductID: 33,
		Slug:      "topup",
		Quantity:  1,
		FormValues: map[string]string{
			"nl_amount": "50",
		},
	})
	if err != nil {
		t.Fatalf("create New-API order: %v", err)
	}
	if order.ProductID != 33 {
		t.Fatalf("order product = %d, want 33", order.ProductID)
	}
	if order.TopupAmount != 50 {
		t.Fatalf("order topup = %d, want 50", order.TopupAmount)
	}
	if len(repo.created) != 1 || repo.created[0].TopupAmount != 50 {
		t.Fatalf("persisted orders = %+v, want topup 50", repo.created)
	}
}

func TestNewAPIOrderRejectsTopupOutsideBounds(t *testing.T) {
	for _, amount := range []string{"9", "101", "0", "-5", "abc", "3.5", ""} {
		service, repo := newAPICheckoutService()
		_, err := service.CreateOrder(context.Background(), CreateOrderInput{
			UserID:    7,
			ProductID: 33,
			Quantity:  1,
			FormValues: map[string]string{
				"nl_amount": amount,
			},
		})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("topup %q returned %v, want ErrInvalidInput", amount, err)
		}
		if len(repo.created) != 0 {
			t.Fatalf("topup %q created %d orders, want none", amount, len(repo.created))
		}
	}
}

func TestNewAPIOrderDoesNotRequireCardStock(t *testing.T) {
	service, _ := newAPICheckoutService()
	// CountAvailableCards in this fake returns 999; a New-API product must not
	// consult it at all, so a zero-stock product still orders successfully.
	order, err := service.CreateOrder(context.Background(), CreateOrderInput{
		UserID:    7,
		ProductID: 33,
		Quantity:  1,
		FormValues: map[string]string{
			"nl_amount": "10",
		},
	})
	if err != nil {
		t.Fatalf("New-API order was gated on card stock: %v", err)
	}
	if order.TopupAmount != 10 {
		t.Fatalf("order topup = %d, want 10", order.TopupAmount)
	}
}
