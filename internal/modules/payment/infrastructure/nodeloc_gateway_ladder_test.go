package infrastructure

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/domain"
	"github.com/kaoqy/Nodeloc-Store/internal/shared"
)

// paymentStub is a NodeLoc that remembers which routes it was asked about and how
// many times. Half of what the gateway does right only shows up in a count: a
// ladder that retries a duplicate 下单, or a settings button that opens a payment.
type paymentStub struct {
	mu      sync.Mutex
	hits    map[string]int
	handler func(http.ResponseWriter, *http.Request)
}

func (s *paymentStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	s.mu.Lock()
	if s.hits == nil {
		s.hits = map[string]int{}
	}
	s.hits[r.URL.Path]++
	s.mu.Unlock()
	s.handler(w, r)
}

func (s *paymentStub) count(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[path]
}

func stubForm(r *http.Request) map[string]string {
	form := map[string]string{}
	for key, values := range r.PostForm {
		if key != "signature" && values[0] != "" {
			form[key] = values[0]
		}
	}
	return form
}

func jsonAnswer(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

// acceptsOnly signs with one credential and nothing else, so the only way the store
// succeeds is by working out which reading of the docs is live.
func acceptsOnly(key string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("signature") != shared.Sign(stubForm(r), key) {
			jsonAnswer(w, `{"success": false, "message": "Invalid signature"}`)
			return
		}
		jsonAnswer(w, `{"success": true, "data": {"transaction_id": "tx_1", "status": "pending"}}`)
	}
}

func writeGuarded(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`["BAD CSRF"]`))
}

func checkoutPath() string { return "/payment/pay/pay_test/process" }
func queryPath() string    { return "/payment/query/pay_test" }
func transferPath() string { return "/payment/transfer/pay_test" }

func payRequest() contract.CreatePaymentRequest {
	return contract.CreatePaymentRequest{Amount: 100, Description: "Order NL1", OrderID: "NL1"}
}

// The documentation and this repository's own upstream client disagree about which
// credential signs 下单: hex(SHA256(tk_xxx)) with a timestamp, or hex(SHA256(secret
// key)) with none. Only one is live on a given NodeLoc release, so the store finds
// out and remembers it instead of betting the checkout on a doc page.
func TestLadderFindsAndRemembersTheWorkingConvention(t *testing.T) {
	stub := &paymentStub{handler: acceptsOnly(digestOf("sk_test"))}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())

	if _, err := gateway.CreatePayment(context.Background(), payRequest()); err != nil {
		t.Fatalf("下单 through the ladder: %v", err)
	}
	// The documented convention first, then the one the upstream client used.
	if got := stub.count(checkoutPath()); got != 2 {
		t.Fatalf("the ladder took %d attempts, want 2", got)
	}
	if style := gateway.SigningStyle(); !strings.Contains(style, "下单：Secret Key 的 SHA-256、不带时间戳") {
		t.Fatalf("signing style = %q, want the accepted convention named", style)
	}

	if _, err := gateway.CreatePayment(context.Background(), payRequest()); err != nil {
		t.Fatalf("second 下单: %v", err)
	}
	if got := stub.count(checkoutPath()); got != 3 {
		t.Fatalf("the remembered convention was not tried first: %d attempts in total, want 3", got)
	}
}

// A second 下单 for the same order is refused with 「Order already exists」 whatever
// key signs it, and the endpoint behind it holds the buyer's money. Running the
// ladder against it is four shots at a duplicate charge.
func TestDuplicateOrderIsNeverRetriedWithAnotherKey(t *testing.T) {
	stub := &paymentStub{handler: func(w http.ResponseWriter, _ *http.Request) {
		jsonAnswer(w, `{"success": false, "message": "Order already exists with status paid"}`)
	}}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())

	_, err := gateway.CreatePayment(context.Background(), payRequest())
	var refusal *domain.PaymentAlreadyRequested
	if !errors.As(err, &refusal) || refusal.Status != "paid" {
		t.Fatalf("duplicate 下单 = %v, want the typed refusal", err)
	}
	if got := stub.count(checkoutPath()); got != 1 {
		t.Fatalf("a duplicate order was submitted %d times", got)
	}
}

