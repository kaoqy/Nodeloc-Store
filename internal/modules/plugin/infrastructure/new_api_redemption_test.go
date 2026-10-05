package infrastructure

import (
	"context"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/contract"
)

// TestNewAPIChannelExposesOnlyTheAmountField pins the field contract: the
// New-API channel asks the buyer for exactly one thing, the top-up amount, and
// never for an account, username or any other channel-specific input.
func TestNewAPIChannelExposesOnlyTheAmountField(t *testing.T) {
	manifest := NewNewAPIRedemption(nil).Manifest()
	if len(manifest.FormSchema) != 1 {
		t.Fatalf("New-API exposes %d form fields, want exactly 1", len(manifest.FormSchema))
	}
	field := manifest.FormSchema[0]
	if field.Key != "nl_amount" || field.Type != "number" || !field.Required {
		t.Fatalf("New-API form field = %+v, want a required number nl_amount", field)
	}
	for _, banned := range []string{"account", "username", "user_id", "nl_account"} {
		for _, candidate := range manifest.FormSchema {
			if strings.Contains(candidate.Key, banned) {
				t.Fatalf("New-API must not ask for %q (field %q)", banned, candidate.Key)
			}
		}
	}
}

// TestNewAPIRequestCarriesNameQuotaAndCount pins the upstream contract: the
// request body is exactly name/quota/count, with the name and quota derived from
// the server-confirmed paid NL, and count fixed at 1.
func TestNewAPIRequestCarriesNameQuotaAndCount(t *testing.T) {
	var gotBody string
	var gotAuth string
	var gotUser string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/redemption/" {
			t.Errorf("path = %q", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotUser = r.Header.Get("New-Api-User")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":["32875e383dda48bdb6b272313094dfd6"],"message":"","success":true}`))
	}))
	defer server.Close()

	transport := &rewriteTransport{target: server.URL}
	provider := NewNewAPIRedemption(nil).withClient(&http.Client{Transport: transport})
	// 18 NL paid, 1 NL = 1 USD, 500000 quota/USD ⇒ quota 9000000, name "18NL".
	result, err := provider.Deliver(context.Background(), contract.DeliveryRequest{
		OrderNo:      "NL1",
		Quantity:     1,
		PaidNLAmount: 18,
		FormValues: map[string]string{
			"__plugin_config":  `{"base_url":"https://new-api.example.com","admin_user_id":"7","nl_usd_rate":"1"}`,
			"__plugin_secrets": `{"admin_access_token":"secret-token"}`,
			"nl_amount":        "999",
		},
	})
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if result.Uncertain {
		t.Fatalf("confirmed response marked uncertain: %+v", result)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotUser != "7" {
		t.Fatalf("New-Api-User = %q", gotUser)
	}
	if gotBody != `{"name":"18NL","quota":9000000,"count":1}` {
		t.Fatalf("request body = %s", gotBody)
	}
	if strings.Contains(gotBody, "nl_amount") || strings.Contains(gotBody, "999") ||
		strings.Contains(gotBody, "order") || strings.Contains(gotBody, "user") {
		t.Fatalf("request body leaked extra fields: %s", gotBody)
	}
	if !strings.Contains(result.Content, "32875e383dda48bdb6b272313094dfd6") {
		t.Fatalf("delivery content = %q", result.Content)
	}
	if !strings.Contains(result.Content, "18 NL") {
		t.Fatalf("delivery content did not name the paid amount: %q", result.Content)
	}
}

