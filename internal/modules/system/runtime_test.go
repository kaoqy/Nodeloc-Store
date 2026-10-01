package system

import (
	"context"
	"strings"
	"testing"
)

// A credential pasted out of NodeLoc's console arrives quoted half the time, and
// one stray quote is a different HMAC key on every payment call — which NodeLoc
// reports as a signature problem, sending the owner to replace a key that was
// always right. The artefacts come off on load and on save.
func TestNormalizeStripsPasteArtefactsFromCredentials(t *testing.T) {
	runtime := Default()
	runtime.Payment.PaymentID = "`pay_shop`"
	runtime.Payment.Token = "  \"tk_shop\"  "
	runtime.Payment.SecretKey = "'sk_shop'"
	runtime.OAuth.ClientID = `"client_id"`
	runtime.OAuth.ClientSecret = "'client_secret'"
	runtime.Normalize()

	for name, value := range map[string]string{
		"payment id": runtime.Payment.PaymentID,
		"token":      runtime.Payment.Token,
		"secret":     runtime.Payment.SecretKey,
		"client id":  runtime.OAuth.ClientID,
		"client sec": runtime.OAuth.ClientSecret,
	} {
		if strings.ContainsAny(value, "\"'` ") {
			t.Fatalf("%s kept paste artefacts: %q", name, value)
		}
	}
	if runtime.Payment.Token != "tk_shop" || runtime.Payment.PaymentID != "pay_shop" {
		t.Fatalf("credentials were cut too much or too little: %+v", runtime.Payment)
	}
}

// Payment ID and Client ID, Token and Secret Key: the two pairs on 设置 look alike,
// sit side by side, and each swap stops the shop from taking money with an error
// that names none of them. The store reads the shapes back.
func TestPaymentWarningsNameSwappedCredentials(t *testing.T) {
	runtime := Default()
	runtime.OAuth.ClientID = "client-abc"
	runtime.Payment.PaymentID = "client-abc"
	runtime.Payment.Token = "tk_shop"
	runtime.Payment.SecretKey = "tk_shop"
	warnings := strings.Join(runtime.PaymentWarnings(), "\n")
	if !strings.Contains(warnings, "Client ID") {
		t.Fatalf("a Payment ID copied from the OAuth app was not called out: %q", warnings)
	}
	if !strings.Contains(warnings, "Secret Key") {
		t.Fatalf("Token and Secret Key filled with the same value were not called out: %q", warnings)
	}

	clean := Default()
	clean.OAuth.ClientID = "client-abc"
	clean.Payment.PaymentID = "pay_shop"
	clean.Payment.Token = "tk_shop"
	clean.Payment.SecretKey = "sK9d2jf0dl"
	if got := clean.PaymentWarnings(); len(got) != 0 {
		t.Fatalf("a normal setup was warned about: %v", got)
	}

	// A key that was never really saved is the one case where 「已填写」 lies, so it
	// is said out loud rather than left for the gateway to refuse.
	masked := Default()
	masked.Payment.PaymentID = "pay_shop"
	masked.Payment.Token = Redacted
	if got := masked.PaymentWarnings(); len(got) == 0 || !strings.Contains(got[0], Redacted) {
		t.Fatalf("the mask placeholder was not reported: %v", got)
	}
}