// 「{"status":404,"error":"Not Found"}」 is what the real provider answers for a
// Payment ID it does not have. That is one field on 设置, and no signing key turns
// it into a different answer, so the store stops and says which field.
func TestUnknownPaymentIDStopsTheLadder(t *testing.T) {
	for _, path := range []string{checkoutPath(), transferPath()} {
		stub := &paymentStub{handler: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status":404,"error":"Not Found"}`))
		}}
		server := httptest.NewServer(stub)
		gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())

		var err error
		if path == transferPath() {
			_, err = gateway.Transfer(context.Background(), contract.TransferRequest{ToUserID: "7", Amount: 5, OrderID: "NLG1"})
		} else {
			_, err = gateway.CreatePayment(context.Background(), payRequest())
		}
		server.Close()

		if !errors.Is(err, domain.ErrPaymentAppNotFound) {
			t.Fatalf("%s with an unknown Payment ID = %v", path, err)
		}
		if got := stub.count(path); got != 1 {
			t.Fatalf("%s was tried %d times, want 1", path, got)
		}
	}
}

// 查单 on the real provider is Discourse's merchant-browser route: every server-side
// shape comes back 403 ["BAD CSRF"]. Taken literally that strands paid money, so the
// answer is asked of 下单 instead — idempotent per order_id, which is what makes it
// safe to use as an oracle.
func TestGuardedQueryIsAnsweredThroughCheckout(t *testing.T) {
	stub := &paymentStub{handler: func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == queryPath() {
			writeGuarded(w)
			return
		}
		jsonAnswer(w, `{"success": false, "message": "Order already exists with status paid"}`)
	}}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())

	result, err := gateway.QueryPayment(context.Background(), contract.QueryPaymentRequest{
		TransactionID: "tx_1", OrderID: "NL1", Amount: 100, Description: "Order NL1",
	})
	if err != nil {
		t.Fatalf("核实 through 下单: %v", err)
	}
	if result.Via != "reprocess" || result.Status != "succeeded" {
		t.Fatalf("核实 result = %+v, want a succeeded payment via reprocess", result)
	}
	if _, err := gateway.QueryPayment(context.Background(), contract.QueryPaymentRequest{
		TransactionID: "tx_2", OrderID: "NL2", Amount: 100,
	}); err != nil {
		t.Fatalf("second 核实: %v", err)
	}
	if got := stub.count(queryPath()); got != 1 {
		t.Fatalf("查单 was asked %d times, want the guard remembered after the first", got)
	}
	if style := gateway.SigningStyle(); !strings.Contains(style, "查单：NodeLoc 只认浏览器会话") {
		t.Fatalf("signing style = %q", style)
	}
}

// 「测试支付网关」 is a button, and a button must never open a payment. The probe
// carries a transaction id and no order, so with 查单 guarded it can only report
// what it found.
func TestProbeWithoutAnOrderNeverChecksOut(t *testing.T) {
	stub := &paymentStub{handler: func(w http.ResponseWriter, _ *http.Request) { writeGuarded(w) }}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())

	_, err := gateway.QueryPayment(context.Background(), contract.QueryPaymentRequest{TransactionID: "probe-1"})
	if !errors.Is(err, domain.ErrProviderGuarded) {
		t.Fatalf("probe = %v, want the guarded diagnosis", err)
	}
	if got := stub.count(checkoutPath()); got != 0 {
		t.Fatalf("the probe submitted %d 下单 requests", got)
	}
	if !gateway.queryIsGuarded() {
		t.Fatal("the guarded 查单 was not remembered")
	}
}

// A 404 from 查单 is ambiguous: it is the same answer for an unknown Payment ID and
// for a transaction id the provider has simply never seen. Resolving it through the
// order rather than declaring the store misconfigured is what keeps a paid order
// settleable.
func TestNotFoundQueryIsResolvedThroughTheOrder(t *testing.T) {
	stub := &paymentStub{handler: func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == queryPath() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status":404,"error":"Not Found"}`))
			return
		}
		jsonAnswer(w, `{"success": false, "message": "Order already exists with status success"}`)
	}}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())

	result, err := gateway.QueryPayment(context.Background(), contract.QueryPaymentRequest{
		TransactionID: "tx_9", OrderID: "NL9", Amount: 100,
	})
	if err != nil {
		t.Fatalf("核实 after a 404 查单: %v", err)
	}
	if result.Via != "reprocess" || result.Status != "succeeded" {
		t.Fatalf("核实 result = %+v, want the settled payment read off 下单", result)
	}
}

