package application

import (
	"context"
	"errors"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/catalog/domain"
)

func newAPIProduct() *domain.Product {
	return &domain.Product{
		Name:            "New-API 充值",
		Slug:            "topup",
		Price:           5,
		DeliveryChannel: domain.DeliveryChannelNewAPI,
		MinTopupAmount:  10,
		MaxTopupAmount:  100,
		IsPublished:     true,
		ProductType:     domain.ProductTypeCard,
		StockVisible:    true,
		StockCount:      50,
	}
}

func TestCreateNewAPIProductNeedsNoCardStock(t *testing.T) {
	repo := &restockProducts{}
	service := newPatchService(repo)
	product := newAPIProduct()
	if err := service.CreateProduct(context.Background(), product); err != nil {
		t.Fatalf("create New-API product: %v", err)
	}
	if product.ProductType != domain.ProductTypeManual || product.AutoDeliver {
		t.Fatalf("New-API product kept card delivery: %+v", product)
	}
	if product.StockCount != 0 || product.StockVisible {
		t.Fatalf("New-API product kept card stock: count=%d visible=%v", product.StockCount, product.StockVisible)
	}
	if product.MinTopupAmount != 10 || product.MaxTopupAmount != 100 {
		t.Fatalf("bounds = %d/%d, want 10/100", product.MinTopupAmount, product.MaxTopupAmount)
	}
}

func TestNewAPIProductRejectsInvertedBounds(t *testing.T) {
	repo := &restockProducts{}
	service := newPatchService(repo)
	product := newAPIProduct()
	product.MinTopupAmount = 200
	product.MaxTopupAmount = 100
	err := service.CreateProduct(context.Background(), product)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("inverted bounds returned %v, want ErrInvalidInput", err)
	}
}

func TestNewAPIProductRejectsMissingBounds(t *testing.T) {
	for _, bounds := range [][2]int{{0, 100}, {10, 0}, {0, 0}, {-1, 100}} {
		repo := &restockProducts{}
		service := newPatchService(repo)
		product := newAPIProduct()
		product.MinTopupAmount = bounds[0]
		product.MaxTopupAmount = bounds[1]
		err := service.CreateProduct(context.Background(), product)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("bounds %v returned %v, want ErrInvalidInput", bounds, err)
		}
	}
}

// A New-API product with no price would save here and then fail every checkout
// with a vague "not purchasable"; the channel has no amount→price rule, so the
// save must refuse it up front and name the fix.
func TestNewAPIProductRejectsZeroPrice(t *testing.T) {
	repo := &restockProducts{}
	service := newPatchService(repo)
	product := newAPIProduct()
	product.Price = 0
	err := service.CreateProduct(context.Background(), product)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("zero-price New-API product returned %v, want ErrInvalidInput", err)
	}
}

func TestCardProductClearsTopupBounds(t *testing.T) {
	repo := &restockProducts{}
	service := newPatchService(repo)
	product := &domain.Product{
		Name:            "卡密商品",
		Slug:            "card",
		Price:           10,
		DeliveryChannel: domain.DeliveryChannelCard,
		ProductType:     domain.ProductTypeCard,
		MinTopupAmount:  10,
		MaxTopupAmount:  100,
		IsPublished:     true,
	}
	if err := service.CreateProduct(context.Background(), product); err != nil {
		t.Fatalf("create card product: %v", err)
	}
	if product.MinTopupAmount != 0 || product.MaxTopupAmount != 0 {
		t.Fatalf("card product kept topup bounds: %d/%d", product.MinTopupAmount, product.MaxTopupAmount)
	}
}

// Legacy rows have no delivery_channel; the channel must be derived from
// product_type so an existing card product is not silently reclassified.
func TestLegacyProductDerivesChannelFromType(t *testing.T) {
	repo := &restockProducts{}
	service := newPatchService(repo)
	product := &domain.Product{
		Name:        "旧卡密",
		Slug:        "legacy",
		Price:       10,
		ProductType: domain.ProductTypeCard,
		IsPublished: true,
	}
	if err := service.CreateProduct(context.Background(), product); err != nil {
		t.Fatalf("create legacy product: %v", err)
	}
	if product.DeliveryChannel != domain.DeliveryChannelCard {
		t.Fatalf("legacy channel = %q, want card", product.DeliveryChannel)
	}

	manual := &domain.Product{
		Name:        "旧人工",
		Slug:        "legacy-manual",
		Price:       10,
		ProductType: domain.ProductTypeManual,
		IsPublished: true,
	}
	if err := service.CreateProduct(context.Background(), manual); err != nil {
		t.Fatalf("create legacy manual product: %v", err)
	}
	if manual.DeliveryChannel != domain.DeliveryChannelManual {
		t.Fatalf("legacy manual channel = %q, want manual", manual.DeliveryChannel)
	}
}
