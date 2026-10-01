package infrastructure

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/domain"
	"github.com/kaoqy/Nodeloc-Store/internal/shared"
)

// A buyer can be bound to NodeLoc by uid alone, so 转账 goes out with an empty
// to_username. Whatever the provider does with a blank field, it can only sign
// what it received — and a server that drops blanks would then reject a
// signature computed over them. This test plays that server.
func TestTransferSignsOnlyTheParametersItSends(t *testing.T) {
	const token = "tk_test"
	var received map[string][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		received = r.PostForm
		unsigned := map[string]string{}
		for key, values := range r.PostForm {
			if key != "signature" && values[0] != "" {
				unsigned[key] = values[0]
			}
		}
		if r.FormValue("signature") != shared.Sign(unsigned, shared.HashedTokenKey(token)) {
			http.Error(w, `{"success": false, "message": "signature mismatch"}`, http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success": true, "data": {"transaction_id": "tx_1", "status": "success"}}`))
	}))
	defer server.Close()

	gateway := NewNodeLocGateway(server.URL, "pay_test", token, "sk_test", server.Client())
	result, err := gateway.Transfer(context.Background(), contract.TransferRequest{
		ToUserID: "4242",
		Amount:   100,
		OrderID:  "NL1",
	})
	if err != nil {
		t.Fatalf("transfer to a username-less buyer: %v", err)
	}
	if result.TransactionID != "tx_1" {
		t.Fatalf("transaction id = %q", result.TransactionID)
	}
	if _, sent := received["to_username"]; sent {
		t.Fatalf("an empty recipient name was still sent: %v", received)
	}
}