// The same 404 reaches 「测试支付网关」 without an order to ask about, and there it
// is not ambiguous: a live 查单 route answers a server-side call with 「["BAD CSRF"]」,
// so a route that replies 404 is a route this forum does not have — one wrong field
// on the 设置 page, and the page has to say which one.
func TestProbeReadsAFourOhFourAsAWrongPaymentID(t *testing.T) {
	stub := &paymentStub{handler: func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status":404,"error":"Not Found","message":"The resource you requested could not be found."}`))
	}}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_wrong", "tk_test", "sk_test", server.Client())

	_, err := gateway.QueryPayment(context.Background(), contract.QueryPaymentRequest{TransactionID: "nodeloc-store-connectivity-probe"})
	if !errors.Is(err, domain.ErrPaymentAppNotFound) {
		t.Fatalf("probe with a 404 route = %v, want the Payment ID named", err)
	}
	if got := stub.count(checkoutPath()); got != 0 {
		t.Fatalf("the probe submitted %d 下单 requests", got)
	}
}

// A drifted clock makes NodeLoc refuse the request timestamp. The credentials are
// right and every payment route fails, so re-signing with three more keys would
// only bury the diagnosis inside a signature complaint.
func TestClockRefusalIsItsOwnAnswer(t *testing.T) {
	stub := &paymentStub{handler: func(w http.ResponseWriter, _ *http.Request) {
		jsonAnswer(w, `{"success": false, "message": "Request timestamp expired"}`)
	}}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())

	_, err := gateway.CreatePayment(context.Background(), payRequest())
	if !errors.Is(err, domain.ErrProviderClockSkew) {
		t.Fatalf("clock refusal = %v", err)
	}
	if got := stub.count(checkoutPath()); got != 1 {
		t.Fatalf("the ladder ran %d attempts against a drifted clock", got)
	}
}

// NodeLoc's console hands out tk_xxx and pay_xxx, and its docs show them quoted.
// One stray quote is a different HMAC key on every call and an error that reads
// like a credential mix-up, so the paste artefacts come off at construction.
func TestQuotedCredentialsStillSign(t *testing.T) {
	stub := &paymentStub{handler: acceptsOnly(shared.HashedTokenKey("tk_test"))}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(`"`+server.URL+`"`, "`pay_test`", "'tk_test'", `"sk_test"`, server.Client())

	if gateway.baseURL != server.URL || gateway.paymentID != "pay_test" {
		t.Fatalf("settings kept paste artefacts: base=%q payment id=%q", gateway.baseURL, gateway.paymentID)
	}
	if gateway.token != "tk_test" || gateway.secretKey != "sk_test" {
		t.Fatalf("credentials kept paste artefacts: token=%q secret=%q", gateway.token, gateway.secretKey)
	}
	if _, err := gateway.CreatePayment(context.Background(), payRequest()); err != nil {
		t.Fatalf("下单 with quoted credentials: %v", err)
	}
}

// Some NodeLoc payment applications issue one secret, and the client in this
// repository that demonstrably took money signed 下单 with HMAC over
// SHA-256(thatSecret) and sent no timestamp. Requiring a second box to be filled
// turned a working single key into 「未配置」, so one credential is enough.
func TestASingleSecretIsEnoughToCheckout(t *testing.T) {
	stub := &paymentStub{handler: acceptsOnly(digestOf("sk_only"))}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "", "sk_only", server.Client())

	if missing := gateway.Missing(); len(missing) != 0 {
		t.Fatalf("one secret reported %v missing", missing)
	}
	if _, err := gateway.CreatePayment(context.Background(), payRequest()); err != nil {
		t.Fatalf("下单 with only a Secret Key: %v", err)
	}
	if style := gateway.SigningStyle(); !strings.Contains(style, "Secret Key 的 SHA-256") {
		t.Fatalf("signing style = %q, want the upstream convention", style)
	}
}

// The same single secret has to serve 转账 too: a refund that cannot sign is money
// the buyer is owed and the shop cannot pay.
func TestTransferWorksWithOneSecret(t *testing.T) {
	stub := &paymentStub{handler: acceptsOnly(shared.HashedTokenKey("tk_only"))}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_only", "", server.Client())

	if _, err := gateway.Transfer(context.Background(), contract.TransferRequest{
		ToUserID: "4242", Amount: 100, OrderID: "NL1",
	}); err != nil {
		t.Fatalf("转账 with only a Payment Token: %v", err)
	}
	if got := stub.count(transferPath()); got != 1 {
		t.Fatalf("转账 ran %d attempts, want the documented convention first", got)
	}
}

