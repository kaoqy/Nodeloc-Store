package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/domain"
	"github.com/kaoqy/Nodeloc-Store/internal/shared"
)

// NodeLocGateway implements contract.PaymentGateway using NodeLoc Payment.
//
// NodeLoc hands a merchant three separate credentials and its documentation does
// not use them consistently: the Payment ID (pay_xxx) selects the application in
// the URL, the Token (tk_xxx) is documented as the 下单/转账 key *after* a
// SHA-256 pass, and the Secret Key signs 查单 as issued. This repository's own
// upstream client — the code that used to take money here — signed every outbound
// call with the SHA-256 of the Secret Key and sent no timestamp at all. Only one
// of those conventions is live on any given NodeLoc release, and betting the
// storefront on a single reading is how every payment feature dies at once. So a
// call NodeLoc refuses for a signature-shaped reason is retried with the next
// convention, and the convention that worked is remembered for this process.
type NodeLocGateway struct {
	baseURL   string
	paymentID string
	token     string
	secretKey string
	client    *http.Client

	mu sync.Mutex
	// styles remembers which signing convention NodeLoc accepted per route, so a
	// store that had to discover one does not rediscover it on every call.
	styles map[routeKind]string
	// queryGuardedAt is when 查单 last answered 「browser session only」. It
	// expires, because that guard is a fact about NodeLoc's route and routes
	// change — see queryIsGuarded.
	queryGuardedAt time.Time
}

// queryGuardHold is how long one guarded answer keeps the store off the 查单
// route. Holding it for the life of the process would strand 查单 until a
// restart, so the shop re-asks once the window has passed and marks it guarded
// again if NodeLoc says the same thing.
const queryGuardHold = 15 * time.Minute

// routeKind is the axis the documentation disagrees on: which credential each
// endpoint is signed with.
type routeKind string

const (
	routePay      routeKind = "pay"
	routeQuery    routeKind = "query"
	routeTransfer routeKind = "transfer"
)

// maxSigningAttempts bounds the ladder. A misconfigured store must not turn every
// checkout into five POSTs at the provider; four covers the documented
// convention, the one this repository's upstream client used, and the timestamp
// variants of each — after that the shop is told it is a credential problem.
const maxSigningAttempts = 4

// credentialSource names which stored setting a signing attempt is built from, so
// a diagnostic can point at a field on the 设置 page instead of a theory.
type credentialSource string

const (
	sourceTokenHash  credentialSource = "token_hash"
	sourceSecretHash credentialSource = "secret_hash"
	sourceSecretRaw  credentialSource = "secret_raw"
)

// signingAttempt is one complete reading of "how this store signs a NodeLoc
// payment call": which credential becomes the HMAC key, and whether the
// documented timestamp goes out with it.
type signingAttempt struct {
	source    credentialSource
	key       string
	timestamp bool
}

func NewNodeLocGateway(baseURL, paymentID, token, secretKey string, client *http.Client) *NodeLocGateway {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &NodeLocGateway{
		baseURL:   strings.TrimRight(shared.TrimCredential(baseURL), "/"),
		paymentID: shared.TrimCredential(paymentID),
		token:     shared.TrimCredential(token),
		secretKey: shared.TrimCredential(secretKey),
		client:    client,
		styles:    map[routeKind]string{},
	}
}

// Missing reports which of the four payment settings are still empty, named the
// way the 设置 page names its fields. A storefront that cannot take money is
// otherwise diagnosed by guessing, and the guess is usually wrong.
func (g *NodeLocGateway) Missing() []string {
	missing := make([]string, 0, 4)
	if g == nil {
		return append(missing, "payment 网关未初始化")
	}
	if strings.TrimSpace(g.baseURL) == "" {
		missing = append(missing, "base_url")
	}
	if strings.TrimSpace(g.paymentID) == "" {
		missing = append(missing, "payment_id")
	}
	// One secret is a complete credential: the ladder signs with whichever box the
	// owner filled, in both the raw and the SHA-256 spelling. Requiring both made a
	// store with a working single key read as 未配置.
	if strings.TrimSpace(g.token) == "" && strings.TrimSpace(g.secretKey) == "" {
		missing = append(missing, "token", "secret_key")
	}
	return missing
}

