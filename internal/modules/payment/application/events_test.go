package application

import (
	"context"
	"strings"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/contract"
)

func TestDeliveryEventNamesWhatFulfillmentBecame(t *testing.T) {
	cases := []struct {
		fulfillment string
		want        string
	}{
		{"delivered", eventDelivered},
		{"completed", eventDelivered},
		{"manual_pending", eventManualPending},
		{"waiting_stock", eventWaitingStock},
		// A payment NodeLoc confirmed whose delivery is still queued must not read
		// as if the goods arrived.
		{"pending", eventPaymentHeld},
		{"", eventPaymentHeld},
	}
	for _, tc := range cases {
		order := &models.Order{FulfillmentStatus: tc.fulfillment}
		if got := deliveryEvent(order); got != tc.want {
			t.Errorf("fulfillment %q: got %q, want %q", tc.fulfillment, got, tc.want)
		}
	}
	if deliveryEvent(nil) != "" {
		t.Error("an order that never loaded must not produce an event")
	}
}

func TestBuyerEventPointsAtTheBuyersOwnOrder(t *testing.T) {
	order := &models.Order{
		UserID:      7,
		OrderNo:     "NL20260929070000",
		Quantity:    2,
		TotalAmount: 180,
		Product:     &models.Product{Name: " VPN 月卡 "},
	}
	event, ok := buyerEvent(order, eventDelivered)
	if !ok {
		t.Fatal("a delivered order belongs to a buyer, so it must produce an event")
	}
	if event.UserID != 7 || event.Type != "order" {
		t.Errorf("event addressed wrong: %+v", event)
	}
	if event.Link != "/orders/"+order.OrderNo {
		t.Errorf("link %q does not open the buyer's own order page", event.Link)
	}
	if !strings.Contains(event.Title, "交付") {
		t.Errorf("title %q does not say the goods arrived", event.Title)
	}
	// The copy has to identify the order and the goods among a buyer's other
	// messages, and the product name is stored with padding around it.
	if !strings.Contains(event.Content, order.OrderNo) || !strings.Contains(event.Content, "VPN 月卡") {
		t.Errorf("content %q names neither the order nor the trimmed product", event.Content)
	}
	if strings.Contains(event.Content, "  VPN") {
		t.Errorf("product name was not trimmed: %q", event.Content)
	}

	refunded, ok := buyerEvent(order, eventRefunded)
	if !ok || !strings.Contains(refunded.Content, "180 NL") {
		t.Errorf("a refund must say what came back, got %+v ok=%v", refunded, ok)
	}
}

func TestBuyerEventRefusesOrdersNobodyOwns(t *testing.T) {
	// A row without a buyer or without a number cannot be linked or delivered to
	// anyone, so it must be dropped rather than written as an unreadable message.
	for _, order := range []*models.Order{
		nil,
		{OrderNo: "NL1"},
		{UserID: 3},
	} {
		if _, ok := buyerEvent(order, eventDelivered); ok {
			t.Errorf("order %+v must not produce an event", order)
		}
	}
	if _, ok := buyerEvent(&models.Order{UserID: 3, OrderNo: "NL1"}, "unknown_kind"); ok {
		t.Error("an event nobody described must not be sent")
	}
}

// recordedNotifier is the inbox as far as the payment module is concerned.
type recordedNotifier struct {
	events []contract.BuyerEvent
}

func (n *recordedNotifier) Publish(_ context.Context, event contract.BuyerEvent) {
	n.events = append(n.events, event)
}

func TestNotifyOrderIsSilentWithoutAnInbox(t *testing.T) {
	// A store wired with no inbox still has to take money: the notification path
	// must not be able to panic the settlement that calls it.
	svc := &Service{}
	svc.notifyOrder(context.Background(), &models.Order{UserID: 1, OrderNo: "NL1"}, eventDelivered)

	events := &recordedNotifier{}
	svc = &Service{events: events}
	svc.notifyOrder(context.Background(), &models.Order{UserID: 1, OrderNo: "NL1"}, eventDelivered)
	if len(events.events) != 1 {
		t.Fatalf("got %d events, want the delivered order reported once", len(events.events))
	}
	// An order that cannot be described reaches nobody, and must not reach the inbox half-written.
	svc.notifyOrder(context.Background(), nil, eventDelivered)
	svc.notifyOrder(context.Background(), &models.Order{UserID: 1, OrderNo: "NL1"}, "")
	if len(events.events) != 1 {
		t.Fatalf("got %d events, want nothing extra from orders nobody owns", len(events.events))
	}
}