// 「测试支付网关」 has to tell eight failures apart, and the copy is written here
// rather than read out of the payment module's error strings.
func TestPaymentProbeReportsEachProviderAnswer(t *testing.T) {
	cases := []struct {
		name          string
		probe         PaymentProbe
		wantsOK       bool
		wantsFragment string
		plainContains string
		notContains   string
	}{
		{name: "accepted", probe: PaymentProbe{}, wantsOK: true, wantsFragment: "已接受商店的签名"},
		{name: "unknown payment id", probe: PaymentProbe{Code: "payment_id_unknown"}, wantsFragment: "找不到后台填写的 Payment ID"},
		{name: "guarded query", probe: PaymentProbe{Code: "provider_guarded"}, wantsOK: true, wantsFragment: "只接受论坛后台的浏览器会话"},
		// A store that has never taken money has no order id NodeLoc remembers, so the
		// probe cannot prove its credentials — and must not claim it did.
		{name: "nothing to replay", probe: PaymentProbe{Code: "not_verified", Message: "商店还没有任何一单在 NodeLoc 留下交易号"},
			wantsFragment: "还没有测出下单凭据"},
		{name: "drifted clock", probe: PaymentProbe{Code: "provider_clock"}, wantsFragment: "同步时钟"},
		{name: "rejected signature", probe: PaymentProbe{Code: "provider_rejected"}, wantsFragment: "Payment Token 与 Secret Key"},
		{name: "unknown transaction", probe: PaymentProbe{Code: "not_found"}, wantsOK: true, wantsFragment: "本就不存在"},
		{name: "missing field", probe: PaymentProbe{Code: "not_configured", Detail: "payment is not configured on this store：缺少 token"},
			wantsFragment: "缺少 Payment Token"},
		{name: "unreachable", probe: PaymentProbe{Code: "provider_unreachable", Detail: "dial tcp: no such host"}, wantsFragment: "no such host"},
	}
	for _, item := range cases {
		service := NewService("", 0)
		service.Attach(nil, nil, func(context.Context) PaymentProbe { return item.probe })
		ok, msg := service.TestPayment(context.Background())
		if ok != item.wantsOK {
			t.Fatalf("%s: ok = %v, want %v (%s)", item.name, ok, item.wantsOK, msg)
		}
		if !strings.Contains(msg, item.wantsFragment) {
			t.Fatalf("%s: msg = %q, want it to contain %q", item.name, msg, item.wantsFragment)
		}
		// A probe result always says which convention the provider took, when the
		// store had to find out.
		probe := item.probe
		probe.Style = "下单：Secret Key 的 SHA-256、不带时间戳"
		service = NewService("", 0)
		service.Attach(nil, nil, func(context.Context) PaymentProbe { return probe })
		if _, msg := service.TestPayment(context.Background()); !strings.Contains(msg, "本次实测：下单：Secret Key") {
			t.Fatalf("%s: the signing style was not reported: %q", item.name, msg)
		}
	}
}

// The readiness fields travel together on one document: 缺哪几项 is what turns the
// switch's 「已启用」 into 「还收不了款」, and a warning list must stay empty rather
// than nil for the page that reads it.
func TestPaymentMissingAndWarningsTravelTogether(t *testing.T) {
	runtime := Default()
	runtime.Payment.PaymentID = "`pay_shop`"
	runtime.Payment.Token = "\"tk_shop\""
	runtime.Payment.SecretKey = "sk_shop"
	runtime.Normalize()
	if missing := runtime.Payment.MissingCredentials(); len(missing) != 0 {
		t.Fatalf("a complete setup reported %v missing", missing)
	}
	if warnings := runtime.PaymentWarnings(); warnings == nil || len(warnings) != 0 {
		t.Fatalf("a clean setup returned %v instead of an empty list", warnings)
	}

	emptied := Default()
	emptied.Payment.PaymentID = "pay_shop"
	emptied.Payment.Token = "tk_shop"
	emptied.Payment.SecretKey = "sk_shop"
	emptied.Payment.PaymentID = ""
	if missing := emptied.Payment.MissingCredentials(); len(missing) != 1 || missing[0] != "payment_id" {
		t.Fatalf("an empty Payment ID reported %v", missing)
	}
}

// NodeLoc has shipped payment applications with one secret and applications with
// two, and either box is enough for the store to sign with. Reading 「Token 空着」
// as 未配置 is how a working single key became 「无法支付」 for every buyer.
func TestOneSecretCountsAsConfigured(t *testing.T) {
	secretOnly := Default()
	secretOnly.Payment.PaymentID = "pay_shop"
	secretOnly.Payment.SecretKey = "sk_shop"
	if missing := secretOnly.Payment.MissingCredentials(); len(missing) != 0 {
		t.Fatalf("a Secret Key alone reported %v missing", missing)
	}

	tokenOnly := Default()
	tokenOnly.Payment.PaymentID = "pay_shop"
	tokenOnly.Payment.Token = "tk_shop"
	if missing := tokenOnly.Payment.MissingCredentials(); len(missing) != 0 {
		t.Fatalf("a Payment Token alone reported %v missing", missing)
	}

	neither := Default()
	neither.Payment.PaymentID = "pay_shop"
	neither.Payment.Token = ""
	neither.Payment.SecretKey = ""
	missing := neither.Payment.MissingCredentials()
	if len(missing) != 2 || missing[0] != "token" || missing[1] != "secret_key" {
		t.Fatalf("neither secret reported %v missing, want both names", missing)
	}
	if got := describePaymentMissing("未配置：缺少 token、secret_key"); !strings.Contains(got, "任意一个") {
		t.Fatalf("the two-empty diagnosis reads %q, want it to offer one box", got)
	}
	if got := describePaymentMissing("未配置：缺少 payment_id"); got != "Payment ID" {
		t.Fatalf("a single missing field reads %q", got)
	}
}

