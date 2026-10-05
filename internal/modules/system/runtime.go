// Package system implements the in-app initialization wizard and the
// DB-backed runtime settings. All mutable configuration lives in the
// AppSetting table; the only on-disk artifact is an auto-written bootstrap
// file holding the database location, which users never edit by hand.
package system

import (
	"math/big"
	"net"
	"net/url"
	"regexp"
	"strings"

	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/shared"
)

// RuntimeConfig is the full user-facing configuration, editable later from
// the admin settings page.
type RuntimeConfig struct {
	App      AppConfig      `json:"app"`
	OAuth    OAuthConfig    `json:"oauth"`
	Payment  PaymentConfig  `json:"payment"`
	NewAPI   NewAPIConfig   `json:"new_api"`
	SMTP     SMTPConfig     `json:"smtp"`
	Features FeaturesConfig `json:"features"`
	Theme    ThemeConfig    `json:"theme"`
}

// NewAPIConfig configures the New-API redemption-code delivery channel. The
// store creates a redemption code after payment; it does not credit a New-API
// account directly, so buyers are told to redeem the code themselves.
type NewAPIConfig struct {
	Enabled *bool `json:"enabled"`
	// BaseURL is the New-API site root. The adapter appends /api/redemption/.
	BaseURL string `json:"base_url"`
	// AdminAccessToken is write-only and masked on read.
	AdminAccessToken string `json:"admin_access_token"`
	AdminUserID      string `json:"admin_user_id"`
	// NLToUSD is how many US dollars one paid NL is worth (e.g. "1" means
	// 1 NL = 1 USD). The upstream redemption quota is
	//   paid NL × NLToUSD × 500000
	// because the upstream counts 500000 quota per USD. It is deliberately a
	// decimal string so the rate keeps its precision without float drift.
	NLToUSD string `json:"nl_usd_rate"`
}

func (n NewAPIConfig) On() bool { return n.Enabled != nil && *n.Enabled }

// ValidRate reports whether NLToUSD parses as a finite number greater than zero.
// The quota maths lives in the provider, but the settings save still has to
// refuse an unusable rate rather than store one the provider will reject later.
func (n NewAPIConfig) ValidRate() bool {
	value := strings.TrimSpace(n.NLToUSD)
	if value == "" {
		return false
	}
	parsed, ok := new(big.Rat).SetString(value)
	return ok && parsed.Sign() > 0
}

// MissingFields names what prevents the channel from working.
func (n NewAPIConfig) MissingFields() []string {
	missing := make([]string, 0, 5)
	if strings.TrimSpace(n.BaseURL) == "" {
		missing = append(missing, "base_url")
	}
	if strings.TrimSpace(n.AdminAccessToken) == "" {
		missing = append(missing, "admin_access_token")
	}
	if strings.TrimSpace(n.AdminUserID) == "" {
		missing = append(missing, "admin_user_id")
	}
	if strings.TrimSpace(n.NLToUSD) == "" {
		missing = append(missing, "nl_usd_rate")
	}
	return missing
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
	// Enabled is a pointer for the same reason the 签到/优惠码 switches are: an
	// absent key means 开, so a settings document written before the switch was
	// honoured does not switch NodeLoc 登录 off underneath the shop.
	Enabled      *bool  `json:"enabled"`
	BaseURL      string `json:"base_url"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURI  string `json:"redirect_uri"`
	Scopes       string `json:"scopes"`
}

func (o OAuthConfig) On() bool { return o.Enabled == nil || *o.Enabled }

type SMTPConfig struct {
	Enabled  *bool  `json:"enabled"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Secure   string `json:"secure"`
	From     string `json:"from"`
}

func (s SMTPConfig) On() bool { return s.Enabled != nil && *s.Enabled }

