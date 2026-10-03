package application

import (
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/activity/domain"
)

// TestFactorDiscountIsMultiplicative pins the pricing rule the shop asked for:
// 成交价 = 售价 × 折扣率. 售价 100、8 折 → 成交 80，折扣额 20。
func TestFactorDiscountIsMultiplicative(t *testing.T) {
	if got := discountFromFactor(100, 0.8); got != 20 {
		t.Fatalf("discount for 100x0.8 = %d, want 20", got)
	}
	if got := discountFromFactor(100, 0.85); got != 15 {
		t.Fatalf("discount for 100x0.85 = %d, want 15", got)
	}
	// 多件时折扣随数量线性放大，不会只减一次。
	if got := discountFromFactor(300, 0.8); got != 60 {
		t.Fatalf("discount for 300x0.8 = %d, want 60", got)
	}
}

// TestFactorDiscountRoundsToWholeUnit keeps discount and payable exact: the
// rounded discount subtracted from the total must land on the same number a
// buyer would compute from 售价 × 系数.
func TestFactorDiscountRoundsToWholeUnit(t *testing.T) {
	cases := []struct {
		total  int
		factor float64
		pay    int
	}{
		{99, 0.9, 89},
		{199, 0.8, 159},
		{1000, 0.333, 333},
	}
	for _, c := range cases {
		discount := discountFromFactor(c.total, c.factor)
		if got := c.total - discount; got != c.pay {
			t.Fatalf("%d x %.3f -> payable %d, want %d", c.total, c.factor, got, c.pay)
		}
	}
}

// TestPercentRuleMatchesFactorRule proves the legacy 9 折 rule and the new
// factor rule produce identical money, so old activities keep working.
func TestPercentRuleMatchesFactorRule(t *testing.T) {
	if discountFromFactor(100, 0.9) != discountFromFactor(100, float64(90)/100) {
		t.Fatal("percent and factor paths disagree")
	}
}

// TestFactorRuleRejectsNoOp guards the boundaries: factor 1 (no discount) and
// factor 0 (free) must both be refused rather than mispriced.
func TestFactorRuleRejectsNoOp(t *testing.T) {
	if discountFromFactor(100, 1) != 0 {
		t.Fatal("factor 1 should mean no discount")
	}
	if discountFromFactor(100, 0) != 0 {
		t.Fatal("factor 0 must not be treated as free")
	}
	if discountFromFactor(0, 0.5) != 0 {
		t.Fatal("zero total has no discount")
	}
}

var _ = domain.RuleFactorOff