// 「测试 NodeLoc 登录」 used to only build an authorization URL, which the live forum
// answers with its own login redirect no matter what Client ID the store sent — so a
// reset Secret read as 「配置正确」 and each buyer then bounced back to the login page.
// Now the settings page reads the token endpoint's answer, and each of those answers has
// to name the field the owner can actually change.
func TestOAuthProbeReportsEachTokenEndpointAnswer(t *testing.T) {
	cases := []struct {
		name          string
		probe         OAuthProbe
		wantsOK       bool
		wantsFragment string
	}{
		{name: "凭据被认", probe: OAuthProbe{}, wantsOK: true, wantsFragment: "认这组 Client ID 与 Client Secret"},
		{name: "配对不上", probe: OAuthProbe{Code: "client_rejected",
			Detail: `HTTP 400：{"error":"invalid_client","error_description":"Invalid client credentials"}`},
			wantsFragment: "对不上这个 OAuth 应用"},
		{name: "这个域没有 OAuth", probe: OAuthProbe{Code: "route_missing", Detail: "HTTP 404：论坛回了网页"},
			wantsFragment: "填成论坛本体"},
		{name: "回调对不上", probe: OAuthProbe{Code: "redirect_mismatch", Detail: "HTTP 400：redirect_uri mismatch"},
			wantsFragment: "站点域名"},
		{name: "授权方式不被允许", probe: OAuthProbe{Code: "grant_unsupported", Detail: "HTTP 400：unsupported_grant_type"},
			wantsFragment: "authorization code flow"},
		{name: "scope 没过审", probe: OAuthProbe{Code: "scope_rejected", Detail: "HTTP 400：invalid_scope"},
			wantsFragment: "openid"},
		{name: "连不上", probe: OAuthProbe{Code: "unreachable", Detail: "dial tcp: no such host"},
			wantsFragment: "no such host"},
		// An empty-field answer is the store's own field names, never the English
		// sentinel the provider logs.
		{name: "没配齐", probe: OAuthProbe{Code: "not_configured", Detail: "NodeLoc 登录还没有配置完整：缺少 Client Secret"},
			wantsFragment: "请补齐后台设置里的 Client Secret"},
		{name: "读不懂的应答", probe: OAuthProbe{Code: "rejected", Detail: "HTTP 502：gateway gave up"},
			wantsFragment: "gateway gave up"},
	}
	for _, item := range cases {
		outcome := item.probe
		service := NewService("", 0)
		service.Attach(nil, func(context.Context) OAuthProbe { return outcome }, nil)
		ok, _, msg := service.TestOAuth(context.Background())
		if ok != item.wantsOK {
			t.Fatalf("%s: ok = %v, want %v (%s)", item.name, ok, item.wantsOK, msg)
		}
		if !strings.Contains(msg, item.wantsFragment) {
			t.Fatalf("%s: msg = %q, want it to contain %q", item.name, msg, item.wantsFragment)
		}
		if strings.Contains(msg, "configuration is incomplete") {
			t.Fatalf("%s: the English sentinel reached the 设置 page: %q", item.name, msg)
		}
	}
}

// The settings page also offers the authorize link as a next step, so a probe with
// complete fields hands it back either way — including when NodeLoc refused, because the
// owner's next move is to try a login in a browser.
func TestOAuthProbeKeepsTheAuthorizeLink(t *testing.T) {
	service := NewService("", 0)
	service.Attach(nil, func(context.Context) OAuthProbe {
		return OAuthProbe{Code: "client_rejected", AuthorizeURL: "https://www.nodeloc.com/oauth-provider/authorize?client_id=x"}
	}, nil)
	ok, link, msg := service.TestOAuth(context.Background())
	if ok || link != "https://www.nodeloc.com/oauth-provider/authorize?client_id=x" {
		t.Fatalf("ok=%v link=%q msg=%q", ok, link, msg)
	}
	if !strings.Contains(msg, "授权链接已生成") {
		t.Fatalf("the link was not announced: %q", msg)
	}

	bare := NewService("", 0)
	if ok, _, msg := bare.TestOAuth(context.Background()); ok || !strings.Contains(msg, "尚未初始化") {
		t.Fatalf("before install: ok=%v msg=%q", ok, msg)
	}
}

