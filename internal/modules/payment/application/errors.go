package application

import (
	"errors"
	"net/http"
	"strings"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/domain"
)

// Failure is the machine-readable reading of a payment error. The transport
// turns it into an HTTP status plus copy a buyer can act on, the storefronts
// switch on Code, and only the back office is ever handed Detail — NodeLoc's
// own words, which name credentials and internal state a buyer cannot use.
type Failure struct {
	Code      string `json:"code"`
	Status    int    `json:"-"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	Detail    string `json:"detail,omitempty"`
}

// couponRefusal is the shape the catalogue's coupon errors happen to have.
// Declaring it here instead of importing that package keeps the modules apart:
// payment only needs the reason a code was refused, whatever produced it.
type couponRefusal interface {
	error
	CouponCode() string
	CouponMessage() string
}

// Classify maps a payment error to its Failure. Anything unrecognised stays a
// 500 with a generic buyer message: a Go error string is written for the log,
// not for a storefront.
func Classify(err error) *Failure {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, ErrInvalidInput):
		return &Failure{Code: "invalid_input", Status: http.StatusBadRequest, Message: "订单信息有误，请检查后重新提交。"}
	case errors.Is(err, ErrForbidden):
		return &Failure{Code: "forbidden", Status: http.StatusForbidden, Message: "这个订单不属于当前账号。"}
	case errors.Is(err, ErrAmountMismatch):
		return &Failure{Code: "amount_mismatch", Status: http.StatusBadRequest, Message: "NodeLoc 记录的金额与本单不一致，商店已暂停自动入账，请联系店家核对。", Detail: err.Error()}
	case errors.Is(err, ErrForeignTransaction):
		return &Failure{Code: "foreign_transaction", Status: http.StatusConflict, Message: "NodeLoc 把这笔交易归属到了其他订单，商店已暂停自动入账，请联系店家核实。", Detail: err.Error()}
	case errors.Is(err, ErrInvalidCallback):
		return &Failure{Code: "callback_invalid", Status: http.StatusUnprocessableEntity, Message: "支付回调未通过签名校验，商店已向 NodeLoc 核实这单。", Detail: err.Error()}
	case errors.Is(err, ErrNoProviderTransaction):
		return &Failure{Code: "no_transaction", Status: http.StatusConflict, Message: "商店还没有拿到这单在 NodeLoc 的交易号，无法代为核实。请点「继续支付」重新发起，或联系店家。"}
	case errors.Is(err, ErrPaymentUnsettled):
		return &Failure{Code: "unsettled", Status: http.StatusConflict, Message: "NodeLoc 还没有这单的到账记录。确认已扣款请稍候再查，仍未到账请联系店家。", Retryable: true, Detail: err.Error()}
	case errors.Is(err, ErrPaymentNotComplete):
		return &Failure{Code: "not_complete", Status: http.StatusConflict, Message: "NodeLoc 回报这笔支付尚未完成。", Retryable: true, Detail: err.Error()}
	case errors.Is(err, ErrRefundRecipientUnknown):
		return &Failure{Code: "refund_recipient_unknown", Status: http.StatusConflict, Message: err.Error()}
	case errors.Is(err, ErrCouponUnavailable):
		// Checkout and the quote box run the same coupon rules, so the refusal the
		// catalogue already worded for the buyer is reused verbatim instead of a
		// second, vaguer sentence written here. A code that carries no reason (the
		// pricing port is not wired at all) keeps the catch-all wording.
		var refusal couponRefusal
		if errors.As(err, &refusal) {
			return &Failure{Code: refusal.CouponCode(), Status: http.StatusUnprocessableEntity, Message: refusal.CouponMessage()}
		}
		return &Failure{Code: "coupon_unavailable", Status: http.StatusUnprocessableEntity, Message: "优惠码已经用不了了（可能过期、额度用满或不适用于本单），请重新确认后再下单。"}
	case errors.Is(err, domain.ErrPaymentNotConfigured):
		return &Failure{Code: "not_configured", Status: http.StatusServiceUnavailable, Message: "商店的 NodeLoc 支付还没有配置好，请稍后再试或联系店家。", Detail: err.Error()}
	case errors.Is(err, domain.ErrProviderUnreachable):
		return &Failure{Code: "provider_unreachable", Status: http.StatusBadGateway, Message: "暂时联系不上 NodeLoc 的支付服务，稍后可以再查一次。", Retryable: true, Detail: err.Error()}
	case errors.Is(err, domain.ErrProviderRejected):
		return &Failure{Code: "provider_rejected", Status: http.StatusBadGateway, Message: "NodeLoc 拒绝了商店的支付请求，通常是后台支付凭据不匹配，请联系店家处理。", Detail: err.Error()}
	case errors.Is(err, domain.ErrOrderNotFound), errors.Is(err, domain.ErrPaymentOrderNotFound):
		return &Failure{Code: "not_found", Status: http.StatusNotFound, Message: "没有找到这个订单。", Detail: err.Error()}
	case errors.Is(err, domain.ErrProductNotPurchasable):
		return &Failure{Code: "product_unavailable", Status: http.StatusNotFound, Message: "该商品已下架或暂不可购。"}
	case errors.Is(err, domain.ErrInsufficientStock):
		return &Failure{Code: "insufficient_stock", Status: http.StatusConflict, Message: "卡密库存不足，请稍后再试或联系店家补货。"}
	case errors.Is(err, domain.ErrNotPayable):
		return &Failure{Code: "not_payable", Status: http.StatusConflict, Message: "本单当前不可支付或已交付。", Detail: err.Error()}
	case strings.Contains(strings.ToLower(err.Error()), "not found"):
		return &Failure{Code: "not_found", Status: http.StatusNotFound, Message: "没有找到对应的记录。", Detail: err.Error()}
	default:
		return &Failure{Code: "internal", Status: http.StatusInternalServerError, Message: "操作未能完成，请稍后重试或联系店家。", Detail: err.Error()}
	}
}
