package application

import (
	"context"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/catalog/domain"
)

func newPatchService(repo *restockProducts) *Service {
	return NewService(repo, nil, nil, nil, config.FeaturesConfig{})
}

// TestPatchUpdateOnlyTouchesListedFields is the regression this guards: the back
// office list flips one boolean, and an early version overwrote the whole row
// with the zero values it did not send, wiping the product's name and price.
func TestPatchUpdateOnlyTouchesListedFields(t *testing.T) {
	product := &domain.Product{
		Base:         models.Base{ID: 7},
		Name:         "原商品",
		Slug:         "original",
		Price:        100,
		ProductType:  domain.ProductTypeCard,
		IsPublished:  true,
		StockVisible: true,
		AutoDeliver:  true,
	}
	repo := &restockProducts{product: product}
	service := newPatchService(repo)

	// The list page sends only the flag it flipped.
	_, err := service.UpdateProductPatch(context.Background(), 7, &domain.Product{IsPublished: false}, map[string]bool{"is_published": true})
	if err != nil {
		t.Fatalf("patch update: %v", err)
	}
	if repo.product.Name != "原商品" {
		t.Fatalf("name was wiped: %q", repo.product.Name)
	}
	if repo.product.Price != 100 {
		t.Fatalf("price was wiped: %d", repo.product.Price)
	}
	if repo.product.IsPublished {
		t.Fatal("is_published was not updated")
	}
	if repo.product.Slug != "original" {
		t.Fatalf("slug was wiped: %q", repo.product.Slug)
	}
}

// A full form submission still replaces every field, which is what the edit
// screen expects.
func TestFullUpdateStillReplacesFields(t *testing.T) {
	repo := &restockProducts{product: &domain.Product{
		Base: models.Base{ID: 9}, Name: "旧名", Slug: "old", Price: 10,
		ProductType: domain.ProductTypeCard,
	}}
	service := newPatchService(repo)
	next := &domain.Product{
		Name: "新名", Slug: "new", Price: 20, ProductType: domain.ProductTypeCard,
		StockVisible: true, AutoDeliver: true, IsPublished: true,
	}
	if _, err := service.UpdateProductPatch(context.Background(), 9, next, nil); err != nil {
		t.Fatalf("full update: %v", err)
	}
	if repo.product.Name != "新名" || repo.product.Price != 20 || repo.product.Slug != "new" {
		t.Fatalf("full update did not replace fields: %+v", repo.product)
	}
}
