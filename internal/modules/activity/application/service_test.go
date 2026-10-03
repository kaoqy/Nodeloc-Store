package application

import (
	"context"
	"testing"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/activity/domain"
)

// fakeRepo implements the activity repository surface the pricing tests need.
type fakeRepo struct {
	activities []domain.Activity
	rules      map[uint][]domain.ActivityRule
	counts     map[uint]int64
	category   *uint
}

func (f *fakeRepo) List(context.Context, domain.ListFilter) ([]domain.ActivityView, int64, error) {
	return nil, 0, nil
}
func (f *fakeRepo) GetByID(context.Context, uint) (*domain.Activity, error) { return nil, nil }
func (f *fakeRepo) GetByIDWithRules(context.Context, uint) (*domain.Activity, error) {
	return nil, nil
}
func (f *fakeRepo) Create(context.Context, *domain.Activity) error { return nil }
func (f *fakeRepo) Update(context.Context, *domain.Activity) error { return nil }
func (f *fakeRepo) Delete(context.Context, uint) error             { return nil }
func (f *fakeRepo) ListRules(_ context.Context, activityID uint) ([]domain.ActivityRule, error) {
	return f.rules[activityID], nil
}
func (f *fakeRepo) ReplaceRules(context.Context, uint, []domain.ActivityRule) error { return nil }
func (f *fakeRepo) ListActive(context.Context, time.Time) ([]domain.Activity, error) {
	return f.activities, nil
}
func (f *fakeRepo) Reserve(context.Context, uint, *domain.ActivityRecord) error { return nil }
func (f *fakeRepo) Release(context.Context, uint, string) error                 { return nil }
func (f *fakeRepo) MarkRecordStatus(context.Context, uint, string) error        { return nil }
func (f *fakeRepo) CountUserRecords(_ context.Context, activityID, _ uint) (int64, error) {
	return f.counts[activityID], nil
}
func (f *fakeRepo) ListRecords(context.Context, uint, int, int) ([]domain.ActivityRecord, int64, error) {
	return nil, 0, nil
}
func (f *fakeRepo) ListRecordsByUser(context.Context, uint, int, int) ([]domain.ActivityRecord, int64, error) {
	return nil, 0, nil
}
func (f *fakeRepo) Stats(context.Context, uint) (*domain.ActivityStats, error) {
	return &domain.ActivityStats{}, nil
}
func (f *fakeRepo) ListLogs(context.Context, uint, int, int) ([]domain.ActivityLog, int64, error) {
	return nil, 0, nil
}
func (f *fakeRepo) AppendLog(context.Context, *domain.ActivityLog) error    { return nil }
func (f *fakeRepo) HasPurchased(context.Context, uint) (bool, error)        { return false, nil }
func (f *fakeRepo) ProductCategory(context.Context, uint) (*uint, error)    { return f.category, nil }
func (f *fakeRepo) CouponClaim(context.Context, *domain.CouponRecord) error { return nil }
func (f *fakeRepo) ListCouponRecords(context.Context, uint, int, int) ([]domain.CouponRecord, int64, error) {
	return nil, 0, nil
}
func (f *fakeRepo) CountCouponRecord(context.Context, uint, uint) (int64, error) { return 0, nil }

func newPricingService(t *testing.T, repo *fakeRepo) *Service {
	t.Helper()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return service
}

func activeActivity(id uint, rules []domain.ActivityRule, mutate func(*domain.Activity)) *fakeRepo {
	activity := domain.Activity{
		Base: models.Base{ID: id}, Name: "测试活动", Type: models.ActivityTypeStoreDiscount,
		Status: models.ActivityStatusRunning, AutoApply: true,
	}
	if mutate != nil {
		mutate(&activity)
	}
	return &fakeRepo{activities: []domain.Activity{activity}, rules: map[uint][]domain.ActivityRule{id: rules}}
}

// TestPricePercentOff proves the discount is computed on the server from the
// stored rule, never from a client-supplied amount.
func TestPricePercentOff(t *testing.T) {
	repo := activeActivity(1, []domain.ActivityRule{{RuleType: domain.RulePercentOff, Config: "{\"percent\":90}", IsEnabled: true}}, nil)
	service := newPricingService(t, repo)
	pricing, err := service.Price(context.Background(), domain.MatchInput{UserID: 3, ProductID: 10, Quantity: 2, UnitPrice: 100})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	if pricing.DiscountAmount != 20 || pricing.Payable != 180 {
		t.Fatalf("unexpected pricing: %+v", pricing)
	}
	if pricing.Snapshot == "" {
		t.Fatal("pricing did not keep a rule snapshot")
	}
}

