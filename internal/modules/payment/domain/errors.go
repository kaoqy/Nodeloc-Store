package domain

import "errors"

// Sentinels shared by the payment module's layers. Infrastructure returns them
// and application/transport classify them without importing concrete stores.
var (
	ErrOrderNotFound         = errors.New("order not found")
	ErrPaymentOrderNotFound  = errors.New("payment order not found")
	ErrInsufficientStock     = errors.New("insufficient card stock")
	ErrProductNotPurchasable = errors.New("product is not purchasable")
	ErrNotPayable            = errors.New("order is not paid")
	// ErrPaymentNotConfigured is returned when the Payment ID / Secret Key pair
	// is missing, so the buyer gets a clear message instead of a generic 500.
	ErrPaymentNotConfigured = errors.New("商店还没有配置 NodeLoc 支付凭据")
	// ErrPaymentsDisabled is the owner's 「启用 NodeLoc Payments 支付」 switch turned
	// off: the credentials are fine, the shop just is not taking money right now.
	// Refunds keep working, so only 下单 and 去支付 check it.
	ErrPaymentsDisabled = errors.New("商店已暂停收款")
	// ErrProviderUnreachable means NodeLoc was never able to answer: DNS, TLS,
	// timeout, an HTTP 5xx or a body that is not JSON. Nothing about the order is
	// known, so the caller is told to try again.
	ErrProviderUnreachable = errors.New("联系不上 NodeLoc 支付服务")
	// ErrProviderRejected means NodeLoc answered and refused the call itself,
	// typically because the credentials or signature do not match the payment
	// application. Retrying cannot help; the shop owner has to fix the settings.
	ErrProviderRejected = errors.New("NodeLoc 拒绝了商店的支付请求")
	// ErrPaymentAlreadyRequested is the reading of NodeLoc's 「Order already
	// exists with status …」 answer to a second 下单 for an order_id this store
	// already sent. The payment exists on their side, so the storefront must hand
	// the buyer that payment rather than refuse them forever.
	ErrPaymentAlreadyRequested = errors.New("NodeLoc 上已经有这一单的支付记录")
	// ErrPaymentAppNotFound is the payment interface answering a call it could not
	// place. Probed against the live forum with the store's own request shape: an
	// unknown Payment ID is the plugin's 404 ({"status":404,"error":"Not Found"}),
	// while a path the forum has never heard of is the router's 404
	// ({"errors":[…],"error_type":"not_found"}, or its 404 web page on a
	// browser-shaped request) — this host has no payment routes at all, so
	// 支付 API 地址 points at the wrong domain. The host answered either way, and no
	// signing key changes what it said, so this is kept apart from
	// ErrProviderRejected: one is a settings field to fix, the other is a
	// credential to replace.
	ErrPaymentAppNotFound = errors.New("这个 Payment ID 在 NodeLoc 上没有对应的支付应用")
	// ErrProviderGuarded means NodeLoc treated the call as a browser request and
	// stopped it before the payment code ran: a CSRF complaint on 查单 (the forum
	// answers both 「["BAD CSRF"]」 and a JSON 「CSRF Token is missing or invalid」),
	// or a 401/403 that carries no JSON at all. The endpoint exists and is
	// reachable; it is simply not callable from a server, which is a different fact
	// from 「连不上」 and from 「签名不对」, and it is the reason 查单 must fall back to
	// 下单. A page served over HTTP 200 is not this — that one says the endpoint did
	// not speak the API.
	ErrProviderGuarded = errors.New("NodeLoc 把这个接口挡在论坛的浏览器会话后面")
	// ErrProviderClockSkew means NodeLoc accepted the credentials and refused the
	// request's timestamp: this server's clock is more than the provider's window
	// away from its own. Every payment call carries that field, so a drifted clock
	// kills 下单/查单/转账 at once while nothing in 设置 is wrong — which is why it
	// gets its own family instead of being reported as a bad key.
	ErrProviderClockSkew = errors.New("NodeLoc 拒绝了商店请求里的时间戳")
	// ErrProviderIPNotAllowed is NodeLoc's code 1003: the payment application only
	// answers calls from the IP addresses its owner listed on the forum. The store's
	// credentials can be perfectly right and every route still refuse, because the
	// check runs on NodeLoc's side before any of them is read. Only the 白名单 on
	// the payment application fixes it, so this is neither 连不上 nor 签名不对, and
	// it is the one family where retrying from a new IP (a restarted container on a
	// different host) changes the answer while nothing in 设置 does.
	ErrProviderIPNotAllowed = errors.New("NodeLoc 的支付应用不允许这台服务器的 IP 调用")
)

