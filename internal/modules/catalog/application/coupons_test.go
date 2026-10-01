package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/catalog/domain"
)

// shelfRepo is the only half of the catalogue the promo shelf reads: the coupons
// and the live order counts behind a code's remaining quota.
type shelfRepo struct {
	advertised []domain.Coupon
	used       map[uint]int64
	// byCode is what a buyer types and the repo finds, advertised or not, so a
	// test can hand the storefront a private code and see what it says.
	byCode map[string]*domain.Coupon
}

func (r *shelfRepo) List(context.Context) ([]domain.Coupon, error) { return nil, nil }
func (r *shelfRepo) ListAdvertised(context.Context) ([]domain.Coupon, error) {
	return r.advertised, nil
}
func (r *shelfRepo) GetByID(context.Context, uint) (*domain.Coupon, error) { return nil, nil }
func (r *shelfRepo) GetByCode(_ context.Context, code string) (*domain.Coupon, error) {
	if coupon, found := r.byCode[code]; found {
		return coupon, nil
	}
	return nil, nil
}
func (r *shelfRepo) Create(context.Context, *domain.Coupon) error { return nil }
func (r *shelfRepo) Update(context.Context, *domain.Coupon) error { return nil }
func (r *shelfRepo) Delete(context.Context, uint) error           { return nil }
func (r *shelfRepo) UsedBy(context.Context, uint, uint, time.Time) (int64, error) {
	return 0, nil
}
func (r *shelfRepo) UsedTotal(_ context.Context, couponID uint, _ time.Time) (int64, error) {
	return r.used[couponID], nil
}

func (r *shelfRepo) HeldByCoupons(_ context.Context, _ time.Time) (map[uint]int64, error) {
	held := make(map[uint]int64, len(r.used))
	for couponID, count := range r.used {
		held[couponID] = count
	}
	return held, nil
}

func at(value time.Time) *time.Time { return &value }

func TestStorefrontCouponsKeepsOnlyWhatCanBeUsed(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	repo := &shelfRepo{
		used: map[uint]int64{4: 3, 6: 3},
		advertised: []domain.Coupon{
			{Base: models.Base{ID: 1}, Code: "LATER", DiscountType: "fixed", DiscountValue: 100, ValidFrom: at(now.Add(time.Hour))},
			{Base: models.Base{ID: 2}, Code: "OVER", DiscountType: "fixed", DiscountValue: 100, ValidUntil: at(now.Add(-time.Minute))},
			{Base: models.Base{ID: 6}, Code: "RUNOUT", DiscountType: "fixed", DiscountValue: 100, MaxUses: 3},
			{Base: models.Base{ID: 4}, Code: "TWOLEFT", DiscountType: "percent", DiscountValue: 10, MaxUses: 5},
			{Base: models.Base{ID: 5}, Code: "OPEN", DiscountType: "fixed", DiscountValue: 500, Scope: "category", CategoryID: number(3), Description: text("新人立减")},
		},
	}
	service := NewService(nil, nil, nil, repo, config.FeaturesConfig{})
	service.now = func() time.Time { return now }

	list, err := service.StorefrontCoupons(context.Background())
	if err != nil {
		t.Fatalf("StorefrontCoupons: %v", err)
	}
	// OVER expired a minute ago, LATER has not started, and RUNOUT has all three
	// live orders against it — none of them belongs on the shelf.
	if len(list) != 2 || list[0].Code != "TWOLEFT" || list[1].Code != "OPEN" {
		t.Fatalf("shelf = %+v, want TWOLEFT then OPEN", list)
	}
	if list[0].Remaining == nil || *list[0].Remaining != 2 {
		t.Errorf("TWOLEFT remaining = %v, want 2 live uses left", list[0].Remaining)
	}
	if list[1].Remaining != nil {
		t.Errorf("OPEN remaining = %v, want nil for an unlimited code", list[1].Remaining)
	}
	if list[1].Description != "新人立减" {
		t.Errorf("OPEN description = %q, want the shop's own words", list[1].Description)
	}
	if list[1].Scope != "category" || list[1].CategoryID == nil || *list[1].CategoryID != 3 {
		t.Errorf("OPEN scope = %q/%v, want the shelf to say which goods it covers", list[1].Scope, list[1].CategoryID)
	}
}