type PaymentConfig struct {
	Enabled   *bool  `json:"enabled"`
	PaymentID string `json:"payment_id"`
	// BaseURL is where 下单/查单/转账 go. Left empty they follow the OAuth host,
	// which is right for a shop with one NodeLoc domain and wrong for one that
	// logs in through a mirror — payments then leave for a host that has no
	// payment application at all, and every buyer sees 「无法支付」.
	BaseURL string `json:"base_url"`
	// Token (tk_xxx) signs 下单/转账 requests; SecretKey signs 查单 and
	// verifies the payment callback. They are two different credentials.
	Token     string `json:"token"`
	SecretKey string `json:"secret_key"`
}

// On reports whether the owner left NodeLoc 收款 switched on. Absent means on.
func (p PaymentConfig) On() bool { return p.Enabled == nil || *p.Enabled }

// MissingCredentials names the payment settings that have to be filled before a
// buyer can pay, in the order the shop owner reads them on the settings page.
// A half-configured gateway is the single most common reason a storefront
// refuses money, and "not configured" on its own says nothing about which field.
//
// Token and Secret Key are one credential the owner may only have one copy of.
// NodeLoc's own payment application has released both spellings over time, and the
// client that demonstrably took money on this forum signed with a single secret,
// so the store asks for either: two empty boxes are 配置 incomplete, one is not.
func (p PaymentConfig) MissingCredentials() []string {
	missing := make([]string, 0, 3)
	if strings.TrimSpace(p.PaymentID) == "" {
		missing = append(missing, "payment_id")
	}
	token := strings.TrimSpace(p.Token)
	secret := strings.TrimSpace(p.SecretKey)
	if token == "" && secret == "" {
		missing = append(missing, "token", "secret_key")
	}
	return missing
}

// PaymentWarnings names the settings shapes that make NodeLoc Payments fail in a
// way nobody can guess from 「无法支付」: two credentials swapped, an OAuth Client
// ID typed into the Payment ID box, a key that is still the mask placeholder. Each
// is a sentence to show on 设置, not a reason to refuse a save — a shop whose
// payment application really does use another format has to be able to keep it.
func (r *RuntimeConfig) PaymentWarnings() []string {
	warnings := make([]string, 0, 4)
	paymentID := r.Payment.PaymentID
	token := r.Payment.Token
	secret := r.Payment.SecretKey

	for _, item := range []struct {
		name  string
		value string
	}{{"Payment ID", paymentID}, {"Payment Token", token}, {"Secret Key", secret}} {
		if item.value == Redacted {
			warnings = append(warnings, item.name+" 里存的还是占位符 "+Redacted+"，这一项从未真正保存过密钥。请重新粘贴并保存，再点「测试支付网关」。")
		}
	}
	if token != "" && secret != "" && token == secret {
		warnings = append(warnings, "Payment Token 与 Secret Key 现在是同一串。NodeLoc 的支付应用只发一串密钥时这样填没问题，商店会依次按文档写的几种签名方式试；若只发两串而这里填成了同一串，回调验签会失败。")
	}
	if paymentID != "" && r.OAuth.ClientID != "" && paymentID == r.OAuth.ClientID {
		warnings = append(warnings, "Payment ID 与 OAuth 的 Client ID 填了同一串。登录用 Client ID，收款用支付应用编号（pay_xxx），两者混填会让每个买家都在下单时被 NodeLoc 拒绝。")
	}
	if paymentID != "" && paymentID != Redacted && !strings.HasPrefix(strings.ToLower(paymentID), "pay_") {
		warnings = append(warnings, "Payment ID 通常以 pay_ 开头（当前是 "+shortValue(paymentID)+"）。若你的支付应用编号确实不是这个格式可以忽略本条，否则请确认没有把 Token、Client ID 或应用数字 ID 填到这里。")
	}
	if token != "" && token != Redacted && !strings.HasPrefix(strings.ToLower(token), "tk_") {
		warnings = append(warnings, "Payment Token 通常以 tk_ 开头（当前是 "+shortValue(token)+"）。若 NodeLoc 只给了你一串商户密钥，把它填在 Secret Key 那一格更稳妥，商店同样能用它签名。")
	}
	return warnings
}

