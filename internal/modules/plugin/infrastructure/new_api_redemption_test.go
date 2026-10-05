package infrastructure

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/contract"
)

func TestNewAPIKeyIsThirteenLowercaseAlphanumeric(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 128; i++ {
		key, err := newAPIKey()
		if err != nil {
			t.Fatalf("newAPIKey: %v", err)
		}
		if len(key) != newAPIKeyLength {
			t.Fatalf("key %q has length %d", key, len(key))
		}
		for _, r := range key {
			if !strings.ContainsRune(newAPIKeyAlphabet, r) {
				t.Fatalf("key %q contains invalid rune %q", key, r)
			}
		}
		if seen[key] {
			t.Fatalf("duplicate key %q", key)
		}
		seen[key] = true
	}
}

func TestNewAPIRequestCarriesOnlyKeyAndQuota(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"success":true,"message":"","data":{"id":1,"name":"","key":"abc123xyz7890","status":1,"quota":100000}}`))
	}))
	defer server.Close()

	transport := &rewriteTransport{target: server.URL}
	provider := NewNewAPIRedemption(nil).withClient(&http.Client{Transport: transport})
	result, err := provider.Deliver(context.Background(), contract.DeliveryRequest{
		OrderNo:    "NL1",
		TotalPrice: 100,
		Quantity:   1,
		FormValues: map[string]string{
			"__plugin_config":  `{"base_url":"https://new-api.example.com","admin_user_id":"7","quota_per_nl":"1000","success_field":"data.key"}`,
			"__plugin_secrets": `{"admin_access_token":"secret-token"}`,
			"nl_amount":        "100",
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
	if strings.Contains(gotBody, "order") || strings.Contains(gotBody, "user") || strings.Contains(gotBody, "NL") {
		t.Fatalf("request body leaked extra fields: %s", gotBody)
	}
	if !strings.HasPrefix(gotBody, `{"key":"`) || !strings.Contains(gotBody, `"quota":100000`) {
		t.Fatalf("request body = %s", gotBody)
	}
	if !strings.Contains(result.Content, "abc123xyz7890") {
		t.Fatalf("delivery content = %q", result.Content)
	}
}

func TestNewAPIUncertainResponseDoesNotClaimSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"unknown shape"}`))
	}))
	defer server.Close()

	transport := &rewriteTransport{target: server.URL}
	provider := NewNewAPIRedemption(nil).withClient(&http.Client{Transport: transport})
	result, err := provider.Deliver(context.Background(), contract.DeliveryRequest{
		OrderNo:    "NL2",
		TotalPrice: 10,
		Quantity:   1,
		FormValues: map[string]string{
			"__plugin_config":  `{"base_url":"https://new-api.example.com","admin_user_id":"7","quota_per_nl":"100"}`,
			"__plugin_secrets": `{"admin_access_token":"secret-token"}`,
			"nl_amount":        "10",
		},
	})
	if err != nil {
		t.Fatalf("Deliver returned hard error for uncertain result: %v", err)
	}
	if !result.Uncertain {
		t.Fatalf("unknown response was treated as success: %+v", result)
	}
	if strings.Contains(result.Content, "已到账") || strings.Contains(result.Note, "已交付") {
		t.Fatalf("uncertain result claimed success: %+v", result)
	}
}

func TestNewAPIDataStringEnvelopeIsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"message":"","data":"abc123xyz7890"}`))
	}))
	defer server.Close()

	transport := &rewriteTransport{target: server.URL}
	provider := NewNewAPIRedemption(nil).withClient(&http.Client{Transport: transport})
	result, err := provider.Deliver(context.Background(), contract.DeliveryRequest{
		OrderNo:    "NL3",
		TotalPrice: 10,
		Quantity:   1,
		FormValues: map[string]string{
			"__plugin_config":  `{"base_url":"https://new-api.example.com","admin_user_id":"7","quota_per_nl":"100"}`,
			"__plugin_secrets": `{"admin_access_token":"secret-token"}`,
			"nl_amount":        "10",
		},
	})
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if result.Uncertain {
		t.Fatalf("data string envelope marked uncertain: %+v", result)
	}
	if !strings.Contains(result.Content, "abc123xyz7890") {
		t.Fatalf("delivery content = %q", result.Content)
	}
}

func TestNewAPIBareRedemptionObjectIsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"name":"","key":"abc123xyz7890","status":1,"quota":100000}`))
	}))
	defer server.Close()

	transport := &rewriteTransport{target: server.URL}
	provider := NewNewAPIRedemption(nil).withClient(&http.Client{Transport: transport})
	result, err := provider.Deliver(context.Background(), contract.DeliveryRequest{
		OrderNo:    "NL5",
		TotalPrice: 10,
		Quantity:   1,
		FormValues: map[string]string{
			"__plugin_config":  `{"base_url":"https://new-api.example.com","admin_user_id":"7","quota_per_nl":"100"}`,
			"__plugin_secrets": `{"admin_access_token":"secret-token"}`,
			"nl_amount":        "10",
		},
	})
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if result.Uncertain {
		t.Fatalf("bare redemption object marked uncertain: %+v", result)
	}
	if !strings.Contains(result.Content, "abc123xyz7890") {
		t.Fatalf("delivery content = %q", result.Content)
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
		OrderNo:    "NL6",
		TotalPrice: 10,
		Quantity:   1,
		FormValues: map[string]string{
			"__plugin_config":  `{"base_url":"https://new-api.example.com","admin_user_id":"7","quota_per_nl":"100"}`,
			"__plugin_secrets": `{"admin_access_token":"secret-token"}`,
			"nl_amount":        "10",
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

func TestNewAPIFailedEnvelopeIsAHardFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":false,"message":"quota invalid","data":null}`))
	}))
	defer server.Close()

	transport := &rewriteTransport{target: server.URL}
	provider := NewNewAPIRedemption(nil).withClient(&http.Client{Transport: transport})
	_, err := provider.Deliver(context.Background(), contract.DeliveryRequest{
		OrderNo:    "NL4",
		TotalPrice: 10,
		Quantity:   1,
		FormValues: map[string]string{
			"__plugin_config":  `{"base_url":"https://new-api.example.com","admin_user_id":"7","quota_per_nl":"100"}`,
			"__plugin_secrets": `{"admin_access_token":"secret-token"}`,
			"nl_amount":        "10",
		},
	})
	if err == nil {
		t.Fatal("success=false was accepted as created")
	}
	if !strings.Contains(err.Error(), "quota invalid") {
		t.Fatalf("provider message lost: %v", err)
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

func TestNewAPIQuotaBounds(t *testing.T) {
	if _, err := quotaForOrder(0, 1, 100); err == nil {
		t.Fatal("zero order amount was accepted")
	}
	if _, err := positiveQuota("0", "quota"); err == nil {
		t.Fatal("zero quota was accepted")
	}
	if _, err := positiveQuota("-1", "quota"); err == nil {
		t.Fatal("negative quota was accepted")
	}
	if _, err := positiveQuota("1.5", "quota"); err == nil {
		t.Fatal("fractional quota was accepted")
	}
	if _, err := quotaForOrder(1_000_000_000, 1, newAPIMaxQuota); err == nil {
		t.Fatal("overflowing quota was accepted")
	}
}
