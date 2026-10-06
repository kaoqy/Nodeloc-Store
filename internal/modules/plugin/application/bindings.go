package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/domain"
)

// BindingView is one product mapping plus the labels the back office shows, so
// the 插件绑定 table does not have to look the product up itself.
type BindingView struct {
	domain.PluginBinding
	PluginName  string `json:"plugin_name"`
	PluginKey   string `json:"plugin_key"`
	ProductName string `json:"product_name"`
	ProductSlug string `json:"product_slug"`
}

// BindingInput is one mapping as submitted by 插件绑定.
type BindingInput struct {
	PluginID  uint   `json:"plugin_id" binding:"required"`
	ProductID uint   `json:"product_id" binding:"required"`
	Value     string `json:"value"`
	// MatchField names the purchase-form field whose answer selects this
	// mapping. It is the plugin standard's one required wiring: the same field
	// every provider reads to decide which concrete item to deliver.
	MatchField string `json:"match_field"`
	RemoteName string `json:"remote_name"`
	RemoteRef  string `json:"remote_ref"`
	Extra      string `json:"extra"`
	IsEnabled  *bool  `json:"is_enabled"`
	SortOrder  int    `json:"sort_order"`
}

// Bindings lists the mappings, optionally for one plugin or one product.
func (s *Service) Bindings(ctx context.Context, pluginID, productID uint) ([]BindingView, error) {
	var (
		bindings []domain.PluginBinding
		err      error
	)
	switch {
	case productID != 0:
		bindings, err = s.repo.ListBindingsForProduct(ctx, productID)
	case pluginID != 0:
		bindings, err = s.repo.ListBindings(ctx, pluginID)
	default:
		bindings, err = s.repo.ListBindings(ctx, 0)
	}
	if err != nil {
		return nil, err
	}
	domain.SortBindings(bindings)
	plugins, err := s.repo.ListPlugins(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[uint]domain.Plugin, len(plugins))
	for _, plugin := range plugins {
		byID[plugin.ID] = plugin
	}
	out := make([]BindingView, 0, len(bindings))
	for _, binding := range bindings {
		view := BindingView{PluginBinding: binding}
		if plugin, ok := byID[binding.PluginID]; ok {
			view.PluginName = plugin.Name
			view.PluginKey = plugin.Key
		}
		out = append(out, view)
	}
	return out, nil
}

// UpsertBinding creates or replaces the mapping for one (plugin, product, value)
// triple. The value is normalized so the same choice typed twice resolves once.
func (s *Service) UpsertBinding(ctx context.Context, input BindingInput) (*domain.PluginBinding, error) {
	if input.PluginID == 0 || input.ProductID == 0 {
		return nil, fmt.Errorf("%w: 请选择插件与商品", domain.ErrInvalidInput)
	}
	plugin, err := s.repo.FindPlugin(ctx, input.PluginID)
	if err != nil {
		return nil, err
	}
	exists, err := s.repo.ProductExists(ctx, input.ProductID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("%w: 商品不存在或已删除", domain.ErrInvalidInput)
	}
	value := domain.NormalizeBindingValue(input.Value)
	if len(value) > 120 {
		return nil, fmt.Errorf("%w: 匹配值最多 120 个字符", domain.ErrInvalidInput)
	}
	remoteName := strings.TrimSpace(input.RemoteName)
	remoteRef := strings.TrimSpace(input.RemoteRef)
	if len(remoteName) > 160 || len(remoteRef) > 160 {
		return nil, fmt.Errorf("%w: 交付项目名称或编号过长", domain.ErrInvalidInput)
	}
	if remoteRef == "" {
		return nil, fmt.Errorf("%w: 请填写提供方侧的交付项目编号（SKU / 套餐 ID / 模板 ID）", domain.ErrInvalidInput)
	}
	extra := strings.TrimSpace(input.Extra)
	if extra != "" {
		if !isJSONObject(extra) {
			return nil, fmt.Errorf("%w: 附加参数需要是 JSON 对象", domain.ErrInvalidInput)
		}
	}
	enabled := true
	if input.IsEnabled != nil {
		enabled = *input.IsEnabled
	}

	binding, err := s.repo.ResolveBinding(ctx, input.ProductID, value)
	switch {
	case err == nil && binding != nil:
		// An update may move the mapping to a different plugin, so the plugin id
		// is written as well as the provider-side fields.
		binding.PluginID = plugin.ID
		binding.MatchField = strings.TrimSpace(input.MatchField)
		binding.RemoteName = remoteName
		binding.RemoteRef = remoteRef
		binding.Extra = extra
		binding.IsEnabled = enabled
		binding.SortOrder = input.SortOrder
		if err := s.repo.UpdateBinding(ctx, binding); err != nil {
			return nil, err
		}
		return binding, nil
	case errors.Is(err, domain.ErrBindingNotFound), binding == nil:
		created := &domain.PluginBinding{
			PluginID:   plugin.ID,
			ProductID:  input.ProductID,
			Value:      value,
			MatchField: strings.TrimSpace(input.MatchField),
			RemoteName: remoteName,
			RemoteRef:  remoteRef,
			Extra:      extra,
			IsEnabled:  enabled,
			SortOrder:  input.SortOrder,
		}
		if err := s.repo.CreateBinding(ctx, created); err != nil {
			return nil, err
		}
		return created, nil
	default:
		return nil, err
	}
}