// quota must come from paid NL and the configured rate, with exact decimal math.
func TestNewAPIQuotaUsesRateWithoutFloatDrift(t *testing.T) {
	cases := []struct {
		name   string
		paidNL int64
		rate   string
		want   int64
	}{
		{"one to one", 18, "1", 9000000},
		{"half dollar per NL", 18, "0.5", 4500000},
		{"two dollars per NL", 3, "2", 3000000},
		{"tenth", 10, "0.1", 500000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rate, err := positiveRate(tc.rate, "rate")
			if err != nil {
				t.Fatalf("positiveRate(%q): %v", tc.rate, err)
			}
			got, err := quotaForPaidNL(tc.paidNL, rate)
			if err != nil {
				t.Fatalf("quotaForPaidNL: %v", err)
			}
			if got != tc.want {
				t.Fatalf("quota = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestNewAPIQuotaRejectsBadInput(t *testing.T) {
	one, _ := positiveRate("1", "rate")
	if _, err := quotaForPaidNL(0, one); err == nil {
		t.Fatal("zero paid NL was accepted")
	}
	if _, err := positiveRate("0", "rate"); err == nil {
		t.Fatal("zero rate was accepted")
	}
	if _, err := positiveRate("-1", "rate"); err == nil {
		t.Fatal("negative rate was accepted")
	}
	if _, err := positiveRate("abc", "rate"); err == nil {
		t.Fatal("non-numeric rate was accepted")
	}
	// 1 NL = 0.000001 USD ⇒ 1 × 0.000001 × 500000 = 0.5 quota, not an integer.
	oddRate, err := positiveRate("0.000001", "rate")
	if err != nil {
		t.Fatalf("positiveRate: %v", err)
	}
	if _, err := quotaForPaidNL(1, oddRate); err == nil {
		t.Fatal("non-integral quota was accepted")
	}
	// Enormous rate overflows the allowed range.
	huge := new(big.Rat).SetInt64(1 << 40)
	if _, err := quotaForPaidNL(1<<40, huge); err == nil {
		t.Fatal("overflowing quota was accepted")
	}
}

func TestNewAPIDataArrayIsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":["abc123xyz7890"],"message":"","success":true}`))
	}))
	defer server.Close()

	transport := &rewriteTransport{target: server.URL}
	provider := NewNewAPIRedemption(nil).withClient(&http.Client{Transport: transport})
	result, err := provider.Deliver(context.Background(), contract.DeliveryRequest{
		OrderNo:      "NL3",
		Quantity:     1,
		PaidNLAmount: 10,
		FormValues: map[string]string{
			"__plugin_config":  `{"base_url":"https://new-api.example.com","admin_user_id":"7","nl_usd_rate":"1"}`,
			"__plugin_secrets": `{"admin_access_token":"secret-token"}`,
		},
	})
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if result.Uncertain {
		t.Fatalf("data array response marked uncertain: %+v", result)
	}
	if !strings.Contains(result.Content, "abc123xyz7890") {
		t.Fatalf("delivery content = %q", result.Content)
	}
}

