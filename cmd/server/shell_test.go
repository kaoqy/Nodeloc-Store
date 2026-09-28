package main

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/system"
)

const builtShell = `<!doctype html>
<html lang="zh-CN" data-theme="dark">
  <head>
    <meta charset="UTF-8" />
    <link rel="icon" href="/favicon.svg" />
    <meta name="description" content="Nodeloc Store 数字商品平台" />
    <title>Nodeloc Store</title>
  </head>
  <body><div id="app"></div></body>
</html>`

func TestStampShellWritesTheShopIntoTheDocument(t *testing.T) {
	id := system.ShellIdentity{
		Name:        "网卡小店",
		Description: "秒发激活码，付款即到邮箱",
		Logo:        "/uploads/face.png",
		Locale:      "zh-TW",
	}

	page := stampShell(builtShell, id, "https://shop.example", false)

	for _, want := range []string{
		"<title>网卡小店</title>",
		`<meta name="description" content="秒发激活码，付款即到邮箱" />`,
		`<link rel="icon" href="/uploads/face.png" />`,
		`<html lang="zh-TW" data-theme="dark">`,
		`<meta property="og:title" content="网卡小店" />`,
		`<meta property="og:description" content="秒发激活码，付款即到邮箱" />`,
		`<meta property="og:image" content="https://shop.example/uploads/face.png" />`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("stamped page is missing %s\n%s", want, page)
		}
	}
	if n := strings.Count(page, "<title>"); n != 1 {
		t.Errorf("title stamped twice: %d copies", n)
	}
	if strings.Contains(page, "noindex") {
		t.Error("the storefront must stay indexable")
	}
}

func TestStampShellNamesTheBackOfficeAndHidesIt(t *testing.T) {
	page := stampShell(builtShell, system.ShellIdentity{Name: "网卡小店"}, "", true)

	if !strings.Contains(page, "<title>网卡小店 管理后台</title>") {
		t.Errorf("back office title not stamped: %s", page)
	}
	if !strings.Contains(page, `<meta name="robots" content="noindex, nofollow" />`) {
		t.Error("the control panel should never reach a search index")
	}
}

// A shop that left 网站描述 blank keeps the summary its build ships with, and one
// without a logo has no preview image to advertise — neither gets an empty tag.
func TestStampShellLeavesAloneWhatTheOwnerDidNotFillIn(t *testing.T) {
	page := stampShell(builtShell, system.ShellIdentity{Name: "网卡小店", Logo: "data:image/svg+xml,%3Csvg/%3E"}, "https://shop.example", false)

	if !strings.Contains(page, `content="Nodeloc Store 数字商品平台"`) {
		t.Errorf("blank summary overwrote the shipped one: %s", page)
	}
	if strings.Contains(page, "og:description") {
		t.Error("an empty summary should not become preview copy")
	}
	if strings.Contains(page, "og:image") {
		t.Error("a data: logo cannot be fetched by a crawler, so it is no preview image")
	}
	if !strings.Contains(page, `href="data:image/svg+xml,%3Csvg/%3E"`) {
		t.Error("the inline logo should still paint the tab")
	}
}

func TestStampShellAddsWhatTheBuildLeftOut(t *testing.T) {
	page := stampShell("<html><head></head><body></body></html>",
		system.ShellIdentity{Name: "网卡小店", Description: "秒发卡密", Logo: "/uploads/face.png", Locale: "en"},
		"https://shop.example", false)

	for _, want := range []string{
		`<html lang="en">`,
		"<title>网卡小店</title>",
		`<meta name="description" content="秒发卡密" />`,
		`<link rel="icon" href="/uploads/face.png" />`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("missing tag never added: %s\n%s", want, page)
		}
	}
	if !strings.Contains(page, `<meta property="og:title" content="网卡小店" />`) {
		t.Errorf("preview copy did not land either: %s", page)
	}
	if strings.Index(page, "<title>") > strings.Index(page, "</head>") {
		t.Errorf("added tags did not land in the head: %s", page)
	}
}

// The name is typed by a person and lands inside markup the browser parses, so a
// quote or a closing tag has to arrive as text.
func TestStampShellEscapesCopyThatLooksLikeMarkup(t *testing.T) {
	hostile := `</title><script>alert(1)</script>" onload=x`
	page := stampShell(builtShell, system.ShellIdentity{Name: hostile, Description: hostile}, "", false)

	if strings.Contains(page, "<script>alert(1)</script>") {
		t.Fatalf("script survived the stamp: %s", page)
	}
	if n := strings.Count(page, "<title>"); n != 1 {
		t.Errorf("the name broke out of the title: %d titles", n)
	}
	if !strings.Contains(page, "&lt;/title&gt;") {
		t.Errorf("name not escaped: %s", page)
	}
}

func TestRequestBaseFollowsTheProxy(t *testing.T) {
	r := httptest.NewRequest("GET", "https://shop.example/admin", nil)
	r.TLS = nil
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "shop.example, 10.0.0.1")

	if got := requestBase(r); got != "https://shop.example" {
		t.Errorf("behind a proxy: got %q", got)
	}

	plain := httptest.NewRequest("GET", "http://127.0.0.1:8080/", nil)
	if got := requestBase(plain); got != "http://127.0.0.1:8080" {
		t.Errorf("direct: got %q", got)
	}

	// A Host header is the client's, not ours to repeat back as a URL.
	smuggled := httptest.NewRequest("GET", "http://shop.example/", nil)
	smuggled.Host = "evil.example/https://shop.example"
	if got := requestBase(smuggled); got != "" {
		t.Errorf("host with a path in it accepted: %q", got)
	}
}