// UpdateBinding edits one mapping by id.
func (s *Service) UpdateBinding(ctx context.Context, id uint, input BindingInput) (*domain.PluginBinding, error) {
	binding, err := s.repo.FindBinding(ctx, id)
	if err != nil {
		return nil, err
	}
	if input.PluginID != 0 {
		if _, err := s.repo.FindPlugin(ctx, input.PluginID); err != nil {
			return nil, err
		}
		binding.PluginID = input.PluginID
	}
	binding.Value = domain.NormalizeBindingValue(input.Value)
	if len(binding.Value) > 120 {
		return nil, fmt.Errorf("%w: 匹配值最多 120 个字符", domain.ErrInvalidInput)
	}
	binding.MatchField = strings.TrimSpace(input.MatchField)
	if len(binding.MatchField) > 64 {
		return nil, fmt.Errorf("%w: 匹配字段名过长", domain.ErrInvalidInput)
	}
	if remoteRef := strings.TrimSpace(input.RemoteRef); remoteRef != "" {
		binding.RemoteRef = remoteRef
	}
	if len(binding.RemoteRef) > 160 {
		return nil, fmt.Errorf("%w: 交付项目编号过长", domain.ErrInvalidInput)
	}
	binding.RemoteName = strings.TrimSpace(input.RemoteName)
	if len(binding.RemoteName) > 160 {
		return nil, fmt.Errorf("%w: 交付项目名称过长", domain.ErrInvalidInput)
	}
	if extra := strings.TrimSpace(input.Extra); extra != "" {
		if !isJSONObject(extra) {
			return nil, fmt.Errorf("%w: 附加参数需要是 JSON 对象", domain.ErrInvalidInput)
		}
		binding.Extra = extra
	}
	if input.IsEnabled != nil {
		binding.IsEnabled = *input.IsEnabled
	}
	binding.SortOrder = input.SortOrder
	if err := s.repo.UpdateBinding(ctx, binding); err != nil {
		return nil, err
	}
	return binding, nil
}

func (s *Service) DeleteBinding(ctx context.Context, id uint) error {
	if id == 0 {
		return domain.ErrInvalidInput
	}
	return s.repo.DeleteBinding(ctx, id)
}

func isJSONObject(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	return strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")
}