// SigningStyle names the conventions NodeLoc accepted for this process. Empty
// means nothing had to be discovered: the documented convention worked first
// time. The 设置 page reports it so a shop owner can see which key the store is
// really signing with instead of comparing two contradictory doc pages.
func (g *NodeLocGateway) SigningStyle() string {
	if g == nil {
		return ""
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	parts := make([]string, 0, 4)
	for _, kind := range []routeKind{routePay, routeQuery, routeTransfer} {
		if style := g.styles[kind]; style != "" {
			parts = append(parts, routeKindLabels[kind]+"："+style)
		}
	}
	if !g.queryGuardedAt.IsZero() && time.Since(g.queryGuardedAt) < queryGuardHold {
		parts = append(parts, "查单：NodeLoc 只认浏览器会话，已改用下单核实")
	}
	return strings.Join(parts, "；")
}

var routeKindLabels = map[routeKind]string{
	routePay:      "下单",
	routeQuery:    "查单",
	routeTransfer: "转账",
}

func (a signingAttempt) label() string {
	source := map[credentialSource]string{
		sourceTokenHash:  "Payment Token 的 SHA-256",
		sourceSecretHash: "Secret Key 的 SHA-256",
		sourceSecretRaw:  "Secret Key 原文",
	}[a.source]
	if a.timestamp {
		return source + "、带时间戳"
	}
	return source + "、不带时间戳"
}

func (g *NodeLocGateway) CreatePayment(ctx context.Context, request contract.CreatePaymentRequest) (*contract.CreatePaymentResult, error) {
	params := map[string]string{
		"amount":      strconv.Itoa(request.Amount),
		"description": request.Description,
		"order_id":    request.OrderID,
	}
	body, raw, err := g.post(ctx, g.payPath(), params, routePay)
	if err != nil {
		return nil, err
	}
	return &contract.CreatePaymentResult{
		TransactionID: firstString(body, transactionIDKeys...),
		PaymentURL:    firstString(body, paymentURLKeys...),
		Status:        normalizeStatus(firstString(body, statusKeys...)),
		Raw:           raw,
	}, nil
}

// QueryPayment asks NodeLoc what it recorded for one payment.
//
// On the real provider 查单 is not a server-to-server endpoint: every request
// shape tried against it — form or JSON, with the payment token, with a bearer
// token, with a CSRF header, with an Origin and Referer — comes back 403
// ["BAD CSRF"] from Discourse's session guard, because the route belongs to the
// merchant's own browser UI. Treating that as fatal strands paid money, so when
// 查单 is guarded or rejects the store's credentials the answer is taken from
// 下单 instead: this order is submitted again and NodeLoc's duplicate refusal
// names the status it already holds. That route is idempotent per order_id, so
// the buyer is never asked to pay twice.
func (g *NodeLocGateway) QueryPayment(ctx context.Context, request contract.QueryPaymentRequest) (*contract.QueryPaymentResult, error) {
	transactionID := strings.TrimSpace(request.TransactionID)
	if transactionID == "" {
		return nil, errors.New("transaction_id is required")
	}
	// The fallback creates nothing unless there is a real order to re-submit, so
	// the settings probe — which passes a transaction id only — can never open a
	// payment on NodeLoc.
	fallbackPossible := request.OrderID != "" && request.Amount > 0

	// reason is what the shop tells the back office about the route it did not use.
	// The answer still comes from NodeLoc either way, but a store owner reading
	// 「查单被拦」 knows to fix a setting instead of waiting for money that is
	// already settled.
	reason := "NodeLoc 的查单接口只认论坛后台的浏览器会话，本单改用下单核实"
	if !g.queryIsGuarded() {
		result, err := g.queryDirect(ctx, transactionID)
		if err == nil {
			result.Via = "query"
			return result, nil
		}
		if !fallbackPossible || !queryFallbackApplies(err) {
			return nil, err
		}
		reason = providerSentence(err)
		log.Printf("payment query %s: NodeLoc 查单无法从服务器端调用（%v），改用下单接口核实这一单", transactionID, err)
	}
	if !fallbackPossible {
		return nil, fmt.Errorf("%w: NodeLoc 的查单接口只接受论坛后台的浏览器会话，商店改用下单和支付回调核实到账（transaction_id=%s）",
			domain.ErrProviderGuarded, transactionID)
	}
	return g.confirmByReprocess(ctx, request, reason)
}

func (g *NodeLocGateway) queryDirect(ctx context.Context, transactionID string) (*contract.QueryPaymentResult, error) {
	params := map[string]string{"transaction_id": transactionID}
	body, raw, err := g.post(ctx, "/payment/query/"+url.PathEscape(g.paymentID), params, routeQuery)
	if err != nil {
		return nil, err
	}
	return &contract.QueryPaymentResult{
		TransactionID: firstNonEmpty(firstString(body, transactionIDKeys...), transactionID),
		// 查单 returns our order number as external_reference; order_id is what we
		// sent when creating the payment, and some releases echo neither.
		OrderID:        firstString(body, orderRefKeys...),
		Amount:         firstInt(body, "amount", "total_amount"),
		Status:         normalizeStatus(firstString(body, statusKeys...)),
		PlatformFee:    optionalInt(body, feeKeys...),
		MerchantPoints: optionalInt(body, merchantKeys...),
		Raw:            raw,
	}, nil
}

// confirmByReprocess is the 核实 route for a provider whose 查单 is browser-only:
// submit this order to 下单 again and read what NodeLoc says about it.
func (g *NodeLocGateway) confirmByReprocess(ctx context.Context, request contract.QueryPaymentRequest, reason string) (*contract.QueryPaymentResult, error) {
	description := strings.TrimSpace(request.Description)
	if description == "" {
		description = "订单 " + request.OrderID
	}
	params := map[string]string{
		"amount":      strconv.Itoa(request.Amount),
		"description": description,
		"order_id":    request.OrderID,
	}
	body, raw, err := g.post(ctx, g.payPath(), params, routePay)
	if err != nil {
		var already *domain.PaymentAlreadyRequested
		if !errors.As(err, &already) {
			return nil, err
		}
		status := normalizeStatus(already.Status)
		if status == "" {
			status = domain.StatusPending
		}
		return &contract.QueryPaymentResult{
			TransactionID: request.TransactionID,
			OrderID:       request.OrderID,
			Amount:        request.Amount,
			Status:        status,
			Via:           "reprocess",
			Fallback:      reason,
			Raw:           []byte(already.Error()),
		}, nil
	}
	// NodeLoc had no payment under this order_id and just made one: the buyer never
	// completed the first, so this is a truthful 「尚未到账」 — and the payment it
	// opened is one they can actually pay.
	amount := firstInt(body, "amount", "total_amount")
	if amount == 0 {
		amount = request.Amount
	}
	status := normalizeStatus(firstString(body, statusKeys...))
	if status == "" {
		status = domain.StatusPending
	}
	return &contract.QueryPaymentResult{
		TransactionID: firstNonEmpty(firstString(body, transactionIDKeys...), request.TransactionID),
		OrderID:       firstNonEmpty(firstString(body, orderRefKeys...), request.OrderID),
		Amount:        amount,
		Status:        status,
		Via:           "reprocess",
		Fallback:      reason,
		Raw:           raw,
	}, nil
}

// queryIsGuarded reports whether the store is currently taking the 下单 route
// instead of 查单. The answer is time-boxed on purpose: Discourse can open that
// route, and a shop that decided once and held the decision for the life of the
// process would never notice, so a paid order would keep being settled through
// the fallback forever. One re-ask per window is the price of self-healing.
func (g *NodeLocGateway) queryIsGuarded() bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.queryGuardedAt.IsZero() {
		return false
	}
	if time.Since(g.queryGuardedAt) > queryGuardHold {
		g.queryGuardedAt = time.Time{}
		return false
	}
	return true
}

