package application

import (
	"context"
	"errors"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/identity/domain"
)

// closedRepo is the whole identity store as far as this test is concerned. The
// embedded nil interface is deliberate: a shop that closed 注册 has to refuse
// before it looks up a username, and any read or write here panics loudly.
type closedRepo struct {
	contract.UserRepo
}

func newClosedService(t *testing.T, features config.FeaturesConfig) *Service {
	t.Helper()
	service, err := NewService(&closedRepo{}, stubProvider{}, stubTokens{}, features)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

func TestRegisterRefusesWhenTheShopClosedRegistration(t *testing.T) {
	service := newClosedService(t, config.FeaturesConfig{RegistrationDisabled: true})

	_, err := service.Register(context.Background(), RegisterInput{Username: "buyer", Password: "longenoughpw"})
	if !errors.Is(err, domain.ErrRegistrationDisabled) {
		t.Fatalf("register on a closed shop: want ErrRegistrationDisabled, got %v", err)
	}
}

func TestRegistrationStillOpenByDefaultAndForOldSettings(t *testing.T) {
	// The zero value is what a settings document written before the switch
	// existed unmarshals to, and it must not read as 「关闭」.
	for name, features := range map[string]config.FeaturesConfig{
		"zero value": {},
		"explicit":   {RegistrationDisabled: false},
	} {
		service := newClosedService(t, features)
		if !service.RegistrationEnabled() {
			t.Fatalf("%s: registration reads closed", name)
		}
		_, err := service.Register(context.Background(), RegisterInput{Username: "", Password: "x"})
		if errors.Is(err, domain.ErrRegistrationDisabled) {
			t.Fatalf("%s: an open shop refused the signup", name)
		}
	}
}
