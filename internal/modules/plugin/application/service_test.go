package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/infrastructure"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/domain"
)

// fakeRepo is the persistence half of the plugin module as these tests need it.
// The embedded interface means an unexercised method panics loudly instead of
// quietly answering a zero value.
type fakeRepo struct {
	contract.Repository
	plugins  []domain.Plugin
	bindings []domain.PluginBinding
	contexts map[uint]*contract.OrderContext
}

func (r *fakeRepo) ListPlugins(context.Context) ([]domain.Plugin, error) { return r.plugins, nil }

func (r *fakeRepo) FindPlugin(_ context.Context, id uint) (*domain.Plugin, error) {
	for i := range r.plugins {
		if r.plugins[i].ID == id {
			plugin := r.plugins[i]
			return &plugin, nil
		}
	}
	return nil, domain.ErrPluginNotFound
}

func (r *fakeRepo) UpdatePlugin(_ context.Context, plugin *domain.Plugin) error {
	for i := range r.plugins {
		if r.plugins[i].ID == plugin.ID {
			r.plugins[i] = *plugin
			return nil
		}
	}
	return domain.ErrPluginNotFound
}

func (r *fakeRepo) ListBindings(_ context.Context, pluginID uint) ([]domain.PluginBinding, error) {
	out := []domain.PluginBinding{}
	for _, binding := range r.bindings {
		if pluginID == 0 || binding.PluginID == pluginID {
			out = append(out, binding)
		}
	}
	return out, nil
}

func (r *fakeRepo) ListBindingsForProduct(_ context.Context, productID uint) ([]domain.PluginBinding, error) {
	out := []domain.PluginBinding{}
	for _, binding := range r.bindings {
		if binding.ProductID == productID {
			out = append(out, binding)
		}
	}
	return out, nil
}

// ProductDeliveryChannel is only read to repair a missing New-API binding. The
// fake returns "" so these tests see "nothing to repair" unless they opt in.
func (r *fakeRepo) ProductDeliveryChannel(context.Context, uint) (string, error) {
	return "", nil
}

func (r *fakeRepo) ResolveBinding(_ context.Context, productID uint, value string) (*domain.PluginBinding, error) {
	for _, binding := range r.bindings {
		if binding.ProductID == productID && binding.Value == value && binding.IsEnabled {
			match := binding
			return &match, nil
		}
	}
	return nil, domain.ErrBindingNotFound
}

func (r *fakeRepo) OrderContext(_ context.Context, orderID uint) (*contract.OrderContext, error) {
	if info, ok := r.contexts[orderID]; ok {
		return info, nil
	}
	return &contract.OrderContext{}, nil
}

// fakeProvider records what it was asked to deliver.
type fakeProvider struct {
	key       string
	capab     []string
	delivered []contract.DeliveryRequest
	validate  func(config, secrets map[string]string) error
}

func (p *fakeProvider) Key() string { return p.key }

func (p *fakeProvider) Manifest() contract.Manifest {
	return contract.Manifest{Key: p.key, Name: "测试插件", Capabilities: p.capab,
		ConfigSchema: []contract.ConfigField{{Key: "token", Label: "Token", Type: "password", Required: true}}}
}

func (p *fakeProvider) Validate(config, secrets map[string]string) error {
	if p.validate != nil {
		return p.validate(config, secrets)
	}
	if strings.TrimSpace(secrets["token"]) == "" {
		return errors.New("请先填写 Token")
	}
	return nil
}

func (p *fakeProvider) Deliver(_ context.Context, request contract.DeliveryRequest) (contract.DeliveryResult, error) {
	p.delivered = append(p.delivered, request)
	return contract.DeliveryResult{Content: "delivered:" + request.FormValues["__remote_ref"], Note: "ok", Reference: "ref-1"}, nil
}

// registryFor is the tiny registry these tests need.
type registryFor struct{ providers []contract.Provider }

func (r registryFor) All() []contract.Provider { return r.providers }

func (r registryFor) Lookup(key string) (contract.Provider, bool) {
	for _, provider := range r.providers {
		if provider.Key() == key {
			return provider, true
		}
	}
	return nil, false
}