// OAuthMissing names the 登录 settings that have to be filled before a buyer can use
// 「用 NodeLoc 登录」, in the order the 设置 page shows them. A masked placeholder
// counts as empty: it is what the store hands back for a secret nobody ever saved.
func (r *RuntimeConfig) OAuthMissing() []string {
	missing := make([]string, 0, 3)
	if value := strings.TrimSpace(r.OAuth.ClientID); value == "" || value == Redacted {
		missing = append(missing, "client_id")
	}
	if value := strings.TrimSpace(r.OAuth.ClientSecret); value == "" || value == Redacted {
		missing = append(missing, "client_secret")
	}
	if r.RedirectURI() == "" {
		// The callback is derived from 站点域名, so an empty one is that box's
		// problem. 「重定向 URI 没填」 would send the owner to a field that is meant to
		// stay empty.
		missing = append(missing, "domain")
	}
	return missing
}

// OAuthWarnings names 登录 settings that are filled but cannot work. NodeLoc checks the
// redirect address against its own registry before the shop ever sees a code, so a
// host or a path that does not match the OAuth application refuses every buyer at
// 「用 NodeLoc 登录」 with copy that reads like a dead forum.
func (r *RuntimeConfig) OAuthWarnings() []string {
	warnings := make([]string, 0, 3)
	if id := strings.TrimSpace(r.OAuth.ClientID); id != "" && id != Redacted {
		if strings.HasPrefix(strings.ToLower(id), "pay_") {
			warnings = append(warnings, "Client ID 以 pay_ 开头，那通常是收款用的 Payment ID。登录要用 NodeLoc OAuth 应用给出的 Client ID，填错时每个买家都会在授权这一步被拒。")
		}
		if strings.ContainsAny(id, " \t\"'`") {
			warnings = append(warnings, "Client ID 里还带着空格或引号（当前是 "+shortValue(id)+"）。NodeLoc 按原文比对，多一个字符就认不出这组凭据。")
		}
	}
	redirect := r.RedirectURI()
	if redirect == "" {
		return warnings
	}
	parsed, err := url.Parse(redirect)
	if err != nil {
		return warnings
	}
	host := normalizeDomain(parsed.Host)
	domain := normalizeDomain(r.App.Domain)
	if host != "" && domain != "" && host != domain {
		warnings = append(warnings, "重定向 URI 的域名（"+host+"）与站点域名（"+domain+"）不是同一个。NodeLoc 只把买家送回 OAuth 应用里登记过的那个地址：请把两边改成一致，或让重定向 URI 留空，由商店按站点域名生成。")
	}
	if host == domain && parsed.Path != oauthCallbackPath {
		warnings = append(warnings, "重定向 URI 的路径是 "+parsed.Path+"，而本店的登录回调写在 "+oauthCallbackPath+"。地址对不上时 NodeLoc 会把买家送回一个接不住这个参数的页面，登录永远完不成。")
	}
	return warnings
}

// shortValue shows enough of a credential to recognise which field it is without
// putting the whole key in a page a shop owner might screenshot.
func shortValue(value string) string {
	runes := []rune(value)
	if len(runes) <= 12 {
		return string(runes)
	}
	return string(runes[:6]) + "…" + string(runes[len(runes)-3:])
}

type FeaturesConfig struct {
	// Every switch is a pointer so that a settings document written before it
	// existed keeps its old meaning. Absent means on, because a shop that never
	// opted out of 注册, 签到 or 优惠码 should not lose them on upgrade — and since
	// these now decide real behaviour instead of only the badge on 设置, losing one
	// silently would lock buyers out of a store that never asked for that.
	Registration        *bool `json:"enabled_registration,omitempty"`
	Checkin             *bool `json:"enabled_checkin,omitempty"`
	Coupons             *bool `json:"enabled_coupons,omitempty"`
	StockAlertThreshold *int  `json:"stock_alert_threshold,omitempty"`
}

// RegistrationEnabled reports whether a visitor may open a local account.
func (f FeaturesConfig) RegistrationEnabled() bool { return f.Registration == nil || *f.Registration }

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