func (g *NodeLocGateway) markQueryGuarded() {
	g.mu.Lock()
	first := g.queryGuardedAt.IsZero() || time.Since(g.queryGuardedAt) > queryGuardHold
	g.queryGuardedAt = time.Now()
	g.mu.Unlock()
	if first {
		log.Printf("payment: NodeLoc 把服务器端的查单调用当成浏览器请求拦下了，%v 内改用下单核实", queryGuardHold)
	}
}

// queryFallbackApplies is the short list of 查单 answers after which asking through
// 下单 is worth it. A store that is off the network and a store that has not been
// configured fail there too, and 配置 incomplete must not be papered over by a second
// call.
//
// A 404 on 查单 is on the list even though readResponse calls it a missing
// application: that route answers a live transaction lookup and a missing Payment ID
// with the same bytes, so the order-based question decides it. 下单 is idempotent per
// order_id and only shares the diagnosis when it 404s too — which is when the
// Payment ID really is wrong, and there is no ambiguity left to resolve.
func queryFallbackApplies(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, domain.ErrProviderGuarded) ||
		errors.Is(err, domain.ErrProviderRejected) ||
		errors.Is(err, domain.ErrPaymentAppNotFound) ||
		errors.Is(err, errUnreadableAnswer)
}

// errUnreadableAnswer tags a provider reply that is not JSON. It stays in the
// unreachable family — nothing about the order is known — but 查单 needs to read it
// as "this endpoint did not speak the API" and ask somewhere else.
var errUnreadableAnswer = errors.New("unreadable provider answer")

func (g *NodeLocGateway) Transfer(ctx context.Context, request contract.TransferRequest) (*contract.TransferResult, error) {
	// 转账's documented fields are to_user_id, to_username, points, order_id, timestamp
	// and signature — the amount is named **points**, a number with at most two decimals,
	// and both recipient fields are required. The upstream client this store descends from
	// sent 「amount」 instead, which is why every 退款 came back refused: NodeLoc never saw
	// the field it asks for. points goes out first because the contract names it; a release
	// that still reads the old word is answered by the one retry below.
	params := map[string]string{
		"to_user_id":  request.ToUserID,
		"to_username": request.ToUsername,
		"points":      strconv.Itoa(request.Amount),
		"order_id":    request.OrderID,
	}
	body, raw, err := g.post(ctx, "/payment/transfer/"+url.PathEscape(g.paymentID), params, routeTransfer)
	if err != nil {
		if namesMissingAmountParameter(err) {
			body, raw, err = g.post(ctx, "/payment/transfer/"+url.PathEscape(g.paymentID), legacyTransferParams(request), routeTransfer)
		}
		if err != nil {
			return nil, err
		}
	}
	return &contract.TransferResult{
		TransactionID: firstString(body, transactionIDKeys...),
		Status:        normalizeStatus(firstString(body, statusKeys...)),
		Raw:           raw,
	}, nil
}

func legacyTransferParams(request contract.TransferRequest) map[string]string {
	return map[string]string{
		"to_user_id":  request.ToUserID,
		"to_username": request.ToUsername,
		"amount":      strconv.Itoa(request.Amount),
		"order_id":    request.OrderID,
	}
}

// namesMissingAmountParameter is the one refusal worth re-spelling the money for: code
// 1004 for a field NodeLoc asked for and did not get, 1005 for a value it considers
// invalid, and the matching sentences for a release that numbers nothing. Only a refusal
// about the amount counts — a request that is short a recipient field gets the same number,
// and answering it with a second POST that renames a number would fire at the transfer
// endpoint for an answer that was already complete.
func namesMissingAmountParameter(err error) bool {
	var refused *domain.ProviderRefusal
	if !errors.As(err, &refused) || refused.Signature {
		return false
	}
	lowered := strings.ToLower(refused.Message)
	saidMissing := strings.Contains(lowered, "missing") || strings.Contains(lowered, "invalid param") ||
		strings.Contains(refused.Message, "缺少") || strings.Contains(refused.Message, "参数")
	switch refused.Code {
	case domain.CodeParameterMissing, domain.CodeParameterInvalid:
	case 0:
		return saidMissing && !namesRecipientField(lowered)
	default:
		return false
	}
	// A coded refusal still has to be about the amount: 1004 for a missing to_username is
	// this store's uid-only buyer, and no spelling of the money changes that answer.
	return !namesRecipientField(lowered)
}

// namesRecipientField recognises the recipient fields NodeLoc asks for by name, in both
// the field spelling and the sentence a Chinese forum writes.
func namesRecipientField(lowered string) bool {
	for _, name := range []string{"to_username", "to_user_id", "username", "user_id", "收款", "用户名"} {
		if strings.Contains(lowered, name) {
			return true
		}
	}
	return false
}

// VerifyCallback checks the redirect the buyer's browser comes back with.
//
// NodeLoc's own pages disagree about the key: one section names the merchant
// Secret Key, another says the redirect is HMAC'd with the SHA-256 of the tk_xxx
// token. Both credentials belong to this store's payment application, so a
// redirect that verifies under either one is genuinely NodeLoc's — and trying
// only the first is what sent real, signed payments to the 「signature」 failure
// page while the buyer had already paid.
func (g *NodeLocGateway) VerifyCallback(params map[string]string) bool {
	if g == nil {
		return false
	}
	if key := strings.TrimSpace(g.secretKey); key != "" && shared.VerifyCallback(params, key) {
		return true
	}
	if token := strings.TrimSpace(g.token); token != "" && shared.VerifyCallback(params, shared.HashedTokenKey(token)) {
		return true
	}
	// The upstream client this store descends from signed every call with the
	// digest of the Secret Key; a release that mirrors the callback with the same
	// key verifies here instead of landing the buyer on 「签名不符」.
	if key := strings.TrimSpace(g.secretKey); key != "" && shared.VerifyCallback(params, digestOf(key)) {
		return true
	}
	// A shop with one secret may have pasted it into the Token box, where it is used
	// as the digest above. The same value signed the redirect as issued is the last
	// spelling left, and an unmatched redirect is a paid order showing 待支付.
	if token := strings.TrimSpace(g.token); token != "" && shared.VerifyCallback(params, token) {
		return true
	}
	return false
}

