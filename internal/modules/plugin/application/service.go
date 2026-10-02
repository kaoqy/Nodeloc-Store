package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/domain"
)

// Service is the plugin module's use case layer: it keeps enrollments, their
// product mappings and the delivery hook in one place, and it never talks to the
// database or to a provider's HTTP client directly.
type Service struct {
	repo     contract.Repository
	registry contract.Registry
	now      func() time.Time
}

func NewService(repo contract.Repository, registry contract.Registry) (*Service, error) {
	if repo == nil || registry == nil {
		return nil, errors.New("plugin service dependencies are required")
	}
	return &Service{repo: repo, registry: registry, now: time.Now}, nil
}

// CatalogEntry is one provider as 插件管理 shows it: what it is, whether the
// shop enrolled it, and the non-secret half of its configuration.
type CatalogEntry struct {
	Manifest     contract.Manifest `json:"manifest"`
	Installed    bool              `json:"installed"`
	Enabled      bool              `json:"enabled"`
	PluginID     uint              `json:"plugin_id,omitempty"`
	Settings     map[string]string `json:"settings,omitempty"`
	SecretFields []string          `json:"secret_fields,omitempty"`
	BindingCount int64             `json:"binding_count"`
	InstalledAt  *time.Time        `json:"installed_at,omitempty"`
}

// Catalog lists every provider the binary carries, joined with whatever the shop
// has enrolled. It is the one read the 插件管理 screen needs.
func (s *Service) Catalog(ctx context.Context) ([]CatalogEntry, error) {
	installed, err := s.repo.ListPlugins(ctx)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]domain.Plugin, len(installed))
	for _, plugin := range installed {
		byKey[plugin.Key] = plugin
	}

	entries := make([]CatalogEntry, 0, len(s.registry.All()))
	for _, provider := range s.registry.All() {
		manifest := provider.Manifest()
		entry := CatalogEntry{Manifest: manifest}
		plugin, ok := byKey[manifest.Key]
		if !ok {
			entries = append(entries, entry)
			continue
		}
		entry.Installed = true
		entry.Enabled = plugin.IsEnabled
		entry.PluginID = plugin.ID
		entry.InstalledAt = plugin.InstalledAt
		entry.Settings = decodeMap(plugin.Settings)
		entry.SecretFields = presentSecretFields(manifest.ConfigSchema, decodeMap(plugin.ConfigSecrets))
		bindings, err := s.repo.ListBindings(ctx, plugin.ID)
		if err != nil {
			return nil, err
		}
		entry.BindingCount = int64(len(bindings))
		entries = append(entries, entry)
	}
	return entries, nil
}

// Enroll creates the shop's row for a provider. Enrolling twice is refused:
// two rows for one key would run the same delivery hook twice.
func (s *Service) Enroll(ctx context.Context, key string) (*domain.Plugin, error) {
	key = strings.TrimSpace(key)
	provider, ok := s.registry.Lookup(key)
	if !ok {
		return nil, fmt.Errorf("%w: 没有内置名为 %q 的插件提供者", domain.ErrInvalidInput, key)
	}
	if existing, err := s.repo.FindPluginByKey(ctx, key); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, fmt.Errorf("%w: 这个插件已经启用过了", domain.ErrInvalidInput)
	}
	manifest := provider.Manifest()
	schema, err := json.Marshal(manifest.ConfigSchema)
	if err != nil {
		return nil, fmt.Errorf("encode config schema: %w", err)
	}
	capabilities, err := json.Marshal(domain.NormalizeCapabilities(manifest.Capabilities))
	if err != nil {
		return nil, fmt.Errorf("encode capabilities: %w", err)
	}
	now := s.now().UTC()
	plugin := &domain.Plugin{
		Key:           manifest.Key,
		Name:          manifest.Name,
		Description:   manifest.Description,
		Version:       manifest.Version,
		Author:        manifest.Author,
		IsEnabled:     false,
		Settings:      "{}",
		ConfigSecrets: "{}",
		ConfigSchema:  string(schema),
		Capabilities:  string(capabilities),
		InstalledAt:   &now,
	}
	if err := s.repo.CreatePlugin(ctx, plugin); err != nil {
		return nil, err
	}
	return plugin, nil
}

// Plugins lists what the shop has enrolled.
func (s *Service) Plugins(ctx context.Context) ([]domain.Plugin, error) {
	return s.repo.ListPlugins(ctx)
}

func (s *Service) Plugin(ctx context.Context, id uint) (*domain.Plugin, error) {
	if id == 0 {
		return nil, domain.ErrInvalidInput
	}
	return s.repo.FindPlugin(ctx, id)
}

