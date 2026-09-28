package infrastructure

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/contract"
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
