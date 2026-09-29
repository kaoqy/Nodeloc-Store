package http

import (
	"testing"
	"time"
)

// A buyer types their own contact details, and those land in the CSV a shop
// owner opens in Excel. A cell starting with = + - @ would execute there, so the
// download has to defuse it while keeping the text readable.
func TestSafeCellDefusesFormulaStarts(t *testing.T) {
	for _, dangerous := range []string{"=1+1", "+1", "-1", "@SUM(A1)", "\t=cmd", "\r=x"} {
		got := safeCell(dangerous)
		if got[:1] != "'" {
			t.Errorf("safeCell(%q) = %q, want it prefixed with a quote", dangerous, got)
		}
		if got[1:] != dangerous {
			t.Errorf("safeCell(%q) = %q, want the original text kept", dangerous, got)
		}
	}
	for _, plain := range []string{"", "buyer@example.com", "SEED-2026", "支付宝", " 已付款"} {
		if got := safeCell(plain); got != plain {
			t.Errorf("safeCell(%q) = %q, want it unchanged", plain, got)
		}
	}
}

// The export must write the amount the shop set and the buyer was charged, so
// the number is rendered with two decimals and never rescaled.
func TestYuanRendersStatementAmount(t *testing.T) {
	cases := map[int]string{0: "0.00", 1: "1.00", 99: "99.00", 999: "999.00", 123456789: "123456789.00"}
	for amount, want := range cases {
		if got := yuan(amount); got != want {
			t.Errorf("yuan(%d) = %q, want %q", amount, got, want)
		}
	}
}

// An unpaid order has no paid time to write, and an empty cell is what a
// spreadsheet expects — not 0001-01-01.
func TestFormatTimeLeavesUnsetFieldsBlank(t *testing.T) {
	if got := formatTime(nil); got != "" {
		t.Errorf("formatTime(nil) = %q, want blank", got)
	}
	moment := time.Date(2026, 9, 29, 10, 30, 0, 0, time.UTC)
	if got := formatTime(&moment); got != "2026-09-29T10:30:00Z" {
		t.Errorf("formatTime = %q, want RFC3339", got)
	}
}
