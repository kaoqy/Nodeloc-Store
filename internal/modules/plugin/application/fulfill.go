package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/domain"
)

// ErrNoDeliverer is the answer when a product is bound to a plugin whose
// provider is no longer in this build. The order stays paid and undelivered, and
// the back office has to say which plugin went missing.
var ErrNoDeliverer = errors.New("这个商品绑定的插件不在当前版本里")

// OrderInfo is what the plugin module is told about an order. It is a small
// struct rather than the money module's models.Order so the modules stay on
// their own sides of the boundary.
type OrderInfo struct {
	ID         uint
	OrderNo    string
	UserID     uint
	ProductID  uint
	Quantity   int
	UnitPrice  int
	TotalPrice int
	Contact    string
	Note       string
	FormValues map[string]string
}

// Owns reports whether an enabled plugin is bound to this order's product.
func (s *Service) Owns(ctx context.Context, productID uint) (bool, error) {
	return s.ProductNeedsPlugin(ctx, productID)
}

// Deliver resolves one order and hands it to its plugin.
//
// It is the single entry point the money side calls after a payment settles, so
// a plugin is never wired into checkout directly: the order, the buyer's form
// answers and the resolved mapping all arrive here on the plugin module's own
// terms.
func (s *Service) Deliver(ctx context.Context, info OrderInfo) (*contract.DeliveryResult, error) {
	if info.ProductID == 0 {
		return nil, fmt.Errorf("%w: 缺少商品", domain.ErrInvalidInput)
	}
	bindings, err := s.repo.ListBindingsForProduct(ctx, info.ProductID)
	if err != nil {
		return nil, err
	}
	if len(bindings) == 0 {
		// Nothing is bound to this product: the plugin module is not the
		// deliverer, and the caller falls back to the shop's own card/manual
		// delivery. This is not an error.
		return nil, nil
	}

	// The match field is part of the plugin standard rather than each provider's
	// own convention: the first binding's field names the purchase-form answer
	// every mapping for this product is compared against.
	matchField := strings.TrimSpace(bindings[0].MatchField)
	value := ""
	if matchField != "" {
		value = strings.TrimSpace(info.FormValues[matchField])
	}

	binding, err := s.repo.ResolveBinding(ctx, info.ProductID, domain.NormalizeBindingValue(value))
	if err != nil {
		if errors.Is(err, domain.ErrBindingNotFound) {
			// The buyer answered something with no mapping. Refusing is the whole
			// point of the standard: delivering a plausible-but-wrong item is
			// worse than asking the shop to add the missing mapping.
			if value == "" {
				return nil, fmt.Errorf("%w：这个商品没有可用的插件匹配规则", domain.ErrNoBinding)
			}
			return nil, fmt.Errorf("%w：%q 还没有对应的交付项目（%s）", domain.ErrNoBinding, value, describeOptions(bindings))
		}
		return nil, err
	}

	plugin, err := s.repo.FindPlugin(ctx, binding.PluginID)
	if err != nil {
		return nil, err
	}
	if !plugin.IsEnabled {
		return nil, fmt.Errorf("%w: 插件 %q 已停用，订单需要人工处理", domain.ErrPluginDisabled, plugin.Name)
	}
	provider, ok := s.registry.Lookup(plugin.Key)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoDeliverer, plugin.Key)
	}

	orderContext, err := s.repo.OrderContext(ctx, info.ID)
	if err != nil {
		return nil, err
	}
	product := ""
	username := ""
	if orderContext != nil {
		product = orderContext.ProductTitle
		username = orderContext.Username
	}

	formValues := cloneMap(info.FormValues)
	formValues["__remote_ref"] = binding.RemoteRef
	formValues["__remote_name"] = binding.RemoteName
	formValues["__match_field"] = matchField
	formValues["__plugin_config"] = strings.TrimSpace(plugin.Settings)
	// Secrets are handed to the provider through the request map, never through
	// the database-facing plugin object or the API response. Providers must use
	// them only for the outbound request and must not log or echo them.
	if strings.TrimSpace(plugin.ConfigSecrets) != "" {
		formValues["__plugin_secrets"] = strings.TrimSpace(plugin.ConfigSecrets)
	}

	result, err := provider.Deliver(ctx, contract.DeliveryRequest{
		OrderID:    info.ID,
		OrderNo:    info.OrderNo,
		UserID:     info.UserID,
		Username:   username,
		ProductID:  info.ProductID,
		Product:    product,
		Quantity:   info.Quantity,
		UnitPrice:  info.UnitPrice,
		TotalPrice: info.TotalPrice,
		Contact:    info.Contact,
		Note:       info.Note,
		FormValues: formValues,
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ValidateSelection is the checkout-time check: it refuses an order whose
// answered options do not resolve to a plugin delivery item, before any money
// moves. Failing here gives the buyer a sentence they can act on; the same
// condition discovered at delivery time would be a paid order the shop has to
// untangle by hand.
func (s *Service) ValidateSelection(ctx context.Context, productID uint, formValues map[string]string) error {
	if productID == 0 {
		return nil
	}
	bindings, err := s.repo.ListBindingsForProduct(ctx, productID)
	if err != nil {
		return err
	}
	if len(bindings) == 0 {
		return nil
	}
	matchField := strings.TrimSpace(bindings[0].MatchField)
	if matchField == "" {
		return nil
	}
	value := ""
	if formValues != nil {
		value = strings.TrimSpace(formValues[matchField])
	}
	if _, err := s.repo.ResolveBinding(ctx, productID, domain.NormalizeBindingValue(value)); err != nil {
		if errors.Is(err, domain.ErrBindingNotFound) {
			if value == "" {
				return fmt.Errorf("%w：请先选择要购买的交付项目", domain.ErrNoBinding)
			}
			return fmt.Errorf("%w：%q 还没有对应的交付项目（%s）", domain.ErrNoBinding, value, describeOptions(bindings))
		}
		return err
	}
	return nil
}

// ProductNeedsPlugin reports whether checkout must route this product through a
// plugin at all.
func (s *Service) ProductNeedsPlugin(ctx context.Context, productID uint) (bool, error) {
	if productID == 0 {
		return false, nil
	}
	bindings, err := s.repo.ListBindingsForProduct(ctx, productID)
	if err != nil {
		return false, err
	}
	for _, binding := range bindings {
		if binding.IsEnabled {
			return true, nil
		}
	}
	return false, nil
}

func cloneMap(source map[string]string) map[string]string {
	out := make(map[string]string, len(source)+3)
	for key, value := range source {
		out[key] = value
	}
	return out
}
