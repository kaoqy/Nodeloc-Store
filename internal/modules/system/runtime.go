// Package system implements the in-app initialization wizard and the
// DB-backed runtime settings. All mutable configuration lives in the
// AppSetting table; the only on-disk artifact is an auto-written bootstrap
// file holding the database location, which users never edit by hand.
package system

import (
	"regexp"
	"strings"

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

	// Footer copy is the shop owner's, not the code's: 备案号, contact details
	// and a handful of links are what a storefront legally needs to show, and
	// none of it should require a redeploy.
	FooterText string `json:"footer_text"`
	FooterNote string `json:"footer_note"`
	// FooterLinks render left-to-right in the storefront footer.
	FooterLinks []FooterLink `json:"footer_links"`
	// Announcement is the one-line banner on the storefront home page.
	Announcement string `json:"announcement"`
}

// FooterLink is one customizable link in the storefront footer.
type FooterLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
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
	// The new switches are pointers so that a settings document written before
	// they existed keeps its old meaning. Absent means on, because a shop that
	// never opted out of 签到 or 优惠码 should not lose them on upgrade.
	Checkin             *bool `json:"enabled_checkin,omitempty"`
	Coupons             *bool `json:"enabled_coupons,omitempty"`
	StockAlertThreshold *int  `json:"stock_alert_threshold,omitempty"`
}

// CheckinEnabled reports whether the daily check-in is live for buyers.
func (f FeaturesConfig) CheckinEnabled() bool { return f.Checkin == nil || *f.Checkin }

// CouponsEnabled reports whether codes may be spent at checkout.
func (f FeaturesConfig) CouponsEnabled() bool { return f.Coupons == nil || *f.Coupons }

// AlertThreshold is how little stock still counts as 库存告急 on the dashboard.
func (f FeaturesConfig) AlertThreshold() int {
	if f.StockAlertThreshold != nil && *f.StockAlertThreshold >= 0 {
		return *f.StockAlertThreshold
	}
	return 5
}

type ThemeConfig struct {
	Primary string `json:"theme_primary"`
	Locale  string `json:"default_locale"`
}

// defaultAccent is the wizard's starting swatch, and what a malformed stored
// colour falls back to.
const defaultAccent = "#f2704a"

// hexColor is deliberately stricter than CSS: no #rgb shorthand, no colour
// names. The value lands inside a style sheet, so the narrower the gate the
// less room a typo has to turn into a declaration.
var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// defaultLocale is what a storefront with no opinion about language says, and
// what a malformed stored tag falls back to.
const defaultLocale = "zh-CN"

// localeTag accepts a plain BCP 47 primary tag with optional subtags, which is
// everything the settings page offers and nothing that can smuggle markup into
// the document element's lang attribute.
var localeTag = regexp.MustCompile(`^[a-zA-Z]{2,3}(-[A-Za-z0-9]{2,8})*$`)

// Redacted is returned in place of secret values; sending it back keeps the
// stored value untouched.
const Redacted = "********"

func Default() *RuntimeConfig {
	yes := true
	threshold := 5
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
		Features: FeaturesConfig{RegistrationEnabled: true, Checkin: &yes, Coupons: &yes, StockAlertThreshold: &threshold},
		Theme:    ThemeConfig{Primary: defaultAccent, Locale: defaultLocale},
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
	if r.Features.Checkin == nil {
		r.Features.Checkin = d.Features.Checkin
	}
	if r.Features.Coupons == nil {
		r.Features.Coupons = d.Features.Coupons
	}
	if r.Features.StockAlertThreshold == nil {
		r.Features.StockAlertThreshold = d.Features.StockAlertThreshold
	}
}

// Normalize makes the free-text site copy safe to render and bounded in size.
// It runs on load and on save, so a hand-edited settings row can never put an
// arbitrary script or a 100-link footer on the storefront.
func (r *RuntimeConfig) Normalize() {
	r.App.Name = trimRunes(r.App.Name, 120)
	r.App.Slogan = trimRunes(r.App.Slogan, 160)
	r.App.Description = trimRunes(r.App.Description, 500)
	r.App.FooterText = trimRunes(r.App.FooterText, 300)
	r.App.FooterNote = trimRunes(r.App.FooterNote, 120)
	r.App.Announcement = trimRunes(r.App.Announcement, 200)

	primary := strings.TrimSpace(r.Theme.Primary)
	if !hexColor.MatchString(primary) {
		primary = defaultAccent
	}
	// The storefront writes this straight into a CSS custom property, so it is
	// the one setting that reaches the browser as style rather than as text.
	r.Theme.Primary = strings.ToLower(primary)

	locale := strings.TrimSpace(r.Theme.Locale)
	if !localeTag.MatchString(locale) {
		locale = defaultLocale
	}
	r.Theme.Locale = locale

	links := make([]FooterLink, 0, len(r.App.FooterLinks))
	for _, link := range r.App.FooterLinks {
		label := trimRunes(link.Label, 40)
		target := trimRunes(link.URL, 300)
		if label == "" || !isSafeFooterURL(target) {
			continue
		}
		links = append(links, FooterLink{Label: label, URL: target})
		if len(links) == 8 {
			break
		}
	}
	r.App.FooterLinks = links
}

// isSafeFooterURL accepts only what a browser can be told to visit: an absolute
// http(s) address, or a relative path on this very storefront. That rules out
// javascript:, data: and the rest of the schemes an editor could be tricked
// into pasting.
func isSafeFooterURL(value string) bool {
	switch {
	case value == "":
		return false
	case strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//"):
		return true
	case strings.HasPrefix(value, "https://"), strings.HasPrefix(value, "http://"):
		return true
	}
	return false
}

func trimRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
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
	// Feature switches reach the modules through the process config, which is
	// rebuilt from these settings on every save.
	cfg.Features.CheckinDisabled = !r.Features.CheckinEnabled()
	cfg.Features.CouponsDisabled = !r.Features.CouponsEnabled()
	cfg.Features.StockAlertThreshold = r.Features.AlertThreshold()
}