// NodeLoc answers a 转账 the shop's own balance cannot cover with one sentence
// about the balance. That is not a signing question — no other key turns 余额不足
// into 成功 — and four identical POSTs at the transfer endpoint is how one refused
// transfer becomes an unexplained row in the ledger. The provider's words have to
// reach the operator intact, because the numbers in them are the diagnosis.
func TestABalanceRefusalIsOneRequestNotFour(t *testing.T) {
	stub := &paymentStub{handler: func(w http.ResponseWriter, _ *http.Request) {
		jsonAnswer(w, `{"success": false, "message": "Insufficient balance (need 5000, have 12)"}`)
	}}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())

	_, err := gateway.Transfer(context.Background(), contract.TransferRequest{
		ToUserID: "4242", ToUsername: "buyer", Amount: 30, OrderID: "NLG1",
	})
	var refused *domain.ProviderRefusal
	if !errors.As(err, &refused) || !strings.Contains(refused.Message, "need 5000") {
		t.Fatalf("余额不足的转账 = %v, want NodeLoc's own sentence kept", err)
	}
	if strings.Contains(err.Error(), "依次试了") {
		t.Fatalf("a business refusal was reported as a signing failure: %v", err)
	}
	if got := stub.count(transferPath()); got != 1 {
		t.Fatalf("the balance refusal brought %d 转账 attempts, want 1", got)
	}
	// The key was accepted for the provider to be able to answer about the balance.
	if style := gateway.SigningStyle(); !strings.Contains(style, "转账：") {
		t.Fatalf("signing style = %q, want the accepted convention remembered", style)
	}
}

// A refusal that says nothing this store recognises must not end the discovery:
// the whole reason the ladder exists is that no document says which key is live,
// and a terse 「error」 is exactly what a wrong key looks like.
func TestAnUnexplainedRefusalStillWalksTheLadder(t *testing.T) {
	// The documented convention is tried first, so the ambiguous refusal has to
	// push the ladder along to the second one.
	key := digestOf("sk_test")
	stub := &paymentStub{handler: func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("signature") != shared.Sign(stubForm(r), key) {
			jsonAnswer(w, `{"success": false, "message": "error"}`)
			return
		}
		jsonAnswer(w, `{"success": true, "data": {"transaction_id": "TX1", "status": "pending"}}`)
	}}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())

	if _, err := gateway.Transfer(context.Background(), contract.TransferRequest{
		ToUserID: "4242", Amount: 5, OrderID: "NLG2",
	}); err != nil {
		t.Fatalf("转账 through the ladder: %v", err)
	}
	if got := stub.count(transferPath()); got != 2 {
		t.Fatalf("the ladder took %d attempts, want the ambiguous refusal retried once", got)
	}
}

// The 查单 guard is a fact about NodeLoc's route, and routes change: the endpoint
// can be opened, or 支付 API 地址 can get corrected. Holding the decision for the
// life of the process would quietly settle every order through 下单 forever, so
// the store asks again once the window has passed.
func TestTheGuardedQueryIsReaskedOnceTheWindowPasses(t *testing.T) {
	queryAnswered := false
	stub := &paymentStub{handler: func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == queryPath() {
			if queryAnswered {
				jsonAnswer(w, `{"success": true, "data": {"transaction_id": "TX1", "external_reference": "NL1", "amount": 100, "status": "completed"}}`)
				return
			}
			writeGuarded(w)
			return
		}
		jsonAnswer(w, `{"success": false, "message": "Order already exists with status pending"}`)
	}}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())
	request := contract.QueryPaymentRequest{TransactionID: "TX1", OrderID: "NL1", Amount: 100}

	result, err := gateway.QueryPayment(context.Background(), request)
	if err != nil {
		t.Fatalf("guarded 查单: %v", err)
	}
	if result.Via != "reprocess" || !strings.Contains(result.Fallback, "BAD CSRF") {
		t.Fatalf("查单 result = via %q fallback %q, want the 下单 route and NodeLoc's reason", result.Via, result.Fallback)
	}
	if _, err := gateway.QueryPayment(context.Background(), request); err != nil {
		t.Fatalf("second 查单: %v", err)
	}
	if got := stub.count(queryPath()); got != 1 {
		t.Fatalf("the guarded route was asked %d times inside its window, want 1", got)
	}

	queryAnswered = true
	gateway.queryGuardedAt = time.Now().Add(-queryGuardHold - time.Minute)
	settled, err := gateway.QueryPayment(context.Background(), request)
	if err != nil {
		t.Fatalf("查单 after the window: %v", err)
	}
	if settled.Via != "query" || settled.Status != domain.StatusSucceeded {
		t.Fatalf("查单 came back via %q status %q, want the route re-asked and paid", settled.Via, settled.Status)
	}
}

