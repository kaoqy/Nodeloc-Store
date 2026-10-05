package application

import (
	"context"
	"errors"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
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

// New-API is an amount-type channel: it has no product price, and saving one
// with price 0 must succeed on the product level. The missing chargeable-amount
// rule is reported at checkout, not by rejecting the product.
func TestNewAPIProductSavesWithoutPrice(t *testing.T) {
	repo := &restockProducts{}
	service := newPatchService(repo)
	product := newAPIProduct()
	product.Price = 0
	if err := service.CreateProduct(context.Background(), product); err != nil {
		t.Fatalf("price-less New-API product was refused: %v", err)
	}
	if product.Price != 0 {
		t.Fatalf("product price = %d, want 0 for the amount-type channel", product.Price)
	}
}

// A legacy New-API row may carry a price that currently makes it orderable. A
// full edit from the amount-type screen sends no price (0); that must not wipe
// the stored value and silently break a product that was working.
func TestNewAPIFullEditKeepsLegacyPrice(t *testing.T) {
	repo := &restockProducts{product: &domain.Product{
		Base:            models.Base{ID: 5},
		Name:            "New-API 充值",
		Slug:            "topup",
		Price:           7,
		DeliveryChannel: domain.DeliveryChannelNewAPI,
		MinTopupAmount:  10,
		MaxTopupAmount:  100,
		IsPublished:     true,
	}}
	service := newPatchService(repo)
	next := &domain.Product{
		Name:            "New-API 充值",
		Slug:            "topup",
		Price:           0,
		DeliveryChannel: domain.DeliveryChannelNewAPI,
		MinTopupAmount:  20,
		MaxTopupAmount:  200,
		IsPublished:     true,
	}
	if _, err := service.UpdateProductPatch(context.Background(), 5, next, nil); err != nil {
		t.Fatalf("full edit: %v", err)
	}
	if repo.product.Price != 7 {
		t.Fatalf("legacy price was wiped: %d", repo.product.Price)
	}
	if repo.product.MinTopupAmount != 20 || repo.product.MaxTopupAmount != 200 {
		t.Fatalf("amount bounds not saved: %d/%d", repo.product.MinTopupAmount, repo.product.MaxTopupAmount)
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