// Fulfillability tells the storefront what a product's plugin needs before a
// buyer can check out: which form fields come from the plugin, and whether a
// mapping exists at all. It is deliberately read-only and safe to answer before
// login.
type Fulfillability struct {
	PluginKey    string               `json:"plugin_key"`
	PluginName   string               `json:"plugin_name"`
	RequiresForm bool                 `json:"requires_form"`
	MappingValue string               `json:"mapping_value,omitempty"`
	Options      []string             `json:"options,omitempty"`
	FormSchema   []contract.FormField `json:"form_schema,omitempty"`
	// NLToUSD is the shop's public NL→USD rate for the amount-type channel. It
	// is not a credential: showing it lets the product page state the rate the
	// server will use, so the buyer can see how the top-up becomes a redemption
	// amount instead of guessing.
	NLToUSD string `json:"nl_usd_rate,omitempty"`
	// DeliveryNote is a safe, non-secret explanation of what will be delivered.
	DeliveryNote string `json:"delivery_note,omitempty"`
}

// DescribeProduct reports what a product's plugin contributes. It is used by the
// storefront to render the plugin's own choices alongside the product form.
func (s *Service) DescribeProduct(ctx context.Context, productID uint) (*Fulfillability, error) {
	if productID == 0 {
		return nil, nil
	}
	bindings, err := s.repo.ListBindingsForProduct(ctx, productID)
	if err != nil {
		return nil, err
	}
	enabled := make([]domain.PluginBinding, 0, len(bindings))
	for _, binding := range bindings {
		if binding.IsEnabled {
			enabled = append(enabled, binding)
		}
	}
	if len(enabled) == 0 {
		return nil, nil
	}
	plugin, err := s.repo.FindPlugin(ctx, enabled[0].PluginID)
	if err != nil {
		return nil, err
	}
	describe := &Fulfillability{PluginKey: plugin.Key, PluginName: plugin.Name}
	if provider, ok := s.registry.Lookup(plugin.Key); ok {
		describe.FormSchema = provider.Manifest().FormSchema
	}
	if plugin.Key == "new-api-redemption-v1" {
		describe.DeliveryNote = "付款后创建 New-API 兑换码交给买家；买家需自行到 New-API 平台兑换，系统不会自动充值到账户。"
		if s.runtime != nil {
			if settings, _, err := s.runtime.NewAPIConfig(ctx); err == nil {
				describe.NLToUSD = strings.TrimSpace(settings["nl_usd_rate"])
			}
		}
	}
	// New-API needs no mapping choice: the provider contributes its own amount
	// field and the buyer enters the value. Requiring a non-empty binding value
	// here would hide the form for exactly the channel that needs it.
	if plugin.Key == "new-api-redemption-v1" {
		describe.RequiresForm = true
		return describe, nil
	}
	for _, binding := range enabled {
		if binding.Value != "" {
			describe.RequiresForm = true
			describe.Options = append(describe.Options, binding.RemoteName)
		}
	}
	return describe, nil
}

