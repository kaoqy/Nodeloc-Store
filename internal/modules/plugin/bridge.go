package plugin

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	paymentcontract "github.com/kaoqy/Nodeloc-Store/internal/modules/payment/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/application"
)

// DeliveryBridge is the adapter between the money module and the plugin module.
//
// The two modules only know each other's contract packages, so neither imports
// the other's internals: payment hands an order to this bridge, which translates
// it into the plugin module's own terms and translates the answer back.
type DeliveryBridge struct {
	service *application.Service
}

func NewDeliveryBridge(service *application.Service) *DeliveryBridge {
	return &DeliveryBridge{service: service}
}

// SyncProductChannel is the catalogue's hook into this module: it keeps the
// plugin binding that routes delivery in step with the product's chosen channel.
func (b *DeliveryBridge) SyncProductChannel(ctx context.Context, productID uint, channel string) error {
	return b.service.SyncProductChannel(ctx, productID, channel)
}

// CheckProductChannel lets the catalogue refuse a channel that cannot deliver
// yet (for example New-API without complete credentials) before saving.
func (b *DeliveryBridge) CheckProductChannel(ctx context.Context, channel string) error {
	return b.service.CheckProductChannel(ctx, channel)
}

// Owns is what tells payment to route an order through its plugin instead of the
// shop's own card/manual queue.
func (b *DeliveryBridge) Owns(ctx context.Context, order *models.Order) (bool, error) {
	if order == nil || order.ProductID == 0 {
		return false, nil
	}
	return b.service.Owns(ctx, order.ProductID)
}

// Fulfill resolves the order's plugin binding and delivers it. A nil result with
// no error means the binding disappeared between Owns and here, and the caller
// falls back to the shop's own fulfilment.
func (b *DeliveryBridge) Fulfill(ctx context.Context, order *models.Order) (*paymentcontract.PluginDelivery, error) {
	if order == nil || order.ID == 0 {
		return nil, nil
	}
	result, err := b.service.Deliver(ctx, application.OrderInfo{
		ID:          order.ID,
		OrderNo:     order.OrderNo,
		UserID:      order.UserID,
		ProductID:   order.ProductID,
		Quantity:    order.Quantity,
		TopupAmount: order.TopupAmount,
		UnitPrice:   order.UnitPrice,
		TotalPrice:  order.TotalAmount,
		Contact:     deref(order.CustomerContact),
		Note:        deref(order.CustomerNote),
		FormValues:  decodeFormValues(order.FormValues),
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return &paymentcontract.PluginDelivery{
		Content:   result.Content,
		Note:      result.Note,
		Reference: result.Reference,
		Uncertain: result.Uncertain,
	}, nil
}

// ValidateSelection is the checkout-time gate. It runs before the order is
// written, so a buyer never pays for a selection that has no delivery item.
func (b *DeliveryBridge) ValidateSelection(ctx context.Context, productID uint, formValues map[string]string) error {
	return b.service.ValidateSelection(ctx, productID, formValues)
}

// FormFields adapts the provider's buyer-facing form schema to the shared
// ProductFormField shape used by the order validator.
func (b *DeliveryBridge) FormFields(ctx context.Context, productID uint) ([]models.ProductFormField, error) {
	describe, err := b.service.DescribeProduct(ctx, productID)
	if err != nil || describe == nil {
		return nil, err
	}
	out := make([]models.ProductFormField, 0, len(describe.FormSchema))
	for _, field := range describe.FormSchema {
		out = append(out, models.ProductFormField{
			Key:         field.Key,
			Label:       field.Label,
			Type:        field.Type,
			Required:    field.Required,
			Placeholder: field.Placeholder,
			MaxLength:   field.MaxLength,
		})
	}
	return out, nil
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

// decodeFormValues reads the purchase-form answers an order carries. A row that
// is present but unreadable yields no answers rather than failing the delivery:
// the shop still has the order, and the constraint is that the goods go out.
func decodeFormValues(raw string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}