func text(value string) *string { return &value }

func number(value uint) *uint { return &value }

func TestStorefrontCouponsIsSilentWhenCodesAreOff(t *testing.T) {
	repo := &shelfRepo{advertised: []domain.Coupon{{Code: "WHATEVER", DiscountType: "fixed", DiscountValue: 100}}}
	service := NewService(nil, nil, nil, repo, config.FeaturesConfig{CouponsDisabled: true})
	list, err := service.StorefrontCoupons(context.Background())
	if err != nil {
		t.Fatalf("StorefrontCoupons: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("shelf = %+v, want an empty list while the shop has coupons off", list)
	}
}

func TestStorefrontCouponsListsDeadlinesFirst(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	repo := &shelfRepo{
		advertised: []domain.Coupon{
			{Base: models.Base{ID: 1}, Code: "STANDING", DiscountType: "fixed", DiscountValue: 100},
			{Base: models.Base{ID: 2}, Code: "ENDINGLATER", DiscountType: "fixed", DiscountValue: 100, ValidUntil: at(now.Add(48 * time.Hour))},
			{Base: models.Base{ID: 3}, Code: "ENDINGFIRST", DiscountType: "fixed", DiscountValue: 100, ValidUntil: at(now.Add(time.Hour))},
		},
	}
	service := NewService(nil, nil, nil, repo, config.FeaturesConfig{})
	service.now = func() time.Time { return now }
	list, err := service.StorefrontCoupons(context.Background())
	if err != nil {
		t.Fatalf("StorefrontCoupons: %v", err)
	}
	got := []string{list[0].Code, list[1].Code, list[2].Code}
	if got[0] != "ENDINGFIRST" || got[1] != "ENDINGLATER" || got[2] != "STANDING" {
		t.Errorf("shelf order = %v, want the codes about to end first", got)
	}
}

// TestGuestQuoteSeesOnlyAdvertisedCodes is the whole reason the preview route can
// be public: a guest may price a code the shop already advertised, and a code it
// did not is answered exactly like a typo, so the route leaks no code list.
func TestGuestQuoteSeesOnlyAdvertisedCodes(t *testing.T) {
	shelf := &domain.Coupon{
		Base: models.Base{ID: 1}, Code: "SHELF", DiscountType: "percent", DiscountValue: 10,
		IsActive: true, Advertised: true, Scope: "all", PerUserLimit: 1,
	}
	private := &domain.Coupon{
		Base: models.Base{ID: 2}, Code: "PRIVATE", DiscountType: "fixed", DiscountValue: 5,
		IsActive: true, Scope: "all", PerUserLimit: 1,
	}
	repo := &shelfRepo{byCode: map[string]*domain.Coupon{"SHELF": shelf, "PRIVATE": private}}
	service := NewService(&restockProducts{product: cardProduct()}, nil, nil, repo, config.FeaturesConfig{})

	guest, err := service.QuoteCoupon(context.Background(), 0, 7, 2, 30, "shelf")
	if err != nil {
		t.Fatalf("a guest previewing an advertised code: %v", err)
	}
	if guest.Discount != 6 || guest.Payable != 54 {
		t.Errorf("guest quote = %d off / %d payable, want 6 off 60", guest.Discount, guest.Payable)
	}
	if guest.Note == "" {
		t.Errorf("guest quote note is empty, want the 每人限用 the preview could not check")
	}

	if _, err := service.QuoteCoupon(context.Background(), 0, 7, 1, 30, "private"); !errors.Is(err, ErrCouponNotFound) {
		t.Errorf("a guest on a private code = %v, want the same answer as a typo", err)
	}

	buyer, err := service.QuoteCoupon(context.Background(), 5, 7, 1, 30, "PRIVATE")
	if err != nil {
		t.Fatalf("the account the code was sent to: %v", err)
	}
	if !buyer.Accepted || buyer.Note != "" {
		t.Errorf("signed-in quote = accepted %v note %q, want it priced with no guest small print", buyer.Accepted, buyer.Note)
	}
}