// TestPriceFullReductionTiers uses the highest reaching tier.
func TestPriceFullReductionTiers(t *testing.T) {
	config := "{\"tiers\":[{\"threshold\":100,\"amount\":10},{\"threshold\":200,\"amount\":30}]}"
	repo := activeActivity(2, []domain.ActivityRule{{RuleType: domain.RuleFullReduce, Config: config, IsEnabled: true}}, nil)
	service := newPricingService(t, repo)
	pricing, err := service.Price(context.Background(), domain.MatchInput{UserID: 3, ProductID: 10, Quantity: 2, UnitPrice: 120})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	if pricing.DiscountAmount != 30 {
		t.Fatalf("discount = %d, want 30", pricing.DiscountAmount)
	}
}

// TestPriceRespectsScopeAndPerUserLimit checks the applicability gates: a
// product outside the activity's scope is refused, and a used-up per-user limit
// blocks a second participation.
func TestPriceRespectsScopeAndPerUserLimit(t *testing.T) {
	category := uint(9)
	repo := activeActivity(3, []domain.ActivityRule{{RuleType: domain.RuleAmountOff, Config: "{\"amount\":5}", IsEnabled: true}}, func(a *domain.Activity) {
		a.ProductIDs = "[42]"
		a.PerUserLimit = 1
	})
	service := newPricingService(t, repo)
	if _, err := service.Price(context.Background(), domain.MatchInput{UserID: 3, ProductID: 7, Quantity: 1, UnitPrice: 100}); err == nil {
		t.Fatal("out-of-scope product was priced")
	}
	inScope := activeActivity(4, []domain.ActivityRule{{RuleType: domain.RuleAmountOff, Config: "{\"amount\":5}", IsEnabled: true}}, func(a *domain.Activity) {
		a.ProductIDs = "[7]"
		a.PerUserLimit = 1
	})
	inScope.category = &category
	inScope.counts = map[uint]int64{4: 1}
	service = newPricingService(t, inScope)
	if _, err := service.Price(context.Background(), domain.MatchInput{UserID: 3, ProductID: 7, Quantity: 1, UnitPrice: 100}); err == nil {
		t.Fatal("per-user limit was not enforced")
	}
}

// TestPriceNeverReachesZero keeps the NodeLoc floor: an order must remain
// payable, so a 100% style discount still leaves at least one unit.
func TestPriceNeverReachesZero(t *testing.T) {
	repo := activeActivity(5, []domain.ActivityRule{{RuleType: domain.RuleAmountOff, Config: "{\"amount\":999}", IsEnabled: true}}, nil)
	service := newPricingService(t, repo)
	pricing, err := service.Price(context.Background(), domain.MatchInput{UserID: 3, ProductID: 10, Quantity: 1, UnitPrice: 100})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	if pricing.Payable != 1 || pricing.DiscountAmount != 99 {
		t.Fatalf("floor not applied: %+v", pricing)
	}
}

// TestValidateRejectsBadRules is the form-level guard: an unknown rule type or a
// 100% discount never reaches the database.
func TestValidateRejectsBadRules(t *testing.T) {
	service := newPricingService(t, &fakeRepo{})
	activity := domain.Activity{Name: "x", Type: models.ActivityTypeStoreDiscount, Status: models.ActivityStatusDraft, AutoApply: true}
	if err := service.validate(&activity, []domain.ActivityRule{{RuleType: "drop_table", Config: "{}"}}, true); err == nil {
		t.Fatal("unknown rule type was accepted")
	}
	if err := service.validate(&activity, []domain.ActivityRule{{RuleType: domain.RulePercentOff, Config: "{\"percent\":100}"}}, true); err == nil {
		t.Fatal("100%% rule was accepted")
	}
	if err := service.validate(&activity, nil, true); err == nil {
		t.Fatal("auto-applied activity without rules was accepted")
	}
	// A draft that is not auto-applied may be saved before its rules are written.
	draft := domain.Activity{Name: "草稿", Type: models.ActivityTypeStoreDiscount, Status: models.ActivityStatusDraft}
	if err := service.validate(&draft, nil, true); err != nil {
		t.Fatalf("plain draft was refused: %v", err)
	}
}