// NodeLoc's payment API answers a refusal with a number as well as a sentence:
// {"status":400,"errors":[{"code":1001,"detail":"Invalid signature"}]}. The number is
// what the store reads, because the detail follows the forum's locale and a Chinese
// 「验签失败」 vs 「余额不足」 decided whether this store fired another POST at a money
// endpoint. Codes this store acts on:
//
//	1001 验签失败          the key — try the next signing convention
//	1002 时间戳已过期或无效  this server's clock, not the credentials
//	1003 IP 不在白名单      nothing on this machine can fix; the payment app must allow it
//	1004/1005 参数缺失/无效  the request, not the key
//	1006 订单已存在         carries the payment's state in data.status
//	1007/1008/1009 各类不存在
//	1010 余额不足  1011 金额低于最小值  1012 不能转给自己  1013 达到每日上限
const (
	CodeSignatureFailed     = 1001
	CodeTimestampRefused    = 1002
	CodeIPNotAllowed        = 1003
	CodeParameterMissing    = 1004
	CodeParameterInvalid    = 1005
	CodeOrderAlreadyExists  = 1006
	CodeOrderNotFound       = 1007
	CodePaymentNotFound     = 1008
	CodeApplicationNotFound = 1009
	CodeInsufficientBalance = 1010
	CodeBelowMinimum        = 1011
	CodeTransferToYourself  = 1012
	CodeDailyLimitReached   = 1013
)

// codesProvingTheKeyWasRead are the refusals NodeLoc can only reach after it accepted the
// signature and looked at this particular order, balance or limit. Remembering the signing
// style on those is what keeps a working store from paying for the discovery ladder on
// every call; a 1003 (rejected before the key was ever read) or a 1004 (rejected while the
// request was still being parsed) proves nothing and remembers nothing.
var CodesProvingTheKeyWasRead = map[int]bool{
	CodeOrderAlreadyExists:  true,
	CodeInsufficientBalance: true,
	CodeBelowMinimum:        true,
	CodeTransferToYourself:  true,
	CodeDailyLimitReached:   true,
}

// PaymentAlreadyRequested is the typed refusal behind ErrPaymentAlreadyRequested.
// It carries the status word NodeLoc named for the payment it already holds — in its
// documented shape that is data.status next to code 1006, not a word inside the
// sentence — because 「this order is mid-payment」 and 「this order is already paid」 are
// two different next steps.
type PaymentAlreadyRequested struct {
	Status string
	Code   int
}

func (e *PaymentAlreadyRequested) Error() string {
	if e.Status == "" {
		return ErrPaymentAlreadyRequested.Error()
	}
	return ErrPaymentAlreadyRequested.Error() + ": " + e.Status
}

// Unwrap keeps the refusal inside the provider-rejection family: a store that
// cannot resume the existing payment still reports it as NodeLoc refusing the
// call, which is what the buyer-facing copy and the settings probe already read.
func (e *PaymentAlreadyRequested) Unwrap() error { return ErrProviderRejected }

func (e *PaymentAlreadyRequested) Is(target error) bool {
	return target == ErrPaymentAlreadyRequested
}

// ProviderRefusal is NodeLoc's own sentence about a call it parsed and judged.
// Signature records whether that sentence is about how the store signed the
// request, which is the whole difference between 「try another key」 and 「the
// shop's balance is short」: only the first can be answered by a different
// credential, and a 转账 refusal about the balance is worth its exact numbers.
//
// Code is the same fact read from the number NodeLoc sends beside the sentence
// ({"errors":[{"code":1001,"detail":"…"}]}), which is why it is preferred wherever it
// is present: the detail comes back in the forum's own locale, and an English-only
// reading of it is what made a store keep POSTing at a money endpoint over a 每日转账上限
// the provider had already explained. Zero means this release gave no number, and the
// sentence decides.
type ProviderRefusal struct {
	Message   string
	Signature bool
	Code      int
}

func (e *ProviderRefusal) Error() string {
	if e.Message == "" {
		return ErrProviderRejected.Error()
	}
	return ErrProviderRejected.Error() + ": " + e.Message
}

// Unwrap keeps a refusal inside the provider-rejection family: the buyer-facing
// copy, the 设置 probe and 转账's translations all read it as NodeLoc having
// answered and said no, whatever it said no about.
func (e *ProviderRefusal) Unwrap() error { return ErrProviderRejected }