func (g *NodeLocGateway) payPath() string {
	return "/payment/pay/" + url.PathEscape(g.paymentID) + "/process"
}

// attempts builds the signing conventions this store can offer, in the order they
// are worth trying, with the one NodeLoc already accepted for this route moved to
// the front.
func (g *NodeLocGateway) attempts(kind routeKind) []signingAttempt {
	source := map[credentialSource]string{}
	if token := strings.TrimSpace(g.token); token != "" {
		source[sourceTokenHash] = shared.HashedTokenKey(token)
	}
	if secret := strings.TrimSpace(g.secretKey); secret != "" {
		source[sourceSecretRaw] = secret
		source[sourceSecretHash] = digestOf(secret)
	}
	if len(source) == 0 {
		return nil
	}

	type candidate struct {
		source    credentialSource
		timestamp bool
	}
	// The docs sign 下单/转账 with the token digest and 查单 with the Secret Key as
	// issued. Second on each list is the convention this repository's upstream
	// client actually used, because that is the code demonstrably took money with.
	order := []candidate{{sourceTokenHash, true}, {sourceSecretHash, false}, {sourceTokenHash, false}, {sourceSecretRaw, true}, {sourceSecretHash, true}}
	if kind == routeQuery {
		order = []candidate{{sourceSecretRaw, true}, {sourceSecretHash, false}, {sourceSecretRaw, false}, {sourceTokenHash, true}, {sourceSecretHash, true}}
	}

	g.mu.Lock()
	remembered := g.styles[kind]
	g.mu.Unlock()

	attempts := make([]signingAttempt, 0, len(order))
	seen := map[string]bool{}
	for _, item := range order {
		key, ok := source[item.source]
		if !ok {
			continue
		}
		attempt := signingAttempt{source: item.source, key: key, timestamp: item.timestamp}
		if seen[attempt.label()] {
			continue
		}
		seen[attempt.label()] = true
		if attempt.label() == remembered {
			attempts = append([]signingAttempt{attempt}, attempts...)
			continue
		}
		attempts = append(attempts, attempt)
	}
	if len(attempts) > maxSigningAttempts {
		// The remembered winner moved to the front pushes a never-tried convention
		// past the cap; dropping the tail keeps the ladder bounded.
		attempts = attempts[:maxSigningAttempts]
	}
	return attempts
}