// A proxy page over HTTP 200 is not Discourse's session guard, which answers 403 with
// 「["BAD CSRF"]」. Treating it as one would let a transient page take the 查单
// route out of use for a whole window, and the orders behind it would settle
// through the fallback while 设置 looked perfectly healthy.
func TestAPageOverHttpTwoHundredDoesNotRetireTheQueryRoute(t *testing.T) {
	page := true
	stub := &paymentStub{handler: func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case queryPath():
			if page {
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte("<html>502 bad gateway</html>"))
				return
			}
			jsonAnswer(w, `{"success": true, "data": {"transaction_id": "TX1", "external_reference": "NL1", "amount": 100, "status": "completed"}}`)
		default:
			jsonAnswer(w, `{"success": false, "message": "Order already exists with status pending"}`)
		}
	}}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())
	request := contract.QueryPaymentRequest{TransactionID: "TX1", OrderID: "NL1", Amount: 100}

	fallback, err := gateway.QueryPayment(context.Background(), request)
	if err != nil {
		t.Fatalf("查单 answering a page: %v", err)
	}
	if fallback.Via != "reprocess" {
		t.Fatalf("via = %q, want this order verified through 下单", fallback.Via)
	}
	page = false
	settled, err := gateway.QueryPayment(context.Background(), request)
	if err != nil {
		t.Fatalf("查单 after the page: %v", err)
	}
	if settled.Via != "query" {
		t.Fatalf("via = %q, want the route tried again, not held back", settled.Via)
	}
	if got := stub.count(queryPath()); got != 2 {
		t.Fatalf("查单 was asked %d times, want one per call", got)
	}
}

// What the live forum actually answers for a Payment ID it does not have: 下单
// (/payment/pay/{id}/process) and 转账 (/payment/transfer/{id}) are not CSRF-guarded, they
// look the payment application up first, and the 404 comes back either as the plugin's own
// {"status":404,"error":"Not Found"} or, when the request does not ask for JSON, as a 404
// with no body at all.
// The store used to require a body wearing the forum's JSON envelope to recognise this, so
// either answer read as 「Discourse 拒绝了签名」: four POSTs at an endpoint that never had the
// application, and a diagnosis telling the owner to retype credentials that were fine. The
// bug a shop sees as 「nodeloc 相关功能全都用不了」 stays invisible when the fix advice points
// at the wrong box.
func TestAnEmptyFourOhFourNamesThePaymentIdNotTheKey(t *testing.T) {
	stub := &paymentStub{handler: func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())

	_, err := gateway.CreatePayment(context.Background(), payRequest())
	if !errors.Is(err, domain.ErrPaymentAppNotFound) {
		t.Fatalf("空正文 404 = %v, want the Payment ID named", err)
	}
	if got := stub.count(checkoutPath()); got != 1 {
		t.Fatalf("an unknown Payment ID brought %d 下单 attempts, want 1", got)
	}
	if !strings.Contains(err.Error(), "Payment ID") || strings.Contains(err.Error(), "依次试了") {
		t.Fatalf("diagnosis = %v, want the settings field and no signing-ladder talk", err)
	}
	if style := gateway.SigningStyle(); strings.Contains(style, "下单：") {
		t.Fatalf("signing style = %q, want nothing remembered from a call NodeLoc never read", style)
	}
}

// The other 404 is a different field on the same page. A host with no payment extension
// answers the shop's 下单 path with the forum's own 「找不到页面」 HTML page, which means the
// 支付 API 地址 box points at a domain that has no payment routes — commonly the login mirror
// instead of the main site. Saying 「Payment ID 不存在」 here sends the owner to recopy a
// string that was already right.
func TestAForumFourOhFourPageNamesThePaymentApiAddress(t *testing.T) {
	stub := &paymentStub{handler: func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<!DOCTYPE html><html lang="zh-CN"><head><title>找不到页面 - NodeLoc</title></head><body>404</body></html>`))
	}}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())

	_, err := gateway.CreatePayment(context.Background(), payRequest())
	if !errors.Is(err, domain.ErrPaymentAppNotFound) {
		t.Fatalf("论坛 404 网页 = %v, want the payment address named", err)
	}
	if !strings.Contains(err.Error(), "支付 API 地址") {
		t.Fatalf("diagnosis = %v, want 「支付 API 地址」 pointed at", err)
	}
	if got := stub.count(checkoutPath()); got != 1 {
		t.Fatalf("a host with no payment route brought %d 下单 attempts, want 1", got)
	}
}

// The shop asks for JSON (Accept: application/json), and probed against the live forum that
// is what changes the 404 shapes: Discourse's router answers a path it has never heard of
// with {"errors":[…],"error_type":"not_found"} instead of its web page, while the payment
// plugin's own 「this Payment ID is not here」 stays {"status":404,"error":"Not Found"}. The
// two still name two different settings boxes, so the router's keys — not its translated
// text, which follows the forum's locale — have to be what decides.
func TestTheForumsRouterFourOhFourNamesThePaymentApiAddress(t *testing.T) {
	stub := &paymentStub{handler: func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":["The requested URL or resource could not be found."],"error_type":"not_found"}`))
	}}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())

	_, err := gateway.CreatePayment(context.Background(), payRequest())
	if !errors.Is(err, domain.ErrPaymentAppNotFound) {
		t.Fatalf("论坛路由的 404 JSON = %v, want the payment address named", err)
	}
	if !strings.Contains(err.Error(), "「支付 API 地址」") || strings.Contains(err.Error(), "「Payment ID」") {
		t.Fatalf("diagnosis = %v, want 「支付 API 地址」 and no blame on the Payment ID", err)
	}
	if got := stub.count(checkoutPath()); got != 1 {
		t.Fatalf("a path the forum does not know brought %d 下单 attempts, want 1", got)
	}
}

