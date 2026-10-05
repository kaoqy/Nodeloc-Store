package contract

import (
	"context"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/domain"
)

// Repository is the persistence port for plugin enrollments and their product
// mappings.
type Repository interface {
	ListPlugins(ctx context.Context) ([]domain.Plugin, error)
	CountPlugins(ctx context.Context) (int64, error)
	FindPlugin(ctx context.Context, id uint) (*domain.Plugin, error)
	FindPluginByKey(ctx context.Context, key string) (*domain.Plugin, error)
	CreatePlugin(ctx context.Context, plugin *domain.Plugin) error
	UpdatePlugin(ctx context.Context, plugin *domain.Plugin) error
	DeletePlugin(ctx context.Context, id uint) error

	ListBindings(ctx context.Context, pluginID uint) ([]domain.PluginBinding, error)
	ListBindingsForProduct(ctx context.Context, productID uint) ([]domain.PluginBinding, error)
	// ResolveBinding is the checkout lookup: the enabled mapping for one product
	// and one answered value.
	ResolveBinding(ctx context.Context, productID uint, value string) (*domain.PluginBinding, error)
	FindBinding(ctx context.Context, id uint) (*domain.PluginBinding, error)
	CreateBinding(ctx context.Context, binding *domain.PluginBinding) error
	UpdateBinding(ctx context.Context, binding *domain.PluginBinding) error
	DeleteBinding(ctx context.Context, id uint) error
	// ProductExists guards a binding so a mapping cannot point at a product the
	// shop removed.
	ProductExists(ctx context.Context, productID uint) (bool, error)
	// OrderContext reads the names an order is delivered with. Payment hands the
	// plugin module the order alone (that is the module boundary), so the plugin
	// module looks the product title and the buyer's name up itself.
	OrderContext(ctx context.Context, orderID uint) (*OrderContext, error)
}

// OrderContext is what a plugin is told about the product and buyer behind an
// order, without the money module's own model crossing the boundary.
type OrderContext struct {
	ProductTitle string
	Username     string
}

// Resolved is what a purchase resolves to: the plugin that will deliver it and
// the provider-side reference the delivery uses.
type Resolved struct {
	Plugin       domain.Plugin
	Binding      domain.PluginBinding
	RemoteName   string
	RemoteRef    string
	Extra        string
	ProductTitle string
}

// DeliveryRequest is one order handed to a plugin to fulfil.
type DeliveryRequest struct {
	OrderID   uint
	OrderNo   string
	UserID    uint
	Username  string
	ProductID uint
	Product   string
	Quantity  int
	// PaidNLAmount is the NL amount the server recorded as actually paid for
	// this order. It is the only amount a provider may base a redemption on.
	PaidNLAmount int
	// TopupAmount is the New-API top-up amount recorded on the order. Providers
	// that sell by amount read this instead of re-parsing the purchase form, so
	// quota is always computed from the order the shop actually stored.
	TopupAmount int
	UnitPrice   int
	TotalPrice  int
	Contact     string
	Note        string
	// FormValues is the buyer's purchase form answers, already validated against
	// the product schema and keyed by field key.
	FormValues map[string]string
}

// DeliveryResult is what a plugin returns for one order. Content is what the
// buyer receives; Reference is the provider's own id for the delivery, kept so a
// duplicate call can be recognised rather than delivered twice.
type DeliveryResult struct {
	Content   string
	Note      string
	Reference string
	// Uncertain marks a delivery whose upstream request was sent but whose
	// result could not be confirmed. The order must not be marked delivered;
	// the shop operator reviews it instead of letting an automatic retry create
	// a second external resource.
	Uncertain bool
}

// Provider is the runtime half of a plugin: the code that checks its own
// configuration, describes its settings form, and delivers an order.
type Provider interface {
	// Key is the stable provider identifier stored on the Plugin row.
	Key() string
	// Manifest is the metadata the 插件管理 screen shows before installation.
	Manifest() Manifest
	// Validate checks a candidate configuration and says what is missing.
	Validate(config map[string]string, secrets map[string]string) error
	// Deliver fulfils one order.
	Deliver(ctx context.Context, request DeliveryRequest) (DeliveryResult, error)
}

// Manifest is a provider's self-description.
type Manifest struct {
	Key          string        `json:"key"`
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	Version      string        `json:"version"`
	Author       string        `json:"author"`
	Capabilities []string      `json:"capabilities"`
	ConfigSchema []ConfigField `json:"config_schema"`
	FormSchema   []FormField   `json:"form_schema,omitempty"`
}

// ConfigField is one input on a plugin's settings form. Secret fields are
// rendered by the back office as write-only: an empty submission keeps the
// stored value, so the front end never has to hold a credential.
type ConfigField struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"` // text | password | select | number | bool
	Required    bool     `json:"required"`
	Placeholder string   `json:"placeholder,omitempty"`
	Help        string   `json:"help,omitempty"`
	Options     []string `json:"options,omitempty"`
}

// FormField is one buyer-facing purchase-form field contributed by a provider.
// Keeping the schema here lets the storefront render it without putting any
// provider setting or credential into the product payload.
type FormField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type"` // text | number
	Required    bool   `json:"required"`
	Placeholder string `json:"placeholder,omitempty"`
	Help        string `json:"help,omitempty"`
	MaxLength   int    `json:"max_length,omitempty"`
}

// Registry is the set of providers the shop's binary carries.
type Registry interface {
	All() []Provider
	Lookup(key string) (Provider, bool)
}

// RuntimeConfigProvider supplies the New-API delivery channel's runtime
// configuration. It is an interface rather than a dependency on the system
// module so plugin infrastructure stays independent of the settings package.
type RuntimeConfigProvider interface {
	NewAPIConfig(ctx context.Context) (map[string]string, map[string]string, error)
}