func digestOf(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func (g *NodeLocGateway) remember(kind routeKind, attempt signingAttempt) {
	g.mu.Lock()
	known := g.styles[kind] == attempt.label()
	g.styles[kind] = attempt.label()
	g.mu.Unlock()
	if !known {
		log.Printf("payment %s: NodeLoc 接受的签名方式是 %s", routeKindLabels[kind], attempt.label())
	}
}

// post sends one payment call, trying the store's signing conventions until NodeLoc
// accepts one. Only a refusal a different key could plausibly fix is retried: a
// duplicate order, an unknown Payment ID and a browser-session guard say the same
// thing whatever the signature is, and repeating a duplicate 下单 is how a shop
// hammers the endpoint that holds the buyer's money.
func (g *NodeLocGateway) post(ctx context.Context, path string, params map[string]string, kind routeKind) (map[string]any, []byte, error) {
	// Every payment call needs all four settings, even the ones the docs only sign
	// with one of them: a store that can 下单 but not 查单 would take money it can
	// never settle, which is a worse failure than refusing the sale. The message
	// names the missing fields so the shop owner fixes one setting instead of
	// guessing why the storefront says 无法支付.
	if missing := g.Missing(); len(missing) > 0 {
		return nil, nil, fmt.Errorf("%w：缺少 %s", domain.ErrPaymentNotConfigured, strings.Join(missing, "、"))
	}
	attempts := g.attempts(kind)
	if len(attempts) == 0 {
		return nil, nil, fmt.Errorf("%w：未填写 Payment Token（tk_xxx）或 Secret Key", domain.ErrPaymentNotConfigured)
	}

	var lastErr error
	tried := 0
	for _, attempt := range attempts {
		tried++
		body, raw, err := g.postOnce(ctx, path, params, attempt, kind)
		if err == nil {
			g.remember(kind, attempt)
			return body, raw, nil
		}
		lastErr = err
		// Only the session guard is evidence about the route itself. A reply that
		// simply is not JSON may be a proxy page, and the next convention could
		// still get a real answer — 查单 asks 下单 about this one call either way.
		if kind == routeQuery && errors.Is(err, domain.ErrProviderGuarded) {
			g.markQueryGuarded()
		}
		if !retriableRefusal(err, attempt) {
			// NodeLoc understood this request and answered about the order, the
			// balance or the timestamp, so it accepted the key it was signed with.
			// Remembering that is what keeps a single-secret store from paying for
			// the discovery on every call.
			if keyWasAccepted(err) {
				g.remember(kind, attempt)
			}
			break
		}
	}
	if lastErr == nil {
		return nil, nil, fmt.Errorf("%w: NodeLoc 未返回任何结果", domain.ErrProviderUnreachable)
	}
	// Several conventions rejected means the credential itself is what NodeLoc will
	// not accept; the ladder is reported as evidence instead of left for the shop
	// owner to infer from one ambiguous sentence. A duplicate order is excluded by
	// name: it unwraps into this same family, and rewriting it would throw away the
	// one answer that tells the storefront the buyer's payment already exists.
	var already *domain.PaymentAlreadyRequested
	if errors.Is(lastErr, domain.ErrProviderRejected) && !errors.As(lastErr, &already) && tried > 1 {
		labels := make([]string, 0, tried)
		for _, attempt := range attempts[:tried] {
			labels = append(labels, attempt.label())
		}
		// NodeLoc's own sentence leads, kept as the typed refusal so 转账 can still
		// read it. This store does not get to claim 「invalid signature」 on its own
		// reading: a refusal about the balance or the amount still names a fact the
		// shop can act on, and grant.go translates exactly those sentences.
		return nil, nil, fmt.Errorf("%w（依次试了 %d 种签名方式：%s，NodeLoc 均拒绝）",
			lastErr, tried, strings.Join(labels, "；"))
	}
	return nil, nil, lastErr
}

// postOnce performs one attempt. The parameters are copied per attempt because the
// signed string has to be built from exactly the fields this request carries,
// timestamp included or not.
func (g *NodeLocGateway) postOnce(ctx context.Context, path string, in map[string]string, attempt signingAttempt, kind routeKind) (map[string]any, []byte, error) {
	// NodeLoc recomputes the signature from the parameters it actually received,
	// and an empty one may not survive the trip. Dropping it from both the signed
	// string and the body keeps the two sides looking at the same fields — with a
	// recipient bound by NodeLoc uid only, signing "to_username=" would reject
	// the refund as tampered.
	params := make(map[string]string, len(in)+2)
	for name, value := range in {
		if strings.TrimSpace(value) != "" {
			params[name] = value
		}
	}
	// The docs require a 10-digit second timestamp on every payment call and refuse
	// one more than five minutes from their clock. It goes in before signing, so it
	// is part of the sorted string like any other field; a store whose clock has
	// drifted gets its own diagnosis rather than the generic 「凭据不匹配」.
	if attempt.timestamp {
		params["timestamp"] = strconv.FormatInt(time.Now().Unix(), 10)
	}
	params["signature"] = shared.Sign(params, attempt.key)

	values := url.Values{}
	for name, value := range params {
		values.Set(name, value)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+path, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: build NodeLoc request: %v", domain.ErrProviderUnreachable, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", domain.ErrProviderUnreachable, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: read NodeLoc response: %v", domain.ErrProviderUnreachable, err)
	}
	summary := summarize(raw)

	if resp.StatusCode >= 500 {
		return nil, raw, fmt.Errorf("%w: NodeLoc returned HTTP %d: %s", domain.ErrProviderUnreachable, resp.StatusCode, summary)
	}
	// Discourse answers 「["BAD CSRF"]」 as plain text before any payment code runs: the
	// route is registered, it simply belongs to a logged-in browser. Nothing about
	// the order is known and no signing key fixes it, so it is its own family.
	if isSessionGuarded(resp, raw) {
		return nil, raw, fmt.Errorf("%w: %s 返回 HTTP %d: %s", domain.ErrProviderGuarded, path, resp.StatusCode, summary)
	}
	// {"status":404,"error":"Not Found"} is what the payment application answers for
	// a Payment ID it does not have, and the forum's own 404 means this host has no
	// payment routes at all. On 查单 it is diagnostic too, and for a reason the
	// probing established: a live 查单 route answers a server-side call with
	// 「["BAD CSRF"]」 before any transaction lookup runs, so an unknown transaction id
	// can never reach the store as a 404. A 404 there means the route itself is
	// missing — a wrong Payment ID or a wrong 支付 API 地址, which is one field in 设置.
	if resp.StatusCode == http.StatusNotFound {
		return nil, raw, missingApplication(g.baseURL+path, raw)
	}
	// The provider's numbered answer is read before the status code is: NodeLoc puts a
	// refusal in an errors array on both a 4xx and a 200, depending on the release, and
	// the number is what says whether another signing convention can change the answer.
	// Falling through to 「HTTP 400 …」 instead reads to this store as an unexplained
	// rejection, which costs four POSTs at a money endpoint and then blames the shop's
	// credentials for a 每日转账上限 it had already been told about.
	envelope, isJSON := decodeEnvelope(raw)
	if isJSON {
		if refused, ok := documentedRefusal(envelope, g.baseURL+path, raw); ok {
			return nil, raw, refused
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, raw, refusal(fmt.Sprintf("NodeLoc returned HTTP %d: %s", resp.StatusCode, summary))
	}
	if !isJSON {
		if isHTMLPage(raw) {
			// A page over HTTP 200 is a proxy or an error page wearing the API's
			// address, which is not the same fact as Discourse's session guard — that
			// answers 403 with 「["BAD CSRF"]」 above. So this is 「the endpoint did not
			// speak the API」: 查单 asks 下单 about this order, but one odd page does not
			// retire the 查单 route for the next quarter hour.
			return nil, raw, fmt.Errorf("%w: %s 返回的是网页而不是接口应答（HTTP %d）%w",
				domain.ErrProviderUnreachable, g.baseURL+path, resp.StatusCode, errUnreadableAnswer)
		}
		return nil, raw, fmt.Errorf("%w: %w: decode NodeLoc response: %s",
			domain.ErrProviderUnreachable, errUnreadableAnswer, summary)
	}
	if success, ok := envelope["success"].(bool); ok && !success {
		return nil, raw, refusal(firstNonEmpty(firstString(envelope, "message", "error", "detail"), "NodeLoc operation failed"))
	}
	if data, ok := envelope["data"].(map[string]any); ok {
		for key, value := range envelope {
			if _, exists := data[key]; !exists {
				data[key] = value
			}
		}
		return data, raw, nil
	}
	return envelope, raw, nil
}

// isSessionGuarded reads Discourse's answer to a request it thinks came from a browser
// without a session: a CSRF complaint, or a 401/403 page carrying no JSON.
//
// The complaint comes in two shapes and both have been seen from the live 查单 route within
// minutes of each other on the identical request: 「["BAD CSRF"]」 as text/plain, and
// {"errors":[" CSRF Token is missing or invalid "]} as JSON. So the word is what decides,
// not the content type — and a JSON-shaped guard answer must not fall through to the
// signing ladder, which would spend four POSTs and then tell the owner to retype credentials
// that were never the problem.
func isSessionGuarded(resp *http.Response, raw []byte) bool {
	if strings.Contains(strings.ToLower(string(raw)), "csrf") {
		return true
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return !jsonShaped(raw)
	}
	return false
}

// missingApplication names the 404s NodeLoc gives a payment call, because they point at
// two different boxes on the 设置 page. Probed against the live forum on the same request
// shape the store sends (form-encoded body plus Accept: application/json): 下单
// (`/payment/pay/{id}/process`) and 转账 (`/payment/transfer/{id}`) are not CSRF-guarded —
// they look the payment application up first, and an unknown Payment ID comes back from the
// plugin itself as {"status":404,"error":"Not Found"}. A path the forum has never heard of
// is answered by the router instead, with {"errors":[…],"error_type":"not_found"} (or the
// full 404 web page on a browser-shaped request), and that means this host has no payment
// route here at all — a wrong 支付 API 地址, typically the mirror domain a shop logs in
// through rather than the one holding the payment application. Reading either as 「the
// signature was refused」 sends the owner to retype working credentials and keeps the ladder
// firing four POSTs at an endpoint that never had this application, so neither advances the
// ladder and neither is a signing complaint.
func missingApplication(address string, raw []byte) error {
	if noSuchRoute(raw) {
		return fmt.Errorf("%w: %s 返回论坛的 404：这个域上没有支付接口的路由，请核对设置页的「支付 API 地址」", domain.ErrPaymentAppNotFound, address)
	}
	return fmt.Errorf("%w: %s 返回 404：这个 Payment ID 在 NodeLoc 上没有对应的支付应用，请核对设置页的「Payment ID」", domain.ErrPaymentAppNotFound, address)
}

// documentedRefusal reads the error envelope NodeLoc's payment API documents and answers
// with: {"status":400,"errors":[{"code":1001,"detail":"Invalid signature"}]}, where code
// 1006 additionally carries the state of the payment in data.status. The number is read
// before the sentence because the sentence is written in whatever locale the forum runs in
// — and it was this store's English-only reading of that sentence that decided whether to
// fire another POST at a money endpoint. 1003 is the one that no credential and no retry
// can answer (the payment application limits the calling IP), 1002 is this server's clock,
// and 1010/1011/1012/1013 are the transfer limits the shop has to be told about in
// NodeLoc's own words rather than as 「签名不匹配」.
//
// A body that carries no errors array still gets read: some releases put only {"status":
// 404,"error":"Not Found"} where the HTTP status said 200, and that 404 is the Payment ID,
// not a fault in the request.
func documentedRefusal(envelope map[string]any, address string, raw []byte) (error, bool) {
	code, detail := providerError(envelope)
	status, _ := envelope["status"].(float64)
	if code == 0 && status < 400 {
		return nil, false
	}
	if detail == "" {
		detail = firstString(envelope, "detail", "message", "error")
	}
	if code == 0 {
		if int(status) == http.StatusNotFound {
			return missingApplication(address, raw), true
		}
		return refusal(fmt.Sprintf("NodeLoc answered status %d: %s", int(status), detail)), true
	}
	switch code {
	case domain.CodeSignatureFailed:
		return &domain.ProviderRefusal{Message: detail, Signature: true, Code: code}, true
	case domain.CodeTimestampRefused:
		return fmt.Errorf("%w: %s", domain.ErrProviderClockSkew, detail), true
	case domain.CodeIPNotAllowed:
		return fmt.Errorf("%w: %s", domain.ErrProviderIPNotAllowed, detail), true
	case domain.CodeOrderAlreadyExists:
		state := ""
		if data, ok := envelope["data"].(map[string]any); ok {
			state = firstString(data, statusKeys...)
		}
		return &domain.PaymentAlreadyRequested{Status: state, Code: code}, true
	case domain.CodeApplicationNotFound:
		return fmt.Errorf("%w: %s 返回 code %d：%s", domain.ErrPaymentAppNotFound, address, code, detail), true
	}
	return &domain.ProviderRefusal{Message: detail, Code: code}, true
}

// providerError reads the first entry of the documented errors array. The forum's router
// uses the same key for a list of plain strings, which carries no number to act on, so it
// comes back with code 0 and only its text.
func providerError(envelope map[string]any) (int, string) {
	list, ok := envelope["errors"].([]any)
	if !ok || len(list) == 0 {
		return 0, ""
	}
	first, ok := list[0].(map[string]any)
	if !ok {
		if text, ok := list[0].(string); ok {
			return 0, text
		}
		return 0, ""
	}
	return firstInt(first, "code"), firstString(first, "detail", "message", "title")
}

// decodeEnvelope reads a JSON object answer and reports whether the body was one. Discourse's
// guard answers 「["BAD CSRF"]」 which is valid JSON but a list, and no status, errors array
// or data block can be read out of that — it has to stay un-unwrapped so the caller can tell
// 「not an API envelope」 apart from 「an envelope that says no」.
func decodeEnvelope(raw []byte) (map[string]any, bool) {
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, false
	}
	return envelope, true
}

// noSuchRoute tells the forum's router answer apart from the payment plugin's own. The
// router's shape carries errors/error_type and its text is translated with the forum's
// locale, so only the keys are read. The plugin's 404 carries status/error, which is a
// different pair of keys, and a bare empty body belongs with it: that is what the same
// route returns when the request does not ask for JSON.
func noSuchRoute(raw []byte) bool {
	if isHTMLPage(raw) {
		return true
	}
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return false
	}
	_, hasList := envelope["errors"]
	_, hasType := envelope["error_type"]
	return hasList || hasType
}