// Read on the wire, {"status":404,"error":"Not Found"} is the payment application lookup
// failing on a registered route. It must not be mistaken for the router's answer above, and
// its English 「Not Found」 must not be handed to the shop as if it were a signing complaint.
func TestThePluginsFourOhFourJsonNamesThePaymentId(t *testing.T) {
	stub := &paymentStub{handler: func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status":404,"error":"Not Found"}`))
	}}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())

	_, err := gateway.CreatePayment(context.Background(), payRequest())
	if !errors.Is(err, domain.ErrPaymentAppNotFound) {
		t.Fatalf("支付应用的 404 JSON = %v, want the Payment ID named", err)
	}
	if !strings.Contains(err.Error(), "Payment ID") || strings.Contains(err.Error(), "支付 API 地址") || strings.Contains(err.Error(), "依次试了") {
		t.Fatalf("diagnosis = %v, want 「Payment ID」 pointed at and no signing-ladder talk", err)
	}
	if got := stub.count(checkoutPath()); got != 1 {
		t.Fatalf("an unknown Payment ID brought %d 下单 attempts, want 1", got)
	}
}

// NodeLoc numbers every refusal: {"status":400,"errors":[{"code":1003,"detail":"…"}]},
// and the docs say the detail is written in the forum's own language. Reading that
// sentence in English is what made this store keep POSTing: a 1013 「每日积分转账达到上限」
// matched no English marker, so it counted as an unexplained rejection and the ladder
// fired four times at the transfer endpoint before telling the owner the credentials
// were wrong. The number says the same thing in any locale, so it decides now, and the
// provider's sentence stays beside it as evidence.
func TestADailyLimitCodeRefusesOnceInAnyLanguage(t *testing.T) {
	for _, detail := range []string{"每日积分转账达到上限", "Daily points transfer limit reached"} {
		stub := &paymentStub{handler: func(w http.ResponseWriter, _ *http.Request) {
			jsonAnswer(w, `{"status":400,"errors":[{"code":1013,"detail":"`+detail+`"}],"data":null}`)
		}}
		server := httptest.NewServer(stub)
		gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())
		_, err := gateway.Transfer(context.Background(), contract.TransferRequest{
			ToUserID: "9001", ToUsername: "ada", Amount: 20, OrderID: "NL1",
		})
		server.Close()
		if !errors.Is(err, domain.ErrProviderRejected) {
			t.Fatalf("%s = %v, want NodeLoc having answered and refused", detail, err)
		}
		if got := stub.count(transferPath()); got != 1 {
			t.Fatalf("%s brought %d 转账 requests, want 1", detail, got)
		}
		if strings.Contains(err.Error(), "依次试了") {
			t.Fatalf("%s was reported as a signing problem: %v", detail, err)
		}
		var refused *domain.ProviderRefusal
		if !errors.As(err, &refused) || refused.Code != domain.CodeDailyLimitReached {
			t.Fatalf("%s lost its code: %#v", detail, err)
		}
	}
}