// SyncProductChannel mirrors a product's delivery channel into the plugin
// runtime. New-API needs one enabled binding for the payment router to recognise
// the product; the binding's value is deliberately empty because New-API creates
// a random code at payment time rather than mapping the order to a stored item.
// Choosing any other channel removes that binding, so a product switched back to
// card or manual delivery no longer routes through New-API.
func (s *Service) SyncProductChannel(ctx context.Context, productID uint, channel string) error {
	if productID == 0 {
		return fmt.Errorf("%w: 请选择商品", domain.ErrInvalidInput)
	}
	enabled := strings.EqualFold(strings.TrimSpace(channel), "new_api")
	existing, err := s.repo.ResolveBinding(ctx, productID, "")
	switch {
	case err == nil && existing != nil:
		existing.IsEnabled = enabled
		return s.repo.UpdateBinding(ctx, existing)
	case !errors.Is(err, domain.ErrBindingNotFound):
		return err
	case !enabled:
		return nil
	}
	plugins, err := s.repo.ListPlugins(ctx)
	if err != nil {
		return err
	}
	var target *domain.Plugin
	for i := range plugins {
		if plugins[i].Key == "new-api-redemption-v1" {
			target = &plugins[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("%w: New-API 发货渠道不在当前版本里", domain.ErrInvalidInput)
	}
	if err := s.requireNewAPIReady(ctx); err != nil {
		return err
	}
	return s.repo.CreateBinding(ctx, &domain.PluginBinding{
		PluginID:   target.ID,
		ProductID:  productID,
		Value:      "",
		RemoteName: "New-API 兑换码",
		RemoteRef:  "new-api-redemption-v1",
		IsEnabled:  true,
	})
}

// CheckProductChannel refuses a channel the shop cannot currently deliver.
// Today only New-API has a precondition — complete runtime credentials — but the
// check lives here so the catalogue does not have to know which channels have
// requirements.
func (s *Service) CheckProductChannel(ctx context.Context, channel string) error {
	if !strings.EqualFold(strings.TrimSpace(channel), "new_api") {
		return nil
	}
	return s.requireNewAPIReady(ctx)
}

func (s *Service) requireNewAPIReady(ctx context.Context) error {
	if s.runtime == nil {
		return fmt.Errorf("%w: New-API 运行时配置不可用", domain.ErrInvalidInput)
	}
	settings, secrets, err := s.runtime.NewAPIConfig(ctx)
	if err != nil {
		return fmt.Errorf("%w: 读取 New-API 配置失败：%v", domain.ErrInvalidInput, err)
	}
	required := []struct {
		key   string
		value string
		label string
	}{
		{"base_url", settings["base_url"], "API 基础地址"},
		{"admin_access_token", secrets["admin_access_token"], "管理员 AccessToken"},
		{"admin_user_id", settings["admin_user_id"], "管理员用户 ID"},
		{"nl_usd_rate", settings["nl_usd_rate"], "NL 与美元兑换比例"},
	}
	missing := make([]string, 0, len(required))
	for _, item := range required {
		if strings.TrimSpace(item.value) == "" {
			missing = append(missing, item.label)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: 请先在系统设置中补全 New-API 配置：%s", domain.ErrInvalidInput, strings.Join(missing, "、"))
	}
	return nil
}

// NewAPIProductEnabled reports whether one product uses the New-API channel.
func (s *Service) NewAPIProductEnabled(ctx context.Context, productID uint) (bool, error) {
	if productID == 0 {
		return false, nil
	}
	bindings, err := s.repo.ListBindingsForProduct(ctx, productID)
	if err != nil {
		return false, err
	}
	for _, binding := range bindings {
		if binding.IsEnabled && binding.RemoteRef == "new-api-redemption-v1" {
			return true, nil
		}
	}
	return false, nil
}

// Resolve is the checkout call: it turns a product and the buyer's answered value
// into the delivery the plugin will perform.
func (s *Service) Resolve(ctx context.Context, productID uint, value string) (*contract.Resolved, error) {
	bindings, err := s.repo.ListBindingsForProduct(ctx, productID)
	if err != nil {
		return nil, err
	}
	if len(bindings) == 0 {
		return nil, nil
	}
	normalized := domain.NormalizeBindingValue(value)
	binding, err := s.repo.ResolveBinding(ctx, productID, normalized)
	if err != nil {
		if errors.Is(err, domain.ErrBindingNotFound) {
			return nil, fmt.Errorf("%w：%s", domain.ErrNoBinding, describeOptions(bindings))
		}
		return nil, err
	}
	plugin, err := s.repo.FindPlugin(ctx, binding.PluginID)
	if err != nil {
		return nil, err
	}
	if !plugin.IsEnabled {
		return nil, fmt.Errorf("%w: 这个商品对应的插件目前没有启用", domain.ErrPluginDisabled)
	}
	return &contract.Resolved{
		Plugin:     *plugin,
		Binding:    *binding,
		RemoteName: binding.RemoteName,
		RemoteRef:  binding.RemoteRef,
		Extra:      binding.Extra,
	}, nil
}

func describeOptions(bindings []domain.PluginBinding) string {
	labels := []string{}
	for _, binding := range bindings {
		if !binding.IsEnabled {
			continue
		}
		if binding.RemoteName != "" {
			labels = append(labels, binding.RemoteName)
		}
	}
	if len(labels) == 0 {
		return "请重新选择"
	}
	return "可选：" + strings.Join(labels, "、")
}