// SetEnabled flips one enrolled plugin. Enabling refuses when the provider's
// own validation says a required setting is still missing, so the shop cannot
// switch on a plugin that would fail on the first order.
func (s *Service) SetEnabled(ctx context.Context, id uint, enabled bool) (*domain.Plugin, error) {
	plugin, err := s.repo.FindPlugin(ctx, id)
	if err != nil {
		return nil, err
	}
	if enabled {
		provider, ok := s.registry.Lookup(plugin.Key)
		if !ok {
			return nil, fmt.Errorf("%w: 这个插件的提供者已不在本版本里", domain.ErrInvalidInput)
		}
		if err := provider.Validate(decodeMap(plugin.Settings), decodeMap(plugin.ConfigSecrets)); err != nil {
			return nil, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
		}
	}
	plugin.IsEnabled = enabled
	if err := s.repo.UpdatePlugin(ctx, plugin); err != nil {
		return nil, err
	}
	return plugin, nil
}

// UpdateConfig writes a plugin's configuration.
//
// Secrets are write-only: a blank submission keeps the stored value, so the back
// office form never has to carry a credential back to the browser. A field the
// owner explicitly clears (by sending the cleared marker) is removed.
func (s *Service) UpdateConfig(ctx context.Context, id uint, settings, secrets map[string]string) (*domain.Plugin, error) {
	plugin, err := s.repo.FindPlugin(ctx, id)
	if err != nil {
		return nil, err
	}
	provider, ok := s.registry.Lookup(plugin.Key)
	if !ok {
		return nil, fmt.Errorf("%w: 这个插件的提供者已不在本版本里", domain.ErrInvalidInput)
	}
	manifest := provider.Manifest()
	cleanSettings := map[string]string{}
	for _, field := range manifest.ConfigSchema {
		if isSecretField(field) {
			continue
		}
		if value, ok := settings[field.Key]; ok {
			cleanSettings[field.Key] = strings.TrimSpace(value)
		}
	}
	storedSecrets := decodeMap(plugin.ConfigSecrets)
	nextSecrets := map[string]string{}
	for _, field := range manifest.ConfigSchema {
		if !isSecretField(field) {
			continue
		}
		submitted, provided := secrets[field.Key]
		if !provided {
			if existing := strings.TrimSpace(storedSecrets[field.Key]); existing != "" {
				nextSecrets[field.Key] = existing
			}
			continue
		}
		submitted = strings.TrimSpace(submitted)
		switch submitted {
		case "":
			// An untouched password box submits empty; that means "keep what is
			// stored", not "erase it".
			if existing := strings.TrimSpace(storedSecrets[field.Key]); existing != "" {
				nextSecrets[field.Key] = existing
			}
		case clearSecretMarker:
			// Explicit erase, so an owner can remove a credential without
			// reinstalling the plugin.
		default:
			nextSecrets[field.Key] = submitted
		}
	}

	settingsJSON, err := json.Marshal(cleanSettings)
	if err != nil {
		return nil, fmt.Errorf("encode plugin settings: %w", err)
	}
	secretsJSON, err := json.Marshal(nextSecrets)
	if err != nil {
		return nil, fmt.Errorf("encode plugin secrets: %w", err)
	}
	plugin.Settings = string(settingsJSON)
	plugin.ConfigSecrets = string(secretsJSON)
	if plugin.IsEnabled {
		if err := provider.Validate(cleanSettings, nextSecrets); err != nil {
			return nil, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
		}
	}
	if err := s.repo.UpdatePlugin(ctx, plugin); err != nil {
		return nil, err
	}
	return plugin, nil
}

// clearSecretMarker is what the back office sends to mean 「删掉这个凭据」, as
// opposed to an empty box, which means 「保持原样」.
const clearSecretMarker = "__clear__"

// Uninstall removes an enrollment and its mappings. Orders already delivered
// keep their delivered content: uninstalling is not a refund.
func (s *Service) Uninstall(ctx context.Context, id uint) error {
	if id == 0 {
		return domain.ErrInvalidInput
	}
	return s.repo.DeletePlugin(ctx, id)
}

// ProvisionSchema is the JSON config form for one provider, so the front end can
// render settings without a build-time knowledge of any plugin.
func (s *Service) ProvisionSchema(key string) ([]contract.ConfigField, bool) {
	provider, ok := s.registry.Lookup(strings.TrimSpace(key))
	if !ok {
		return nil, false
	}
	return provider.Manifest().ConfigSchema, true
}

func isSecretField(field contract.ConfigField) bool {
	return strings.EqualFold(strings.TrimSpace(field.Type), "password")
}

func decodeMap(raw string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func presentSecretFields(schema []contract.ConfigField, secrets map[string]string) []string {
	present := []string{}
	for _, field := range schema {
		if !isSecretField(field) {
			continue
		}
		if strings.TrimSpace(secrets[field.Key]) != "" {
			present = append(present, field.Key)
		}
	}
	return present
}