// recording starts a provider that answers with body and checks the signature
// the way NodeLoc documents it: recompute over the parameters actually received,
// HMAC-SHA256, `signature` excluded.
func recording(t *testing.T, token string, body string) (*NodeLocGateway, *map[string][]string) {
	t.Helper()
	received := map[string][]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		received = r.PostForm
		unsigned := map[string]string{}
		for key, values := range r.PostForm {
			if key != "signature" {
				unsigned[key] = values[0]
			}
		}
		if r.FormValue("signature") != shared.Sign(unsigned, shared.HashedTokenKey(token)) {
			http.Error(w, `{"success": false, "message": "signature mismatch"}`, http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return NewNodeLocGateway(server.URL, "pay_test", token, "sk_test", server.Client()), &received
}

// The docs put a 10-digit second timestamp on every payment call and refuse one
// more than five minutes from their clock. A store that never sends it fails 下单,
// 查单 and 转账 at once, which reads to the shop owner as broken credentials.
func TestEveryPaymentCallCarriesAFreshTimestamp(t *testing.T) {
	gateway, received := recording(t, "tk_test", `{"success": true, "data": {"transaction_id": "tx_1", "status": "pending"}}`)
	if _, err := gateway.CreatePayment(context.Background(), contract.CreatePaymentRequest{
		Amount: 100, Description: "Order NL1", OrderID: "NL1",
	}); err != nil {
		t.Fatalf("下单 with a timestamp: %v", err)
	}
	stamp := (*received)["timestamp"]
	if len(stamp) != 1 || len(stamp[0]) != 10 {
		t.Fatalf("timestamp = %v, want one 10-digit value", *received)
	}
	seconds, err := strconv.ParseInt(stamp[0], 10, 64)
	if err != nil {
		t.Fatalf("timestamp %q is not a second count: %v", stamp[0], err)
	}
	if delta := time.Now().Unix() - seconds; delta < 0 || delta > 5 {
		t.Fatalf("timestamp is %d seconds from now", delta)
	}
}

// 「Order already exists with status …」 is what a buyer pressing 立即购买 a second
// time gets back. Read as a plain refusal it is the message that makes people pay
// twice, so the gateway hands it up typed, with NodeLoc's own status word.
func TestDuplicateCheckoutComesBackTyped(t *testing.T) {
	gateway, _ := recording(t, "tk_test", `{"success": false, "message": "Order already exists with status paid"}`)
	_, err := gateway.CreatePayment(context.Background(), contract.CreatePaymentRequest{
		Amount: 100, Description: "Order NL1", OrderID: "NL1",
	})
	var refusal *domain.PaymentAlreadyRequested
	if !errors.As(err, &refusal) {
		t.Fatalf("duplicate 下单 = %v, want a typed refusal", err)
	}
	if refusal.Status != "paid" {
		t.Fatalf("refusal status = %q", refusal.Status)
	}
	if !errors.Is(err, domain.ErrProviderRejected) {
		t.Fatalf("a duplicate must still read as a provider rejection: %v", err)
	}
}

func TestAlreadyRequestedListensInBothLanguages(t *testing.T) {
	for _, message := range []string{
		"Order already exists with status pending",
		"NodeLoc returned HTTP 400: {\"success\":false,\"message\":\"订单已存在，状态为 processing\"}",
	} {
		status, ok := alreadyRequestedStatus(message)
		if !ok {
			t.Fatalf("%q was not read as a duplicate order", message)
		}
		if !strings.Contains(status, "pending") && !strings.Contains(status, "processing") {
			t.Fatalf("%q produced status %q", message, status)
		}
	}
	if _, ok := alreadyRequestedStatus("signature mismatch"); ok {
		t.Fatal("a signature refusal is not a duplicate order")
	}
}

// NodeLoc's own pages disagree about the callback key: one names the Secret Key,
// another the SHA-256 of the tk_xxx token. Either can be the real one, and a
// redirect that fails to verify is a paid order left showing 待支付.
func TestCallbackAcceptsEitherDocumentedKey(t *testing.T) {
	gateway := NewNodeLocGateway("https://pay.invalid", "pay_test", "tk_test", "sk_test", nil)
	params := map[string]string{"order_id": "NL1", "transaction_id": "tx_1", "amount": "100", "paid_at": "1730000000"}
	for name, key := range map[string]string{
		"secret key":   "sk_test",
		"hashed token": shared.HashedTokenKey("tk_test"),
		"someone else": "not-our-credential",
	} {
		signed := map[string]string{}
		for k, v := range params {
			signed[k] = v
		}
		signed["signature"] = shared.Sign(signed, key)
		verified := gateway.VerifyCallback(signed)
		if want := name != "someone else"; verified != want {
			t.Fatalf("redirect signed with the %s: verified=%v, want %v", name, verified, want)
		}
	}
}

// A payment application that issues one secret may have had it pasted into the
// Token box. NodeLoc then mirrors the redirect with that same value, and a callback
// the store cannot verify is a paid order left showing 待支付 — so the raw token is
// the last spelling tried here.
func TestCallbackFromASingleSecretInEitherBox(t *testing.T) {
	params := map[string]string{"order_id": "NL1", "transaction_id": "tx_1", "amount": "100", "paid_at": "1730000000"}
	for name, gateway := range map[string]*NodeLocGateway{
		"secret box": NewNodeLocGateway("https://pay.invalid", "pay_test", "", "only-secret", nil),
		"token box":  NewNodeLocGateway("https://pay.invalid", "pay_test", "only-secret", "", nil),
	} {
		for _, key := range []string{"only-secret", shared.HashedTokenKey("only-secret"), digestOf("only-secret")} {
			signed := map[string]string{}
			for k, v := range params {
				signed[k] = v
			}
			signed["signature"] = shared.Sign(signed, key)
			if !gateway.VerifyCallback(signed) {
				t.Fatalf("%s: a redirect signed with %q was refused", name, key)
			}
		}
		foreign := map[string]string{}
		for k, v := range params {
			foreign[k] = v
		}
		foreign["signature"] = shared.Sign(foreign, "not-our-credential")
		if gateway.VerifyCallback(foreign) {
			t.Fatalf("%s: a redirect signed with someone else's key was accepted", name)
		}
	}
}