// oauthCallbackPath is where the store finishes a NodeLoc 登录 round trip. It is
// one constant because the derived callback, the route and 设置's warning about a
// hand-typed address all have to name the same path.
const oauthCallbackPath = "/api/v1/auth/oauth/callback"

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
			Enabled: &yes,
			BaseURL: "https://www.nodeloc.com",
			Scopes:  "openid profile",
		},
		Payment:  PaymentConfig{Enabled: &yes},
		NewAPI:   NewAPIConfig{Enabled: new(bool)},
		SMTP:     SMTPConfig{Enabled: new(bool), Port: 587, Secure: "starttls"},
		Features: FeaturesConfig{Checkin: &yes, Coupons: &yes, StockAlertThreshold: &threshold},
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
	if r.NewAPI.Enabled == nil {
		r.NewAPI.Enabled = d.NewAPI.Enabled
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
	r.App.Logo = trimRunes(r.App.Logo, 500)
	r.App.FooterText = trimRunes(r.App.FooterText, 300)
	r.App.FooterNote = trimRunes(r.App.FooterNote, 120)
	r.App.Announcement = trimRunes(r.App.Announcement, 200)

	// The logo is an image source on every page of both interfaces, so a value
	// that is not an image address is dropped rather than loaded everywhere.
	if r.App.Logo != "" && !isSafeImageURL(r.App.Logo) {
		r.App.Logo = ""
	}

	primary := strings.TrimSpace(r.Theme.Primary)
	if !hexColor.MatchString(primary) {
		primary = defaultAccent
	}
	// The storefront writes this straight into a CSS custom property, so it is
	// the one setting that reaches the browser as style rather than as text.
	r.Theme.Primary = strings.ToLower(primary)

	// A payment host that is not a host would send 下单 to a nonsense URL, and
	// the buyer would see a store that cannot take money. Empty means "use the
	// OAuth host", which is what ApplyTo does with it.
	if base := strings.TrimSpace(r.Payment.BaseURL); !isProviderOrigin(base) {
		r.Payment.BaseURL = ""
	} else {
		r.Payment.BaseURL = strings.TrimRight(base, "/")
	}
	if oauth := strings.TrimSpace(r.OAuth.BaseURL); oauth != "" && isProviderOrigin(oauth) {
		r.OAuth.BaseURL = strings.TrimRight(oauth, "/")
	}
	r.NewAPI.BaseURL = normalizeNewAPIBaseURL(r.NewAPI.BaseURL)
	r.NewAPI.AdminAccessToken = shared.TrimCredential(r.NewAPI.AdminAccessToken)
	r.NewAPI.AdminUserID = strings.TrimSpace(r.NewAPI.AdminUserID)
	r.NewAPI.NLToUSD = strings.TrimSpace(r.NewAPI.NLToUSD)

	// The host the store's own URLs are built from. A 站点域名 pasted with its
	// scheme and a trailing slash would otherwise reach NodeLoc as
	// https://https://shop.example.com//api/v1/auth/oauth/callback, which the
	// forum must refuse — and only a real provider checks a redirect_uri against
	// a registry, so every local test would still call that configuration fine.
	r.App.Domain = normalizeDomain(r.App.Domain)
	r.OAuth.RedirectURI = normalizeCallbackURL(r.App.Scheme, r.OAuth.RedirectURI)

	// Credentials are signed over exactly the bytes NodeLoc issued, so the quote a
	// documentation snippet brings along with a paste — "tk_xxx" or `tk_xxx` —
	// breaks every payment call with an error that reads like a wrong key. The
	// paired wrappers come off here, on load and on save alike, so a hand-edited
	// settings row cannot smuggle one in either.
	r.SMTP.Host = strings.TrimSpace(r.SMTP.Host)
	r.SMTP.Username = strings.TrimSpace(r.SMTP.Username)
	r.SMTP.Secure = strings.ToLower(strings.TrimSpace(r.SMTP.Secure))
	if r.SMTP.Port == 0 {
		r.SMTP.Port = 587
	}
	if r.SMTP.Secure == "" {
		r.SMTP.Secure = "starttls"
	}
	if r.SMTP.From != "" {
		r.SMTP.From = strings.TrimSpace(r.SMTP.From)
	}
	r.Payment.PaymentID = shared.TrimCredential(r.Payment.PaymentID)
	r.Payment.Token = shared.TrimCredential(r.Payment.Token)
	r.Payment.SecretKey = shared.TrimCredential(r.Payment.SecretKey)
	r.OAuth.ClientID = shared.TrimCredential(r.OAuth.ClientID)
	r.OAuth.ClientSecret = shared.TrimCredential(r.OAuth.ClientSecret)

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

// isSafeImageURL accepts everything a footer link may be, plus an inline
// data: image, which is how a small logo arrives from a paste. A javascript: or
// file: address is not a picture, and the shop should not try to load one as its
// own face on every page.
func isSafeImageURL(value string) bool {
	if strings.HasPrefix(value, "data:image/") {
		return true
	}
	return isSafeFooterURL(value)
}

// isProviderOrigin accepts only what a NodeLoc API call can be sent to: an
// absolute http(s) URL with a host and no path, query or fragment hanging off
// it. The gateway appends its own paths, so anything more than an origin here
// would quietly produce "/login/payment/pay_xxx/process" style addresses.
func isProviderOrigin(value string) bool {
	if value == "" {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return false
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return false
	}
	return parsed.Path == "" || parsed.Path == "/"
}

func trimRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

// normalizeDomain turns whatever the shop owner typed into the authority the
// store's own URLs are built on: 「https://Shop.Example.com/」, 「shop.example.com/store」
// and 「shop.example.com:8080」 all become the lower-cased host, port included. A
// path is dropped because the store only ever serves from the root, so a
// sub-path could not be its own callback address anyway.
func normalizeDomain(in string) string {
	value := strings.ToLower(strings.TrimSpace(in))
	if value == "" {
		return ""
	}
	// url.Parse reads 「example.com:8080」 as a scheme unless it is given an
	// authority to work on.
	if !strings.Contains(value, "://") {
		value = "//" + value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		if index := strings.IndexByte(value, '/'); index >= 0 {
			value = value[:index]
		}
		if index := strings.Index(value, "://"); index >= 0 {
			value = value[index+3:]
		}
		return strings.Trim(value, "/")
	}
	return parsed.Host
}

// normalizeCallbackURL makes a hand-typed OAuth 回调地址 absolute. The store
// derives this URL from its own domain when the field is blank, so the only
// reason to fill it is a mirror or a proxy in front of the shop — and a value
// missing its scheme is refused by every real provider.
func normalizeCallbackURL(scheme string, in string) string {
	value := strings.TrimSpace(in)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		if scheme != "http" {
			scheme = "https"
		}
		value = scheme + "://" + strings.TrimLeft(value, "/")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	return strings.TrimRight(parsed.String(), "/")
}

// normalizeNewAPIBaseURL accepts only a plain HTTP(S) origin. The provider
// repeats this check at delivery time; doing it on save too means the shop
// owner sees the refusal while editing, not after a paid order.
func normalizeNewAPIBaseURL(in string) string {
	value := strings.TrimSpace(in)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" || isUnsafeNewAPIHost(host) {
		return ""
	}
	return strings.TrimRight(parsed.String(), "/")
}

func isUnsafeNewAPIHost(host string) bool {
	switch host {
	case "localhost", "localhost.localdomain", "metadata.google.internal",
		"169.254.169.254", "100.100.100.200":
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified()
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
	return scheme + "://" + r.App.Domain + oauthCallbackPath
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
	// Payments default to the OAuth host and can be pointed somewhere else: a
	// shop that signs people in through a mirror still has to send money
	// requests to the domain holding its payment application.
	if r.Payment.BaseURL != "" {
		cfg.NodeLoc.PaymentBaseURL = r.Payment.BaseURL
	} else {
		cfg.NodeLoc.PaymentBaseURL = cfg.NodeLoc.BaseURL
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
	cfg.Features.OAuthDisabled = !r.OAuth.On()
	cfg.Features.PaymentsDisabled = !r.Payment.On()
	cfg.Features.RegistrationDisabled = !r.Features.RegistrationEnabled()
}