// A parseable rejection (success=false) is a hard failure. A well-formed HTTP
// 200 that claims success but carries no usable code is parked as uncertain:
// the request may have been processed, so it must never be reported as
// delivered and must never be silently retried.
func TestNewAPIFailureResponsesNeverClaimSuccess(t *testing.T) {
	t.Run("success false is a hard failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":false,"message":"quota invalid","data":[]}`))
		}))
		defer server.Close()
		transport := &rewriteTransport{target: server.URL}
		provider := NewNewAPIRedemption(nil).withClient(&http.Client{Transport: transport})
		result, err := provider.Deliver(context.Background(), contract.DeliveryRequest{
			OrderNo:      "NL4",
			Quantity:     1,
			PaidNLAmount: 10,
			FormValues: map[string]string{
				"__plugin_config":  `{"base_url":"https://new-api.example.com","admin_user_id":"7","nl_usd_rate":"1"}`,
				"__plugin_secrets": `{"admin_access_token":"secret-token"}`,
			},
		})
		if err == nil {
			t.Fatal("success=false was accepted")
		}
		if result.Uncertain {
			t.Fatal("success=false must be a hard failure, not uncertain")
		}
		if !strings.Contains(err.Error(), "quota invalid") {
			t.Fatalf("provider message lost: %v", err)
		}
	})

	for _, tc := range []struct {
		name string
		body string
	}{
		{"missing success", `{"data":["abc123xyz7890"],"message":""}`},
		{"data null", `{"success":true,"message":"","data":null}`},
		{"data empty", `{"success":true,"message":"","data":[]}`},
		{"data not strings", `{"success":true,"message":"","data":[123]}`},
		{"data garbage code", `{"success":true,"message":"","data":["<html>oops</html>"]}`},
	} {
		t.Run(tc.name+" is parked as uncertain, not delivered", func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			transport := &rewriteTransport{target: server.URL}
			provider := NewNewAPIRedemption(nil).withClient(&http.Client{Transport: transport})
			result, err := provider.Deliver(context.Background(), contract.DeliveryRequest{
				OrderNo:      "NL4",
				Quantity:     1,
				PaidNLAmount: 10,
				FormValues: map[string]string{
					"__plugin_config":  `{"base_url":"https://new-api.example.com","admin_user_id":"7","nl_usd_rate":"1"}`,
					"__plugin_secrets": `{"admin_access_token":"secret-token"}`,
				},
			})
			if err != nil {
				t.Fatalf("ambiguous body caused a hard error instead of review: %v", err)
			}
			if !result.Uncertain {
				t.Fatalf("body %s was accepted as delivered: %+v", tc.body, result)
			}
			if strings.Contains(result.Content, "兑换码：") {
				t.Fatalf("ambiguous body produced delivery content: %q", result.Content)
			}
		})
	}
}

func TestNewAPIHTTPErrorIsHardFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"success":false,"message":"bad request"}`))
	}))
	defer server.Close()

	transport := &rewriteTransport{target: server.URL}
	provider := NewNewAPIRedemption(nil).withClient(&http.Client{Transport: transport})
	result, err := provider.Deliver(context.Background(), contract.DeliveryRequest{
		OrderNo:      "NL6",
		Quantity:     1,
		PaidNLAmount: 10,
		FormValues: map[string]string{
			"__plugin_config":  `{"base_url":"https://new-api.example.com","admin_user_id":"7","nl_usd_rate":"1"}`,
			"__plugin_secrets": `{"admin_access_token":"secret-token"}`,
		},
	})
	if err == nil {
		t.Fatal("HTTP 400 was accepted as created")
	}
	if result.Uncertain {
		t.Fatalf("definite HTTP failure marked uncertain: %+v", result)
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("status code lost: %v", err)
	}
}

// A config that cannot be priced must fail before any outbound request.
func TestNewAPIMissingRateDoesNotCallUpstream(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"success":true,"data":["abc123xyz7890"]}`))
	}))
	defer server.Close()

	transport := &rewriteTransport{target: server.URL}
	provider := NewNewAPIRedemption(nil).withClient(&http.Client{Transport: transport})
	_, err := provider.Deliver(context.Background(), contract.DeliveryRequest{
		OrderNo:      "NL8",
		Quantity:     1,
		PaidNLAmount: 10,
		FormValues: map[string]string{
			"__plugin_config":  `{"base_url":"https://new-api.example.com","admin_user_id":"7"}`,
			"__plugin_secrets": `{"admin_access_token":"secret-token"}`,
		},
	})
	if err == nil {
		t.Fatal("a missing rate was accepted")
	}
	if called {
		t.Fatal("upstream was called despite an invalid rate")
	}
}

type rewriteTransport struct {
	target string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	target, err := url.Parse(t.target)
	if err != nil {
		return nil, err
	}
	req = req.Clone(req.Context())
	req.URL.Scheme = target.Scheme
	req.URL.Host = target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func TestNewAPIRejectsUnsafeBaseURL(t *testing.T) {
	for _, raw := range []string{
		"http://localhost:3000",
		"http://127.0.0.1:3000",
		"http://10.0.0.1",
		"http://169.254.169.254",
		"file:///etc/passwd",
	} {
		if _, err := normalizeBaseURL(raw); err == nil {
			t.Errorf("unsafe base URL %q was accepted", raw)
		}
	}
}
