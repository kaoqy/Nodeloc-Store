package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/domain"
)

// ManualDeliveryKey is the built-in provider the shop uses when it wants a
// product's chosen variant to be delivered by hand: the mapping is what tells
// the operator which concrete item to send, and the same uniform works as it
// does for any other provider. Adding a real API-backed provider later means
// registering it next to this one.
const ManualDeliveryKey = "manual-delivery-v1"

// ManualDelivery is the reference provider. It honors the plugin standard without
// reaching the network: an order resolves to the operator's named item and the
// buyer is told which of their answers was matched.
type ManualDelivery struct{}

func NewManualDelivery() *ManualDelivery { return &ManualDelivery{} }

func (p *ManualDelivery) Key() string { return ManualDeliveryKey }

func (p *ManualDelivery) Manifest() contract.Manifest {
	return contract.Manifest{
		Key:          ManualDeliveryKey,
		Name:         "人工交付（按选项匹配）",
		Description:  "把购买表单里的选项匹配到具体的交付项目，付款后由店家按匹配结果人工发货。适合还没有 API 的商品，也是新插件接入前的同一套标准。",
		Version:      "1.0.0",
		Author:       "NodeLoc Store",
		Capabilities: []string{domain.CapabilityForm, domain.CapabilityFulfill},
		ConfigSchema: []contract.ConfigField{
			{
				Key:         "instructions",
				Label:       "交付说明",
				Type:        "text",
				Placeholder: "例如：付款后 24 小时内发货，请在订单里留言你的账号",
				Help:        "会附在订单的交付说明里，告诉店家或买家这一步怎么完成。",
			},
			{
				Key:   "notify_buyer",
				Label: "发货时通知买家",
				Type:  "bool",
				Help:  "开启后，发货内容会通过站内通知发给买家。",
			},
		},
	}
}

// Validate refuses an enabled plugin whose instructions are blank: a manual
// delivery with nothing to tell the buyer is a support ticket waiting to happen.
func (p *ManualDelivery) Validate(config map[string]string, secrets map[string]string) error {
	if strings.TrimSpace(config["instructions"]) == "" {
		return errors.New("请先填写交付说明")
	}
	return nil
}

func (p *ManualDelivery) Deliver(_ context.Context, request contract.DeliveryRequest) (contract.DeliveryResult, error) {
	remoteName := strings.TrimSpace(request.FormValues["__remote_name"])
	if remoteName == "" {
		remoteName = strings.TrimSpace(request.FormValues["__remote_ref"])
	}
	matchField := strings.TrimSpace(request.FormValues["__match_field"])
	answered := ""
	if matchField != "" {
		answered = strings.TrimSpace(request.FormValues[matchField])
	}
	var b strings.Builder
	fmt.Fprintf(&b, "订单 %s\n", request.OrderNo)
	fmt.Fprintf(&b, "商品：%s ×%d\n", request.Product, request.Quantity)
	if remoteName != "" {
		fmt.Fprintf(&b, "匹配到的交付项目：%s\n", remoteName)
	}
	if answered != "" {
		fmt.Fprintf(&b, "买家选择：%s\n", answered)
	}
	if request.Contact != "" {
		fmt.Fprintf(&b, "联系方式：%s\n", request.Contact)
	}
	if request.Note != "" {
		fmt.Fprintf(&b, "买家备注：%s\n", request.Note)
	}
	return contract.DeliveryResult{
		Content:   b.String(),
		Note:      "按选项匹配的人工交付",
		Reference: finishedReference(request.OrderNo),
	}, nil
}

// finishedReference is what a duplicate call is recognised by. The money side
// already guards against re-delivery; the provider keeps its own stable id so a
// retry would not produce a second payload.
func finishedReference(orderNo string) string {
	return "manual:" + orderNo
}
