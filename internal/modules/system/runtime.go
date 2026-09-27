// Package system implements the in-app initialization wizard and the
// DB-backed runtime settings. All mutable configuration lives in the
// AppSetting table; the only on-disk artifact is an auto-written bootstrap
// file holding the database location, which users never edit by hand.
package system

import (
	"github.com/kaoqy/Nodeloc-Store/internal/config"
)

// RuntimeConfig is the full user-facing configuration, editable later from
// the admin settings page.
type RuntimeConfig struct {
	App      AppConfig      `json:"app"`
	OAuth    OAuthConfig    `json:"oauth"`
	Payment  PaymentConfig  `json:"payment"`
	Features FeaturesConfig `json:"features"`
	Theme    ThemeConfig    `json:"theme"`
}

type AppConfig struct {
	Name        string `json:"site_name"`
	Slogan      string `json:"site_slogan"`
	Description string `json:"site_description"`
	Logo        string `json:"site_logo"`
	Scheme      string `json:"scheme"`
	Domain      string `json:"domain"`
}

type OAuthConfig struct {
	Enabled      bool   `json:"enabled"`
	BaseURL      string `json:"base_url"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURI  string `json:"redirect_uri"`
	Scopes       string `json:"scopes"`
}

type PaymentConfig struct {
	Enabled   bool   `json:"enabled"`
	PaymentID string `json:"payment_id"`
	// Token (tk_xxx) signs 下单/转账 requests; SecretKey signs 查单 and
	// verifies the payment callback. They are two different credentials.
	Token     string `json:"token"`
	SecretKey string `json:"secret_key"`
}

type FeaturesConfig struct {
	RegistrationEnabled bool `json:"enabled_registration"`
}

type ThemeConfig struct {
	Primary string `json:"theme_primary"`
	Locale  string `json:"default_locale"`
}

// Redacted is returned in place of secret values; sending it back keeps the
// stored value untouched.
const Redacted = "********"

func Default() *RuntimeConfig {
	return &RuntimeConfig{
		App: AppConfig{
			Name:   "Nodeloc Store",
			Slogan: "数字商品交易平台",
			Scheme: "https",
		},
		OAuth: OAuthConfig{
			Enabled: true,
			BaseURL: "https://www.nodeloc.com",
			Scopes:  "openid profile",
		},
		Payment:  PaymentConfig{Enabled: true},
		Features: FeaturesConfig{RegistrationEnabled: true},
		Theme:    ThemeConfig{Primary: "#f2704a", Locale: "zh-CN"},
	}
}

// MergeDefaults fills empty fields from defaults, keeping stored values.
func (r *RuntimeConfig) MergeDefaults() {
	d := Default()
	if r.App.Name == "" {
		r.App.Name = d.App.Name
	}
	if r.App.Scheme == "" {
		r.App.Scheme = d.App.Scheme
	}
	if r.OAuth.BaseURL == "" {
		r.OAuth.BaseURL = d.OAuth.BaseURL
	}
	if r.OAuth.Scopes == "" {
		r.OAuth.Scopes = d.OAuth.Scopes
	}
	if r.Theme.Primary == "" {
		r.Theme.Primary = d.Theme.Primary
	}
	if r.Theme.Locale == "" {
		r.Theme.Locale = d.Theme.Locale
	}
}

// RedirectURI resolves the OAuth callback URL, deriving it from the site
// domain when it was not set explicitly.
func (r *RuntimeConfig) RedirectURI() string {
	if r.OAuth.RedirectURI != "" {
		return r.OAuth.RedirectURI
	}
	scheme := r.App.Scheme
	if scheme == "" {
		scheme = "https"
	}
	if r.App.Domain == "" {
		return ""
	}
	return scheme + "://" + r.App.Domain + "/api/v1/auth/oauth/callback"
}

// ApplyTo overlays the runtime settings onto the process config so every
// module wires itself from what the wizard stored, not from a hand-edited yml.
func (r *RuntimeConfig) ApplyTo(cfg *config.Config) {
	if r.App.Name != "" {
		cfg.App.Name = r.App.Name
	}
	if r.App.Slogan != "" {
		cfg.App.Slogan = r.App.Slogan
	}
	if r.App.Domain != "" {
		cfg.App.Domain = r.App.Domain
	}
	if r.App.Scheme != "" {
		cfg.App.Scheme = r.App.Scheme
	}
	cfg.App.BaseURL = ""
	if r.App.Domain != "" {
		cfg.App.BaseURL = cfg.GetBaseURL()
	}
	if r.OAuth.BaseURL != "" {
		cfg.NodeLoc.BaseURL = r.OAuth.BaseURL
	}
	if r.OAuth.ClientID != "" {
		cfg.NodeLoc.ClientID = r.OAuth.ClientID
	}
	if r.OAuth.ClientSecret != "" {
		cfg.NodeLoc.ClientSecret = r.OAuth.ClientSecret
	}
	if r.OAuth.Scopes != "" {
		cfg.NodeLoc.Scopes = r.OAuth.Scopes
	}
	if redirect := r.RedirectURI(); redirect != "" {
		cfg.NodeLoc.RedirectURI = redirect
	}
	if r.Payment.PaymentID != "" {
		cfg.NodeLoc.PaymentID = r.Payment.PaymentID
	}
	if r.Payment.Token != "" {
		cfg.NodeLoc.PaymentToken = r.Payment.Token
	}
	if r.Payment.SecretKey != "" {
		cfg.NodeLoc.PaymentSecret = r.Payment.SecretKey
	}
}