func isHTMLPage(raw []byte) bool {
	trimmed := strings.ToLower(strings.TrimLeft(string(raw), " \t\r\n"))
	return strings.HasPrefix(trimmed, "<!doctype html") || strings.HasPrefix(trimmed, "<html")
}

func jsonShaped(raw []byte) bool {
	trimmed := strings.TrimLeft(string(raw), " \t\r\n")
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

// retriableRefusal is the ladder's gate: only a refusal a different key could
// plausibly fix gets another convention. Everything else says the same thing
// whatever the signature is, or means NodeLoc never answered — and a call whose
// outcome is unknown must never be repeated against the endpoint that holds the
// buyer's money.
func retriableRefusal(err error, attempt signingAttempt) bool {
	if err == nil {
		return false
	}
	var already *domain.PaymentAlreadyRequested
	if errors.As(err, &already) {
		return false
	}
	// A timestamp complaint from an attempt that sent no timestamp is the ladder
	// working: the next convention puts one in. Once the store has sent a real
	// timestamp and NodeLoc still refuses it, no key changes that answer.
	if errors.Is(err, domain.ErrProviderClockSkew) {
		return !attempt.timestamp
	}
	if errors.Is(err, domain.ErrProviderUnreachable) ||
		errors.Is(err, domain.ErrProviderGuarded) ||
		errors.Is(err, domain.ErrPaymentAppNotFound) ||
		errors.Is(err, domain.ErrPaymentNotConfigured) {
		return false
	}
	var refused *domain.ProviderRefusal
	if errors.As(err, &refused) {
		// A numbered answer needs no translation read: only code 1001 says the key is
		// what NodeLoc could not check, so only 1001 earns another POST. Every other
		// number is a fact about this order, this balance or this limit, and repeating
		// the request four ways is how one refused transfer becomes a mystery in the
		// ledger.
		if refused.Code != 0 {
			return refused.Code == domain.CodeSignatureFailed
		}
		// The provider's sentence either names the signature — in which case the
		// next convention is worth a POST — or it names something about this
		// operation. A 余额不足 does not become a 成功 because the HMAC key came from
		// the other box.
		return refused.Signature || !namesBusinessCondition(refused.Message)
	}
	return errors.Is(err, domain.ErrProviderRejected)
}

// keyWasAccepted reads the one answer the ladder cannot improve on: NodeLoc
// parsed the request and judged the request's content, which it can only do
// after the signature checked out.
func keyWasAccepted(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, domain.ErrProviderClockSkew) {
		return true
	}
	var refused *domain.ProviderRefusal
	if errors.As(err, &refused) {
		return !refused.Signature && namesBusinessCondition(refused.Message)
	}
	return false
}