// Code 1003 is the refusal no credential on this machine answers: the payment
// application only takes calls from the IP addresses its owner listed, and the check
// runs before the signature is read. A shop that moved server, or restarted a container
// onto a new address, sees every payment route refuse at once while 设置 is entirely
// correct — so this must be its own family, must not burn the ladder, and must not
// remember a signing style from a call NodeLoc never parsed.
func TestTheIPWhitelistCodeIsItsOwnFamily(t *testing.T) {
	stub := &paymentStub{handler: func(w http.ResponseWriter, _ *http.Request) {
		jsonAnswer(w, `{"status":400,"errors":[{"code":1003,"detail":"IP 不在白名单内"}],"data":null}`)
	}}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())

	_, err := gateway.CreatePayment(context.Background(), payRequest())
	if !errors.Is(err, domain.ErrProviderIPNotAllowed) {
		t.Fatalf("1003 = %v, want the IP family", err)
	}
	if got := stub.count(checkoutPath()); got != 1 {
		t.Fatalf("a refused IP brought %d 下单 attempts, want 1", got)
	}
	if !strings.Contains(err.Error(), "IP") || !strings.Contains(err.Error(), "IP 不在白名单内") {
		t.Fatalf("diagnosis = %v, want the IP named and NodeLoc's own words kept", err)
	}
	if style := gateway.SigningStyle(); style != "" {
		t.Fatalf("signing style = %q, want nothing remembered from a call whose key was never read", style)
	}
}

// 1001 is the one number that does invite another signing convention, so the ladder keeps
// going while it is what NodeLoc answers — and the last refusal still says so.
func TestTheSignatureCodeKeepsTheLadderMoving(t *testing.T) {
	stub := &paymentStub{handler: func(w http.ResponseWriter, _ *http.Request) {
		jsonAnswer(w, `{"status":400,"errors":[{"code":1001,"detail":"Invalid signature"}],"data":null}`)
	}}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())

	_, err := gateway.CreatePayment(context.Background(), payRequest())
	if !errors.Is(err, domain.ErrProviderRejected) {
		t.Fatalf("1001 = %v, want the provider having refused the call", err)
	}
	if got := stub.count(checkoutPath()); got != maxSigningAttempts {
		t.Fatalf("1001 brought %d 下单 attempts, want the whole ladder of %d", got, maxSigningAttempts)
	}
	if !strings.Contains(err.Error(), "依次试了") {
		t.Fatalf("diagnosis = %v, want the tried conventions listed for a key problem", err)
	}
}

// The 核实 route turns on this answer: a second 下单 for an order_id NodeLoc already holds
// comes back 1006 with the payment's own state in data.status. Reading the state out of the
// number and its data block — not out of a translated sentence — is what lets a paid order
// settle while 查单 stays behind the forum's browser session.
func TestTheDuplicateOrderCodeSettlesFromItsDataBlock(t *testing.T) {
	stub := &paymentStub{handler: func(w http.ResponseWriter, _ *http.Request) {
		jsonAnswer(w, `{"status":400,"errors":[{"code":1006,"detail":"订单已存在"}],"data":{"status":"paid","transaction_id":"tx_9"}}`)
	}}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())
	gateway.markQueryGuarded()

	settled, err := gateway.QueryPayment(context.Background(), contract.QueryPaymentRequest{
		TransactionID: "tx_9", OrderID: "NL1", Amount: 100, Description: "Order NL1",
	})
	if err != nil {
		t.Fatalf("核实 a 1006 order: %v", err)
	}
	if settled.Status != domain.StatusSucceeded {
		t.Fatalf("status = %q, want the paid order settled from data.status", settled.Status)
	}
	if settled.Via != "reprocess" {
		t.Fatalf("via = %q, want the 下单 fallback", settled.Via)
	}
	if got := stub.count(checkoutPath()); got != 1 {
		t.Fatalf("核实 fired %d 下单 requests, want 1", got)
	}
}

// 转账's documented amount field is named **points**; the upstream client this store
// descends from sent 「amount」. Only one of the two can be the live word on a given
// release, and sending both would sign a request whose meaning the provider has to
// guess, so points goes out first and a 缺少参数 answer is answered with exactly one
// retry under the old spelling.
func TestTransferSendsTheDocumentedPointsField(t *testing.T) {
	var sent []map[string]string
	stub := &paymentStub{handler: func(w http.ResponseWriter, r *http.Request) {
		form := map[string]string{}
		for key, values := range r.PostForm {
			form[key] = values[0]
		}
		sent = append(sent, form)
		jsonAnswer(w, `{"status":200,"data":{"transaction_id":"tx_1","status":"completed"}}`)
	}}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())

	result, err := gateway.Transfer(context.Background(), contract.TransferRequest{
		ToUserID: "9001", ToUsername: "ada", Amount: 20, OrderID: "refund-NL1",
	})
	if err != nil {
		t.Fatalf("转账: %v", err)
	}
	if result.Status != domain.StatusSucceeded {
		t.Fatalf("status = %q, want the completed transfer read back", result.Status)
	}
	if len(sent) != 1 {
		t.Fatalf("转账 fired %d requests, want 1: %v", len(sent), sent)
	}
	if sent[0]["points"] != "20" || sent[0]["to_user_id"] != "9001" || sent[0]["order_id"] != "refund-NL1" {
		t.Fatalf("sent %#v, want the documented points/recipient/reference", sent[0])
	}
	if _, ok := sent[0]["amount"]; ok {
		t.Fatalf("sent %#v, which sends the undocumented spelling beside the documented one", sent[0])
	}
}

