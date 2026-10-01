package application

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/domain"
)

// Each provider answer the store found on the real NodeLoc names a different thing
// to fix, so each needs its own code and its own Chinese sentence. A refusal that
// is missing from Classify is one the storefront will describe as 「稍后再试」 while
// the shop owner needs to open 设置.
func TestClassifyNamesTheProviderAnswersApart(t *testing.T) {
	cases := []struct {
		err        error
		code       string
		status     int
		wantsWord  string
		wantsRetry bool
	}{
		{domain.ErrPaymentAppNotFound, "payment_id_unknown", http.StatusServiceUnavailable, "Payment ID", false},
		{domain.ErrProviderGuarded, "provider_guarded", http.StatusBadGateway, "下单记录", true},
		{domain.ErrProviderClockSkew, "provider_clock", http.StatusServiceUnavailable, "时间", false},
		{domain.ErrProviderRejected, "provider_rejected", http.StatusBadGateway, "凭据", false},
		{domain.ErrProviderUnreachable, "provider_unreachable", http.StatusBadGateway, "联系不上", true},
		{domain.ErrPaymentNotConfigured, "not_configured", http.StatusServiceUnavailable, "配置", false},
		{domain.ErrProviderIPNotAllowed, "provider_ip_blocked", http.StatusForbidden, "白名单", false},
	}
	for _, item := range cases {
		failure := Classify(fmt.Errorf("wrapped: %w", item.err))
		if failure == nil {
			t.Fatalf("%v classified as nothing", item.err)
		}
		if failure.Code != item.code || failure.Status != item.status {
			t.Fatalf("%v => code %q status %d, want %q %d", item.err, failure.Code, failure.Status, item.code, item.status)
		}
		if !strings.Contains(failure.Message, item.wantsWord) {
			t.Fatalf("%s copy says %q, which does not name %q", item.code, failure.Message, item.wantsWord)
		}
		if failure.Retryable != item.wantsRetry {
			t.Fatalf("%s retryable = %v, want %v", item.code, failure.Retryable, item.wantsRetry)
		}
		// The owner reads the Message; the provider's or Go's own sentence stays in
		// Detail, where a back office can show it as evidence. What used to leak here
		// was the wrapped error — sentinel text included — appearing inside the shop's
		// own copy, so the marker is the wrapping this test adds rather than any
		// particular word: the sentinels speak Chinese now and share vocabulary with the
		// advice legitimately.
		if strings.Contains(failure.Message, "wrapped:") {
			t.Fatalf("%s leaked the wrapped provider error into the shop's own copy: %q", item.code, failure.Message)
		}
	}
}

// A refused 转账 shows the owner the sentence the ledger keeps, including for the
// answers that describe the shop rather than this transfer.
func TestGrantRefusalsReuseTheShopWording(t *testing.T) {
	for _, sentinel := range []error{
		domain.ErrPaymentAppNotFound,
		domain.ErrProviderGuarded,
		domain.ErrProviderClockSkew,
		domain.ErrPaymentNotConfigured,
		domain.ErrProviderUnreachable,
	} {
		err := fmt.Errorf("payment: %w", sentinel)
		failure := classifyGrantError(err)
		shared := Classify(err)
		if failure == nil {
			t.Fatalf("%v produced no grant failure", sentinel)
		}
		if failure.Code != shared.Code {
			t.Fatalf("%v => grant code %q, Classify says %q", sentinel, failure.Code, shared.Code)
		}
		if !strings.Contains(failure.Message, shared.Message) {
			t.Fatalf("%v => grant copy %q, want the shop's own %q", sentinel, failure.Message, shared.Message)
		}
	}

	// The gateway names which fields are still empty; the Chinese sentence alone
	// would not tell the owner which box to fill.
	failure := classifyGrantError(fmt.Errorf("%w：缺少 token、secret_key", domain.ErrPaymentNotConfigured))
	if failure == nil || !strings.Contains(failure.Message, "缺少 token、secret_key") {
		t.Fatalf("an incomplete setup lost its field names: %+v", failure)
	}
}