// 站点域名 is the one field a shop owner pastes with its scheme still attached.
// Kept verbatim it doubles up in the OAuth redirect_uri
// (https://https://shop.example.com//api/v1/…) and NodeLoc refuses the login,
// while every local test that does not validate redirect_uri stays green.
func TestNormalizeTurnsTheDomainIntoAHost(t *testing.T) {
	cases := []struct {
		typed string
		want  string
	}{
		{"https://Shop.Example.com/", "shop.example.com"},
		{"http://shop.example.com", "shop.example.com"},
		{"shop.example.com/store/", "shop.example.com"},
		{"shop.example.com:8080", "shop.example.com:8080"},
		{"  shop.example.com  ", "shop.example.com"},
		{"", ""},
	}
	for _, testCase := range cases {
		runtime := Default()
		runtime.App.Domain = testCase.typed
		runtime.OAuth.RedirectURI = ""
		runtime.Normalize()
		if runtime.App.Domain != testCase.want {
			t.Errorf("domain %q normalized to %q, want %q", testCase.typed, runtime.App.Domain, testCase.want)
		}
		if testCase.want == "" {
			continue
		}
		wantRedirect := runtime.App.Scheme + "://" + testCase.want + "/api/v1/auth/oauth/callback"
		if got := runtime.RedirectURI(); got != wantRedirect {
			t.Errorf("redirect_uri for %q = %q, want %q", testCase.typed, got, wantRedirect)
		}
	}
}

// A 回调地址 typed without its scheme is a value the provider will never match,
// so it is completed from the store's own scheme rather than sent as-is.
func TestNormalizeCompletesAHandTypedCallback(t *testing.T) {
	runtime := Default()
	runtime.App.Scheme = "http"
	runtime.App.Domain = "shop.example.com"
	runtime.OAuth.RedirectURI = "shop.example.com/api/v1/auth/oauth/callback"
	runtime.Normalize()

	if got := runtime.RedirectURI(); got != "http://shop.example.com/api/v1/auth/oauth/callback" {
		t.Errorf("redirect_uri = %q, want the typed path made absolute on http", got)
	}

	runtime.OAuth.RedirectURI = "not a url at all"
	runtime.Normalize()
	if runtime.OAuth.RedirectURI != "" {
		t.Errorf("a callback that is not a URL stayed as %q, want it dropped so the domain derives it", runtime.OAuth.RedirectURI)
	}
}

// 设置 has to say which box is still empty (missing) and which one is filled but
// cannot work (warnings), because 「登录不了」 from a buyer names neither — and a
// 重定向 URI on the wrong host is refused by NodeLoc before the shop ever sees a code.
func TestOAuthDiagnosticsReadTheLoginSettings(t *testing.T) {
	named := func(names []string, want string) bool {
		for _, name := range names {
			if name == want {
				return true
			}
		}
		return false
	}

	runtime := Default()
	runtime.App.Domain = ""
	runtime.OAuth.RedirectURI = ""
	runtime.OAuth.ClientID = ""
	runtime.OAuth.ClientSecret = Redacted
	missing := runtime.OAuthMissing()
	if !named(missing, "client_id") || !named(missing, "client_secret") || !named(missing, "domain") {
		t.Errorf("missing = %v, want client_id, client_secret and 站点域名 named", missing)
	}

	runtime.App.Domain = "shop.example.com"
	runtime.OAuth.ClientID = "abc123"
	runtime.OAuth.ClientSecret = "s3cret"
	if got := runtime.OAuthMissing(); len(got) != 0 {
		t.Errorf("a complete login config reports missing %v", got)
	}
	if got := runtime.OAuthWarnings(); len(got) != 0 {
		t.Errorf("the derived callback warned %v, want silence about a field left empty on purpose", got)
	}

	runtime.OAuth.RedirectURI = "http://elsewhere.example.com" + oauthCallbackPath
	warnings := runtime.OAuthWarnings()
	if len(warnings) == 0 || !strings.Contains(warnings[0], "域名") {
		t.Errorf("a callback on another host warned %v, want the host named", warnings)
	}

	runtime.OAuth.RedirectURI = "http://shop.example.com/login"
	warnings = runtime.OAuthWarnings()
	if len(warnings) == 0 || !strings.Contains(warnings[0], "路径") {
		t.Errorf("a callback pointed at a page warned %v, want the path named", warnings)
	}

	// The same mistake in the other direction: the payment application's number
	// pasted into the login field.
	runtime.OAuth.ClientID = "pay_0f2c1b"
	runtime.OAuth.RedirectURI = ""
	warnings = runtime.OAuthWarnings()
	if len(warnings) == 0 || !strings.Contains(warnings[0], "Payment ID") {
		t.Errorf("a pay_ Client ID warned %v, want it read as the payment credential", warnings)
	}
}