func newPluginService(t *testing.T, repo *fakeRepo, provider contract.Provider) *Service {
	t.Helper()
	service, err := NewService(repo, registryFor{providers: []contract.Provider{provider}})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

func fixtures() (*fakeRepo, *fakeProvider) {
	provider := &fakeProvider{key: "demo-v1", capab: []string{domain.CapabilityFulfill}}
	repo := &fakeRepo{
		plugins: []domain.Plugin{{Key: provider.key, Name: "测试插件", IsEnabled: true, Settings: "{}"}},
		bindings: []domain.PluginBinding{
			{PluginID: 1, ProductID: 10, MatchField: "region", Value: "beijing", RemoteName: "北京节点", RemoteRef: "sku-bj", IsEnabled: true},
			{PluginID: 1, ProductID: 10, MatchField: "region", Value: "shanghai", RemoteName: "上海节点", RemoteRef: "sku-sh", IsEnabled: true},
		},
		contexts: map[uint]*contract.OrderContext{
			7: {ProductTitle: "云主机", Username: "buyer"},
		},
	}
	// The fake repo stores plugin ids starting at 1, matching the fixture.
	repo.plugins[0].ID = 1
	return repo, provider
}

// The standard is that a buyer’s answer selects a concrete delivery item, and
// that the comparison does not depend on case or stray spaces.
func TestSelectionMatchesIgnoringCaseAndSpaces(t *testing.T) {
	repo, provider := fixtures()
	service := newPluginService(t, repo, provider)

	for _, answer := range []string{"Beijing", " beijing ", "BEIJING"} {
		if err := service.ValidateSelection(context.Background(), 10, map[string]string{"region": answer}); err != nil {
			t.Fatalf("answer %q was refused: %v", answer, err)
		}
	}
	if err := service.ValidateSelection(context.Background(), 10, map[string]string{"region": "mars"}); err == nil {
		t.Fatal("an answer with no mapping was accepted at checkout")
	}
	if err := service.ValidateSelection(context.Background(), 10, map[string]string{}); err == nil {
		t.Fatal("an empty answer was accepted at checkout")
	}
}

// A product with no plugin binding is the shop’s own business: the plugin module
// must not claim it, or its card/manual delivery would be skipped.
func TestUnboundProductIsNotClaimed(t *testing.T) {
	repo, provider := fixtures()
	service := newPluginService(t, repo, provider)

	owns, err := service.Owns(context.Background(), 99)
	if err != nil {
		t.Fatalf("Owns: %v", err)
	}
	if owns {
		t.Fatal("a product with no binding was claimed by the plugin")
	}
	result, err := service.Deliver(context.Background(), OrderInfo{ID: 7, ProductID: 99})
	if err != nil || result != nil {
		t.Fatalf("Deliver on an unbound product = %+v, %v; want no result and no error", result, err)
	}
}

// A delivery must carry the matched item and the order’s own names, so a
// provider never has to reach back into the shop to know what it is sending.
func TestDeliverCarriesTheMatchedItemAndOrderContext(t *testing.T) {
	repo, provider := fixtures()
	service := newPluginService(t, repo, provider)

	result, err := service.Deliver(context.Background(), OrderInfo{
		ID: 7, OrderNo: "NL1001", UserID: 3, ProductID: 10, Quantity: 1,
		FormValues: map[string]string{"region": "SHANGHAI"},
	})
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if result == nil || result.Content != "delivered:sku-sh" {
		t.Fatalf("result = %+v, want the Shanghai item", result)
	}
	if len(provider.delivered) != 1 {
		t.Fatalf("provider deliveries = %d, want one", len(provider.delivered))
	}
	sent := provider.delivered[0]
	if sent.Product != "云主机" || sent.Username != "buyer" {
		t.Fatalf("order context did not reach the provider: %+v", sent)
	}
	if sent.FormValues["__remote_ref"] != "sku-sh" || sent.FormValues["__match_field"] != "region" {
		t.Fatalf("resolved mapping did not reach the provider: %+v", sent.FormValues)
	}
}

// A plugin the owner switched off must not deliver; the order has to land in the
// human queue instead of being silently dropped.
func TestDisabledPluginRefusesDelivery(t *testing.T) {
	repo, provider := fixtures()
	repo.plugins[0].IsEnabled = false
	service := newPluginService(t, repo, provider)

	_, err := service.Deliver(context.Background(), OrderInfo{
		ID: 7, OrderNo: "NL1002", ProductID: 10, FormValues: map[string]string{"region": "beijing"},
	})
	if !errors.Is(err, domain.ErrPluginDisabled) {
		t.Fatalf("delivery through a disabled plugin = %v, want ErrPluginDisabled", err)
	}
}

// Enabling a plugin with a missing required credential is refused up front, so
// the first order is not the thing that discovers it.
func TestEnableRefusesAMissingRequiredSecret(t *testing.T) {
	repo, provider := fixtures()
	service := newPluginService(t, repo, provider)

	if _, err := service.SetEnabled(context.Background(), 1, true); err == nil {
		t.Fatal("a plugin with no Token was enabled")
	}

	updated, err := service.UpdateConfig(context.Background(), 1, map[string]string{}, map[string]string{"token": "secret-1"})
	if err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	if updated.ConfigSecrets == "" {
		t.Fatal("the credential was not stored")
	}
	if _, err := service.SetEnabled(context.Background(), 1, true); err != nil {
		t.Fatalf("enabling a fully configured plugin: %v", err)
	}

	// A blank submission means 「keep what is stored」 — that is what an untouched
	// password box sends back.
	if _, err := service.UpdateConfig(context.Background(), 1, map[string]string{}, map[string]string{"token": ""}); err != nil {
		t.Fatalf("UpdateConfig with a blank secret: %v", err)
	}
	plugin, err := service.Plugin(context.Background(), 1)
	if err != nil {
		t.Fatalf("Plugin: %v", err)
	}
	if !strings.Contains(plugin.ConfigSecrets, "secret-1") {
		t.Fatalf("a blank submission erased the stored credential: %s", plugin.ConfigSecrets)
	}
}

// The credential itself must never appear in a catalog answer, which is what the
// 插件管理 screen reads.
func TestCatalogNeverCarriesASecretValue(t *testing.T) {
	repo, provider := fixtures()
	repo.plugins[0].ConfigSecrets = `{"token":"super-secret-value"}`
	repo.plugins[0].IsEnabled = true
	service := newPluginService(t, repo, provider)

	entries, err := service.Catalog(context.Background())
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("catalog entries = %d, want one", len(entries))
	}
	if len(entries[0].SecretFields) != 1 || entries[0].SecretFields[0] != "token" {
		t.Fatalf("secret fields = %v, want the token named as present", entries[0].SecretFields)
	}

	encoded := fmt.Sprintf("%+v", entries[0])
	if strings.Contains(encoded, "super-secret-value") {
		t.Fatalf("the credential leaked into a catalog answer: %s", encoded)
	}
}

