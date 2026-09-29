package infrastructure

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/catalog/domain"
)

func newFlagTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.Category{}, &domain.Product{}, &domain.Coupon{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// A column that declares DEFAULT true makes GORM drop the field from the INSERT
// whenever the value is false — so "hide this category", "unpublish this product"
// and "create the code switched off" all silently store the opposite. The flags
// below have to be written out; this test is the guard against re-adding a
// default to any of them.
func TestFalseFlagsSurviveCreate(t *testing.T) {
	db := newFlagTestDB(t)
	ctx := context.Background()

	category := &domain.Category{Slug: "hidden", Name: "Hidden", IsVisible: false}
	product := &domain.Product{
		Slug: "off", Name: "Off", Price: 10, ProductType: domain.ProductTypeManual,
		StockVisible: false, IsPublished: false, AutoDeliver: false,
	}
	coupon := &domain.Coupon{
		Code: "DRAFT", DiscountType: "fixed", DiscountValue: 10,
		IsActive: false, Advertised: false, Scope: "all",
	}
	for _, row := range []any{category, product, coupon} {
		if err := db.WithContext(ctx).Create(row).Error; err != nil {
			t.Fatalf("create %T: %v", row, err)
		}
	}

	var gotCategory domain.Category
	if err := db.First(&gotCategory, "slug = ?", "hidden").Error; err != nil {
		t.Fatal(err)
	}
	if gotCategory.IsVisible {
		t.Error("hidden category came back visible")
	}

	var gotProduct domain.Product
	if err := db.First(&gotProduct, "slug = ?", "off").Error; err != nil {
		t.Fatal(err)
	}
	if gotProduct.StockVisible || gotProduct.IsPublished || gotProduct.AutoDeliver {
		t.Errorf("unpublished product came back visible=%v published=%v autoDeliver=%v",
			gotProduct.StockVisible, gotProduct.IsPublished, gotProduct.AutoDeliver)
	}

	var gotCoupon domain.Coupon
	if err := db.First(&gotCoupon, "code = ?", "DRAFT").Error; err != nil {
		t.Fatal(err)
	}
	if gotCoupon.IsActive || gotCoupon.Advertised {
		t.Errorf("draft coupon came back active=%v advertised=%v", gotCoupon.IsActive, gotCoupon.Advertised)
	}
}

// The other direction, because a flag that never stored true would pass the test
// above just as loudly.
func TestTrueFlagsSurviveCreate(t *testing.T) {
	db := newFlagTestDB(t)

	category := &domain.Category{Slug: "shown", Name: "Shown", IsVisible: true}
	product := &domain.Product{
		Slug: "live", Name: "Live", Price: 10, ProductType: domain.ProductTypeCard,
		StockVisible: true, IsPublished: true, AutoDeliver: true,
	}
	coupon := &domain.Coupon{
		Code: "LIVE", DiscountType: "fixed", DiscountValue: 10,
		IsActive: true, Advertised: true, Scope: "all",
	}
	for _, row := range []any{category, product, coupon} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("create %T: %v", row, err)
		}
	}

	if err := db.First(&category, "slug = ?", "shown").Error; err != nil {
		t.Fatal(err)
	}
	if !category.IsVisible {
		t.Error("visible category came back hidden")
	}
	if err := db.First(&product, "slug = ?", "live").Error; err != nil {
		t.Fatal(err)
	}
	if !product.StockVisible || !product.IsPublished || !product.AutoDeliver {
		t.Errorf("live product came back visible=%v published=%v autoDeliver=%v",
			product.StockVisible, product.IsPublished, product.AutoDeliver)
	}
	if err := db.First(&coupon, "code = ?", "LIVE").Error; err != nil {
		t.Fatal(err)
	}
	if !coupon.IsActive || !coupon.Advertised {
		t.Errorf("live coupon came back active=%v advertised=%v", coupon.IsActive, coupon.Advertised)
	}
}

// The promo shelf is judged in SQL, so it has to see exactly the two switches.
func TestListAdvertisedSelectsOnTheStoredFlags(t *testing.T) {
	db := newFlagTestDB(t)
	repo := &GormCouponRepo{db: db}
	ctx := context.Background()

	shelf := &domain.Coupon{Code: "SHELF", DiscountType: "fixed", DiscountValue: 10, IsActive: true, Advertised: true, Scope: "all"}
	private := &domain.Coupon{Code: "PRIVATE", DiscountType: "fixed", DiscountValue: 10, IsActive: true, Advertised: false, Scope: "all"}
	paused := &domain.Coupon{Code: "PAUSED", DiscountType: "fixed", DiscountValue: 10, IsActive: false, Advertised: true, Scope: "all"}
	for _, row := range []*domain.Coupon{shelf, private, paused} {
		if err := repo.Create(ctx, row); err != nil {
			t.Fatalf("create %s: %v", row.Code, err)
		}
	}

	got, err := repo.ListAdvertised(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Code != "SHELF" {
		t.Errorf("ListAdvertised() = %v, want only SHELF", codes(got))
	}
}

func codes(coupons []domain.Coupon) []string {
	out := make([]string, 0, len(coupons))
	for _, coupon := range coupons {
		out = append(out, coupon.Code)
	}
	return out
}
