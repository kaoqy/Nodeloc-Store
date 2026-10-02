package application

import (
	"context"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/domain"
)

// configStubRepo serves a fixed set of system configs so the accessor helpers
// can be tested without a database.
type configStubRepo struct {
	stubRepo
	configs []domain.SystemConfig
}

func (c *configStubRepo) ListSystemConfigs(context.Context, string) ([]domain.SystemConfig, error) {
	return c.configs, nil
}

func newConfigService(t *testing.T, configs []domain.SystemConfig) *Service {
	t.Helper()
	service, err := NewService(Deps{Repo: &configStubRepo{configs: configs}})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return service
}

// TestSystemConfigValueFallsBack keeps the safe default when the shop has not
// changed a setting yet.
func TestSystemConfigValueFallsBack(t *testing.T) {
	service := newConfigService(t, nil)
	if got := service.SystemConfigValue(context.Background(), "ticket", "ticket_no_prefix", "TK"); got != "TK" {
		t.Fatalf("fallback = %q, want TK", got)
	}
	if got := service.SystemConfigInt(context.Background(), "ticket", "human_timeout_hours", 24); got != 24 {
		t.Fatalf("int fallback = %d, want 24", got)
	}
	if got := service.SystemConfigBool(context.Background(), "ticket", "allow_reopen", true); !got {
		t.Fatal("bool fallback = false, want true")
	}
}

// TestSystemConfigValueReadsSavedValue is the point of the config centre: what
// the shop saves is what the runtime uses.
func TestSystemConfigValueReadsSavedValue(t *testing.T) {
	configs := []domain.SystemConfig{
		{Base: models.Base{ID: 1}, Group: "ticket", Key: "ticket_no_prefix", Value: "SHOP", ValueType: "string"},
		{Base: models.Base{ID: 2}, Group: "ticket", Key: "human_timeout_hours", Value: "4", ValueType: "int"},
		{Base: models.Base{ID: 3}, Group: "ticket", Key: "allow_reopen", Value: "false", ValueType: "bool"},
		{Base: models.Base{ID: 4}, Group: "ticket", Key: "enable_rating", Value: "off", ValueType: "bool"},
	}
	service := newConfigService(t, configs)
	if got := service.SystemConfigValue(context.Background(), "ticket", "ticket_no_prefix", "TK"); got != "SHOP" {
		t.Fatalf("prefix = %q, want SHOP", got)
	}
	if got := service.SystemConfigInt(context.Background(), "ticket", "human_timeout_hours", 24); got != 4 {
		t.Fatalf("timeout = %d, want 4", got)
	}
	if service.SystemConfigBool(context.Background(), "ticket", "allow_reopen", true) {
		t.Fatal("allow_reopen = true, want false")
	}
	if service.SystemConfigBool(context.Background(), "ticket", "enable_rating", true) {
		t.Fatal("enable_rating = true, want false")
	}
}

// TestSystemConfigBoolRejectsGarbage treats an unparsable value as off rather
// than silently leaving a risky feature on.
func TestSystemConfigBoolRejectsGarbage(t *testing.T) {
	service := newConfigService(t, []domain.SystemConfig{
		{Base: models.Base{ID: 1}, Group: "risk", Key: "require_second_confirm", Value: "maybe", ValueType: "bool"},
	})
	if service.SystemConfigBool(context.Background(), "risk", "require_second_confirm", true) {
		t.Fatal("garbage value was treated as enabled")
	}
}