// A release that still reads the old word answers 1004 缺少参数. That is the one refusal
// worth a different spelling of the same money, and it is worth exactly one: a shop whose
// 转账 is refused twice must not keep reposting a refund.
func TestTransferRetriesOnceUnderTheLegacyAmountField(t *testing.T) {
	var sent []map[string]string
	stub := &paymentStub{handler: func(w http.ResponseWriter, r *http.Request) {
		form := map[string]string{}
		for key, values := range r.PostForm {
			form[key] = values[0]
		}
		sent = append(sent, form)
		if _, ok := form["amount"]; !ok {
			jsonAnswer(w, `{"status":400,"errors":[{"code":1004,"detail":"Missing required parameter: amount"}],"data":null}`)
			return
		}
		jsonAnswer(w, `{"status":200,"data":{"transaction_id":"tx_1","status":"completed"}}`)
	}}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())

	result, err := gateway.Transfer(context.Background(), contract.TransferRequest{
		ToUserID: "9001", ToUsername: "ada", Amount: 20, OrderID: "refund-NL2",
	})
	if err != nil {
		t.Fatalf("转账: %v", err)
	}
	if result.Status != domain.StatusSucceeded {
		t.Fatalf("status = %q, want the completed transfer read back", result.Status)
	}
	if len(sent) != 2 {
		t.Fatalf("转账 fired %d requests, want 2: %v", len(sent), sent)
	}
	if sent[1]["amount"] != "20" {
		t.Fatalf("retry sent %#v, want amount=20", sent[1])
	}
	if _, ok := sent[1]["points"]; ok {
		t.Fatalf("retry sent %#v, which repeats the spelling just refused", sent[1])
	}
	if got := stub.count(transferPath()); got != 2 {
		t.Fatalf("转账 route hit %d times, want 2", got)
	}
}

// The same number names a different field, and that decides whether a second POST can
// help: 1004 for a missing amount can be re-spelled, 1004 for a missing to_username is
// this store's uid-only buyer, which no renaming of a number answers. Firing anyway buys
// a money endpoint twice for an answer that was already complete.
func TestTransferDoesNotReSpellTheAmountForAMissingRecipient(t *testing.T) {
	var sent []map[string]string
	stub := &paymentStub{handler: func(w http.ResponseWriter, r *http.Request) {
		form := map[string]string{}
		for key, values := range r.PostForm {
			form[key] = values[0]
		}
		sent = append(sent, form)
		jsonAnswer(w, `{"status":400,"errors":[{"code":1004,"detail":"Missing required parameter: to_username"}],"data":null}`)
	}}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())

	_, err := gateway.Transfer(context.Background(), contract.TransferRequest{
		ToUserID: "9001", Amount: 20, OrderID: "refund-NL3",
	})
	if err == nil {
		t.Fatal("a refusal naming the missing recipient answered as success")
	}
	for _, form := range sent {
		if _, ok := form["amount"]; ok {
			t.Fatalf("转账 retried under the legacy amount field after a recipient refusal: %v", sent)
		}
	}
}

// The same answer without a number to act on still says 「缺参数」, and the recipient half
// of it is the one that must not be retried either.
func TestTransferWithoutAnumberedRefusalStillReadsTheRecipientField(t *testing.T) {
	var sent []map[string]string
	stub := &paymentStub{handler: func(w http.ResponseWriter, r *http.Request) {
		form := map[string]string{}
		for key, values := range r.PostForm {
			form[key] = values[0]
		}
		sent = append(sent, form)
		jsonAnswer(w, `{"success": false, "message": "Missing required parameter: to_username"}`)
	}}
	server := httptest.NewServer(stub)
	defer server.Close()
	gateway := NewNodeLocGateway(server.URL, "pay_test", "tk_test", "sk_test", server.Client())

	if _, err := gateway.Transfer(context.Background(), contract.TransferRequest{
		ToUserID: "9001", Amount: 20, OrderID: "refund-NL4",
	}); err == nil {
		t.Fatal("an unnumbered recipient refusal answered as success")
	}
	for _, form := range sent {
		if _, ok := form["amount"]; ok {
			t.Fatalf("转账 retried under the legacy amount field after a recipient refusal: %v", sent)
		}
	}
}