// signatureComplaints are the sentences NodeLoc uses when the key itself is the
// problem, and the only ones where a different credential can change the answer.
var signatureComplaints = []string{
	"invalid signature", "signature mismatch", "signature does not match",
	"signature error", "wrong signature", "bad signature", "sign failed",
	"invalid key", "invalid credential", "authentication failed",
	"签名", "密钥", "凭据",
}

// businessConditions is NodeLoc's refusal vocabulary about the operation rather
// than the request: its balance, its limits, its transfer switch, its record of
// who is being paid. None of these depends on which settings field the HMAC key
// came from, and each is worth quoting to the shop owner with the numbers the
// provider put in it.
var businessConditions = []string{
	"insufficient balance", "insufficient fund", "not enough balance", "余额不足", "余额不够",
	"transfer feature", "转账功能", "feature is disabled",
	"receiver", "recipient", "收款方", "用户不存在",
	"yourself", "自己",
	"minimum limit", "maximum limit", "below minimum", "exceeds maximum", "限额",
	"invalid amount", "金额",
	"already exists", "已存在", "already paid", "已支付",
	"not open", "未开启", "未启用",
}

func containsAny(lowered string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return false
}

func isSignatureComplaint(message string) bool {
	return containsAny(strings.ToLower(message), signatureComplaints)
}

func namesBusinessCondition(message string) bool {
	lowered := strings.ToLower(message)
	// A sentence about the signature wins over any other word in it: 「signature
	// does not match the receiver application」 is a key problem wearing business
	// clothing, and stopping the ladder there is what makes a store with one
	// mis-pasted credential read as healthy.
	if isSignatureComplaint(lowered) {
		return false
	}
	return containsAny(lowered, businessConditions)
}

// providerSentence takes NodeLoc's own words out of a refusal, so the ladder's
// summary quotes the provider instead of this store's reading of it.
func providerSentence(err error) string {
	var refused *domain.ProviderRefusal
	if errors.As(err, &refused) && strings.TrimSpace(refused.Message) != "" {
		return refused.Message
	}
	var already *domain.PaymentAlreadyRequested
	if errors.As(err, &already) {
		return already.Error()
	}
	return err.Error()
}

// refusal reads a NodeLoc rejection. 「Order already exists with status …」 is the
// one that is not a fault: the payment already sits on NodeLoc's side under our
// order number, which is what happens every time a buyer returns to 立即购买 on an
// order they started before. Refusing it as a generic rejection is how a store
// ends up with orders nobody can ever pay.
func refusal(message string) error {
	if status, ok := alreadyRequestedStatus(message); ok {
		return &domain.PaymentAlreadyRequested{Status: status}
	}
	if isClockRefusal(message) {
		return fmt.Errorf("%w: %s", domain.ErrProviderClockSkew, message)
	}
	return &domain.ProviderRefusal{Message: message, Signature: isSignatureComplaint(message)}
}

// isClockRefusal names the one refusal family this store's own clock causes. Every
// payment call NodeLoc accepts carries a 10-digit second timestamp and it refuses
// one outside its window, so a message about a timestamp can only mean the server
// this shop runs on has drifted — the credentials are not in question.
func isClockRefusal(message string) bool {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "timestamp"), strings.Contains(message, "时间戳"):
		return true
	case strings.Contains(lower, "request expired"), strings.Contains(lower, "request has expired"):
		return true
	case strings.Contains(message, "请求已过期"), strings.Contains(message, "请求过期"):
		return true
	}
	return false
}

// duplicateOrderSubjects and duplicateOrderVerdicts together recognise NodeLoc's
// duplicate-下单 answer. The 核实 route is built on reading it: when 查单 is browser-only,
// the store re-submits the order_id and takes the payment's state out of 「this order is
// already here」. A sentence this function fails to read costs the buyer a 「暂时无法向
// NodeLoc 确认支付结果」 about money that has long settled, and sends the signing ladder
// firing four POSTs at a route that was answering politely. Both halves are required
// because the verdict alone is not the same fact on every route — a 转账 refusal about a
// duplicate record stays the business refusal the shop already reads correctly.
//
// The pair the store used to match, 「order already exists」 and 「订单已存在」, is one
// phrasing of one English release; the forum answers in whatever locale the site runs in,
// and NodeLoc is a Chinese-language site.
var duplicateOrderSubjects = []string{"order", "订单", "支付单", "out_trade_no", "external_reference"}

var duplicateOrderVerdicts = []string{
	"already exists", "already exist", "already paid", "already created", "already submitted", "is paid",
	"已存在", "已经存在", "已被创建", "已支付", "已经支付", "已创建", "重复",
}