// TestPriceFullQuantityIsFlat pins the meaning of the 满件优惠 rule: with only
// an Amount configured it is a one-off reduction once the threshold is reached
// (满 3 件减 5 元), never Amount x quantity. Multiplying turned a 5 元 saving
// into a per-item rebate and is the bug the shop reported for 满件 activities.
func TestPriceFullQuantityIsFlat(t *testing.T) {
	repo := activeActivity(10, []domain.ActivityRule{{RuleType: domain.RuleFullQuantity, Config: "{\"quantity\":3,\"amount\":5}", IsEnabled: true}}, nil)
	service := newPricingService(t, repo)
	pricing, err := service.Price(context.Background(), domain.MatchInput{UserID: 3, ProductID: 10, Quantity: 3, UnitPrice: 100})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	if pricing.DiscountAmount != 5 || pricing.Payable != 295 {
		t.Fatalf("full-quantity discount = %d payable = %d, want 5 / 295", pricing.DiscountAmount, pricing.Payable)
	}
	if len(pricing.AppliedRules) != 1 || pricing.AppliedRules[0] != domain.RuleFullQuantity {
		t.Fatalf("applied rules = %v, want [full_quantity]", pricing.AppliedRules)
	}
	// Below the threshold nothing applies.
	if _, err := service.Price(context.Background(), domain.MatchInput{UserID: 3, ProductID: 10, Quantity: 2, UnitPrice: 100}); err == nil {
		t.Fatal("full-quantity rule fired below its threshold")
	}
}

// TestPriceFullQuantityPerUnitKeepsPerItemSemantics covers the explicit
// per-item form: 每件减 5 元 still scales with quantity.
func TestPriceFullQuantityPerUnitKeepsPerItemSemantics(t *testing.T) {
	repo := activeActivity(11, []domain.ActivityRule{{RuleType: domain.RuleFullQuantity, Config: "{\"quantity\":3,\"per_unit\":5}", IsEnabled: true}}, nil)
	service := newPricingService(t, repo)
	pricing, err := service.Price(context.Background(), domain.MatchInput{UserID: 3, ProductID: 10, Quantity: 4, UnitPrice: 100})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	if pricing.DiscountAmount != 20 {
		t.Fatalf("per-unit discount = %d, want 20", pricing.DiscountAmount)
	}
}

// TestPriceAppliedRulesOnlyListsWinner guards the non-stacking snapshot: a rule
// that loses to a bigger one must not appear in AppliedRules, otherwise the
// stored order snapshot claims a discount that was never granted.
func TestPriceAppliedRulesOnlyListsWinner(t *testing.T) {
	rules := []domain.ActivityRule{
		{RuleType: domain.RulePercentOff, Config: "{\"percent\":90}", IsEnabled: true},
		{RuleType: domain.RuleAmountOff, Config: "{\"amount\":5}", IsEnabled: true},
	}
	repo := activeActivity(12, rules, nil)
	service := newPricingService(t, repo)
	pricing, err := service.Price(context.Background(), domain.MatchInput{UserID: 3, ProductID: 10, Quantity: 1, UnitPrice: 100})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	if pricing.DiscountAmount != 10 {
		t.Fatalf("discount = %d, want 10", pricing.DiscountAmount)
	}
	if len(pricing.AppliedRules) != 1 || pricing.AppliedRules[0] != domain.RulePercentOff {
		t.Fatalf("applied rules = %v, want [percent_off]", pricing.AppliedRules)
	}
}

// TestValidateFullQuantityNeedsAmount rejects a 满件 rule with no reduction,
// which would otherwise look configured but price at zero.
func TestValidateFullQuantityNeedsAmount(t *testing.T) {
	service := newPricingService(t, &fakeRepo{})
	activity := domain.Activity{Name: "x", Type: models.ActivityTypeStoreDiscount, Status: models.ActivityStatusDraft, AutoApply: true}
	if err := service.validate(&activity, []domain.ActivityRule{{RuleType: domain.RuleFullQuantity, Config: "{\"quantity\":3}"}}, true); err == nil {
		t.Fatal("full-quantity rule without amount or per_unit was accepted")
	}
}
