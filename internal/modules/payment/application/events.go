package application

import (
	"context"
	"strconv"
	"strings"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/contract"
)

// The order moments a buyer gets told about. A paid order is not one message:
// what happens next differs completely between goods already delivered, a shop
// that still has to ship by hand, and stock that has to be refilled first.
const (
	eventDelivered     = "delivered"
	eventManualPending = "manual_pending"
	eventWaitingStock  = "waiting_stock"
	// eventPaymentHeld is the money confirmed while delivery is still queued for
	// the background retry — the one state that used to leave a buyer staring at
	// a paid order with no explanation of the wait.
	eventPaymentHeld = "payment_held"
	eventShipped     = "shipped"
	eventRefunded    = "refunded"
)

// notifyOrder hands one order event to the buyer's inbox. Every call site sits
// after the order row has already been written, so a message can only repeat what
// the order page says — never contradict it, and never delay it.
func (s *Service) notifyOrder(ctx context.Context, order *models.Order, kind string) {
	if s.events == nil {
		return
	}
	event, ok := buyerEvent(order, kind)
	if !ok {
		return
	}
	s.events.Publish(ctx, event)
}

// deliveryEvent reads what fulfillment actually became after a settled payment.
func deliveryEvent(order *models.Order) string {
	if order == nil {
		return ""
	}
	switch order.FulfillmentStatus {
	case "delivered", "completed":
		return eventDelivered
	case "manual_pending":
		return eventManualPending
	case "waiting_stock":
		return eventWaitingStock
	default:
		return eventPaymentHeld
	}
}

// buyerEvent writes the inbox entry for one order event. The copy names the order
// and the goods, because a buyer who gets several messages has to be able to tell
// which order each one is about.
func buyerEvent(order *models.Order, kind string) (contract.BuyerEvent, bool) {
	if order == nil || order.UserID == 0 || order.OrderNo == "" {
		return contract.BuyerEvent{}, false
	}
	name := "商品"
	if order.Product != nil {
		if trimmed := strings.TrimSpace(order.Product.Name); trimmed != "" {
			name = trimmed
		}
	}
	event := contract.BuyerEvent{UserID: order.UserID, Type: "order", Link: "/orders/" + order.OrderNo}
	switch kind {
	case eventDelivered:
		event.Title = "商品已交付"
		event.Content = "订单 " + order.OrderNo + " 支付完成，" + name + " × " +
			strconv.Itoa(order.Quantity) + " 已经放进你的订单，卡密随时可以查看。"
	case eventManualPending:
		event.Title = "支付已确认，等待商家发货"
		event.Content = "订单 " + order.OrderNo + " 已收到款项。这件商品由商家手动交付，发货后会再通知你一次，不必重复付款。"
	case eventWaitingStock:
		event.Title = "支付已确认，等待补货"
		event.Content = "订单 " + order.OrderNo + " 已收到款项，但现货暂时不足。补货到位后商店会自动交付，不必重复付款。"
	case eventPaymentHeld:
		event.Title = "支付已确认"
		event.Content = "订单 " + order.OrderNo + " 的付款已确认，" + name + " 正在处理，通常几分钟内送达。"
	case eventShipped:
		event.Title = "商家已发货"
		event.Content = "订单 " + order.OrderNo + " 的 " + name + " 已由商家交付，内容可以在订单页查看。"
	case eventRefunded:
		event.Title = "退款已到账"
		event.Content = "订单 " + order.OrderNo + " 的 ¥" + strconv.Itoa(order.TotalAmount) +
			" 已退回你的 NodeLoc 账户。"
	default:
		return contract.BuyerEvent{}, false
	}
	return event, true
}