func alreadyRequestedStatus(message string) (string, bool) {
	lower := strings.ToLower(message)
	if !containsAny(lower, duplicateOrderSubjects) {
		return "", false
	}
	verdict := firstMarker(lower, duplicateOrderVerdicts)
	if verdict == "" {
		return "", false
	}
	if status := statusAfterVerdict(lower, verdict); status != "" {
		return status, true
	}
	// 「订单已支付」 carries its state in the verdict itself, so a sentence with nothing
	// after it is still an answer about the payment, not an empty one.
	switch verdict {
	case "already paid", "is paid", "已支付", "已经支付":
		return "paid", true
	}
	return "", true
}

// firstMarker returns the verdict that appears earliest in the sentence, so the status word
// is read from the clause that made this a duplicate-order answer.
func firstMarker(lowered string, markers []string) string {
	found, at := "", -1
	for _, marker := range markers {
		index := strings.Index(lowered, marker)
		if index >= 0 && (at < 0 || index < at) {
			found, at = marker, index
		}
	}
	if at < 0 {
		return ""
	}
	return found
}

// statusAfterVerdict returns the state word NodeLoc names after its duplicate-order
// verdict, in either language: 「… already exists with status paid」 and 「…，状态为 paid」
// both have to come out as paid, because that word is what settles the order.
func statusAfterVerdict(lowered, verdict string) string {
	tail := lowered[strings.Index(lowered, verdict)+len(verdict):]
	for _, filler := range []string{"with", "the", "status", "，", ",", "。", "；", ";", "状态为", "状态", "：", ":", " "} {
		for {
			trimmed := strings.TrimPrefix(strings.TrimSpace(tail), filler)
			if trimmed == strings.TrimSpace(tail) {
				break
			}
			tail = trimmed
		}
	}
	if tail == "" {
		return ""
	}
	return strings.Trim(tail, " ：:。.，,\"'；;")
}

// summarize keeps a provider body short enough to log and to show a shop owner,
// and strips the whitespace that makes a raw dump unreadable in a toast.
func summarize(body []byte) string {
	text := strings.Join(strings.Fields(string(body)), " ")
	const limit = 240
	if len(text) > limit {
		return text[:limit] + "…"
	}
	if text == "" {
		return "empty response"
	}
	return text
}

// The NodeLoc payment routes answer with the same facts under more than one name: 下单 puts
// the cashier address in data.payment_url, 查单 names the payment's own number
// transaction_id, and the browser redirect calls this store's order external_reference
// while 下单 was given order_id. A 下单 whose cashier URL the store failed to pick up is a
// buyer staring at 「无法支付」 while NodeLoc already holds the payment open, so every read
// takes the first spelling that is present rather than betting on one. `payment_id` is
// deliberately not an alias for the transaction id: that is the payment *application*'s own
// number, and mistaking it for a transaction makes every 核实 ask NodeLoc about the wrong
// record.
var (
	transactionIDKeys = []string{"transaction_id", "trade_no", "id"}
	paymentURLKeys    = []string{"payment_url", "checkout_url", "pay_url", "redirect_url", "url"}
	statusKeys        = []string{"status", "state", "payment_status"}
	orderRefKeys      = []string{"external_reference", "order_id", "order_no"}
	feeKeys           = []string{"platform_fee", "fee_amount", "fee"}
	merchantKeys      = []string{"merchant_points", "merchant_amount", "net_amount"}
)

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := values[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			if typed != "" {
				return typed
			}
		case json.Number:
			return typed.String()
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64)
		}
	}
	return ""
}

// firstInt reads an amount NodeLoc reported for this payment. The provider writes money as
// a decimal string in some places ("amount":"100.00", "merchant_amount":90.5) and as a
// number in others, so a value that is present but unreadable must not be reported as
// absent — this store compares the number it reads against the order it holds, and a
// silently missing amount is a payment it accepts without checking.
func firstInt(values map[string]any, keys ...string) int {
	for _, key := range keys {
		if points, ok := pointsValue(values[key]); ok {
			return points
		}
	}
	return 0
}

// optionalInt keeps the difference between 「服务商没有报这个字段」 and 「报了 0」: the
// first leaves the store's own ledger alone and the second corrects it.
func optionalInt(values map[string]any, keys ...string) *int {
	for _, key := range keys {
		if points, ok := pointsValue(values[key]); ok {
			return &points
		}
	}
	return nil
}

// pointsValue converts one provider money field. Rounding rather than truncating matters
// for a fee reported as 9.999: dropping the fraction is a different settlement than the one
// the provider booked.
func pointsValue(value any) (int, bool) {
	switch typed := value.(type) {
	case nil:
		return 0, false
	case float64:
		return int(math.Round(typed)), true
	case int:
		return typed, true
	case json.Number:
		return pointsOf(typed.String())
	case string:
		return pointsOf(typed)
	default:
		return 0, false
	}
}

func pointsOf(text string) (int, bool) {
	trimmed := strings.TrimSpace(strings.ReplaceAll(text, ",", ""))
	if trimmed == "" {
		return 0, false
	}
	if whole, err := strconv.Atoi(trimmed); err == nil {
		return whole, true
	}
	decimal, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, false
	}
	return int(math.Round(decimal)), true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// normalizeStatus folds whatever word the provider uses for this payment into the store's
// own vocabulary. It is read on 下单 replies, 查单 replies and the status word inside
// NodeLoc's duplicate-order sentence, so the Chinese spellings are not decoration: NodeLoc
// is a Chinese-language forum, and a 「已支付」 left untranslated is a payment this store
// calls pending forever — a buyer who has paid, staring at a page that says they have not.
func normalizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "paid", "success", "succeeded", "completed", "complete",
		"已支付", "已到账", "支付成功", "已完成", "成功":
		return "succeeded"
	case "failed", "failure", "error", "cancelled", "canceled", "expired", "closed",
		"已失败", "失败", "已取消", "取消", "已过期", "过期", "已关闭", "关闭":
		return "failed"
	case "refunded", "已退款", "退款", "退款成功":
		return "refunded"
	case "processing", "pending", "created", "unpaid", "awaiting",
		"待支付", "未支付", "待付款", "处理中", "进行中", "待处理", "已创建", "未到账", "":
		return "pending"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}