// builtInService wires the real provider the shop actually ships, so these tests
// fail if the shipping plugin's schema and its save path stop agreeing.
func builtInService(t *testing.T) (*Service, *fakeRepo) {
	t.Helper()
	provider := infrastructure.NewManualDelivery()
	plugin := domain.Plugin{
		Key: provider.Key(), Name: "人工交付", IsEnabled: true, Settings: "{}", ConfigSecrets: "{}",
	}
	plugin.ID = 1
	repo := &fakeRepo{
		plugins:  []domain.Plugin{plugin},
		bindings: []domain.PluginBinding{},
		contexts: map[uint]*contract.OrderContext{},
	}
	service, err := NewService(repo, registryFor{providers: []contract.Provider{provider}})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service, repo
}

// The 交付说明 field and the 发货时通知买家 checkbox are the two fields the built-in
// plugin ships with, and both used to be lost on save: the text field was bound
// to the secrets map (so the required value never reached the server) and the
// checkbox arrived as a bare boolean (so binding answered 400). This pins the
// whole round trip against the real provider's schema.
func TestSavingTheBuiltInPluginConfigStoresBothFields(t *testing.T) {
	service, _ := builtInService(t)

	stored, err := service.UpdateConfig(context.Background(), 1,
		map[string]string{"instructions": "付款后 24 小时内发货", "notify_buyer": "true"},
		map[string]string{},
	)
	if err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	if stored.ValidationWarning != "" {
		t.Fatalf("a complete configuration still warned: %q", stored.ValidationWarning)
	}
	if !strings.Contains(stored.Settings, "付款后 24 小时内发货") {
		t.Fatalf("the instructions were not stored: %s", stored.Settings)
	}
	if !strings.Contains(stored.Settings, `"notify_buyer":"true"`) {
		t.Fatalf("the checkbox was not normalised to true: %s", stored.Settings)
	}

	// Every spelling a browser checkbox can send has to land as one canonical
	// value, so the provider never has to guess.
	for _, spelling := range []string{"on", "1", "TRUE", "yes", "false", "0", "junk"} {
		updated, err := service.UpdateConfig(context.Background(), 1,
			map[string]string{"instructions": "x", "notify_buyer": spelling},
			map[string]string{},
		)
		if err != nil {
			t.Fatalf("UpdateConfig(%q): %v", spelling, err)
		}
		want := `"notify_buyer":"false"`
		if strings.EqualFold(spelling, "on") || spelling == "1" || strings.EqualFold(spelling, "true") || strings.EqualFold(spelling, "yes") {
			want = `"notify_buyer":"true"`
		}
		if !strings.Contains(updated.Settings, want) {
			t.Fatalf("checkbox spelling %q stored as %s, want %s", spelling, updated.Settings, want)
		}
	}
}

// Saving an incomplete configuration has to succeed: a fresh install has no
// 交付说明 yet, and refusing the save would leave the owner with a 「保存」 that can
// never work (enabling is what Validate gates). The provider's own complaint
// rides back as a warning instead, so the screen can say what is still missing.
func TestSavingAnIncompleteConfigSucceedsWithAWarning(t *testing.T) {
	service, _ := builtInService(t)

	stored, err := service.UpdateConfig(context.Background(), 1, map[string]string{"instructions": "  "}, map[string]string{})
	if err != nil {
		t.Fatalf("an incomplete configuration was refused at save time: %v", err)
	}
	if stored.ValidationWarning == "" {
		t.Fatal("an incomplete configuration came back without a warning")
	}
	if _, err := service.SetEnabled(context.Background(), 1, true); err == nil {
		t.Fatal("a plugin with no 交付说明 was enabled")
	}
}
