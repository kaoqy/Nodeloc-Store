package main

import (
	"html"
	"net/http"
	"regexp"
	"strings"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/system"
)

// Both bundles repaint the document head as soon as they boot, which is exactly
// the problem: a crawler never runs them, a forum preview never runs them, and a
// person glancing at the tab sees "Nodeloc Store 数字商品平台" for the second
// before the status request lands. The shop's name, its summary and its icon
// belong to the shop, not to the build, so the server writes them into the
// document it sends.

const adminTitleSuffix = " 管理后台"

var (
	reHTML        = regexp.MustCompile(`(?s)<html\b[^>]*>`)
	reLang        = regexp.MustCompile(`(?i)\blang=["'][^"']*["']`)
	reTitle       = regexp.MustCompile(`(?s)<title\b[^>]*>.*?</title>`)
	reDescription = regexp.MustCompile(`(?s)<meta\b[^>]*\bname=["']description["'][^>]*>`)
	reIcon        = regexp.MustCompile(`(?s)<link\b[^>]*\brel=["']icon["'][^>]*>`)
	reHeadClose   = regexp.MustCompile(`(?i)</head>`)
)

// stampShell returns document with the shop's own language, title, description,
// icon and social-preview tags in its head. admin picks the back office's
// wording and keeps the page out of search results, which is the one thing a
// storefront wants and a control panel does not.
func stampShell(document string, id system.ShellIdentity, base string, admin bool) string {
	title := strings.TrimSpace(id.Name)
	if title == "" {
		title = system.Default().App.Name
	}
	if admin {
		title += adminTitleSuffix
	}
	description := strings.TrimSpace(id.Description)
	logo := strings.TrimSpace(id.Logo)

	var missing []string
	if locale := strings.TrimSpace(id.Locale); locale != "" {
		document = reHTML.ReplaceAllStringFunc(document, func(open string) string {
			quoted := `lang="` + html.EscapeString(locale) + `"`
			if reLang.MatchString(open) {
				return reLang.ReplaceAllLiteralString(open, quoted)
			}
			return strings.Replace(open, ">", ` `+quoted+">", 1)
		})
	}
	document = putInHead(document, reTitle, "<title>"+html.EscapeString(title)+"</title>", &missing)
	if description != "" {
		document = putInHead(document, reDescription, `<meta name="description" content="`+html.EscapeString(description)+`" />`, &missing)
	}
	if logo != "" {
		document = putInHead(document, reIcon, `<link rel="icon" href="`+html.EscapeString(logo)+`" />`, &missing)
	}

	// A preview card is built from og: tags, and an image another server cannot
	// fetch is not a preview image, so an inline data: logo paints the tab and
	// nothing else.
	preview := []string{`<meta property="og:title" content="` + html.EscapeString(title) + `" />`}
	if description != "" {
		preview = append(preview, `<meta property="og:description" content="`+html.EscapeString(description)+`" />`)
	}
	if image := absoluteImageURL(logo, base); image != "" {
		preview = append(preview, `<meta property="og:image" content="`+html.EscapeString(image)+`" />`)
	}
	preview = append(preview, `<meta name="twitter:card" content="summary" />`)
	if admin {
		preview = append(preview, `<meta name="robots" content="noindex, nofollow" />`)
	}

	block := strings.Join(append(missing, preview...), "\n    ")
	return reHeadClose.ReplaceAllLiteralString(document, block+"</head>")
}

// putInHead swaps one tag for the shop's own version, or records it to append
// when the built document never had one.
func putInHead(document string, re *regexp.Regexp, tag string, missing *[]string) string {
	if !re.MatchString(document) {
		*missing = append(*missing, tag)
		return document
	}
	return re.ReplaceAllLiteralString(document, tag)
}

// absoluteImageURL resolves a stored logo against the address the shop is being
// reached at, because a preview card needs a URL another server can open.
func absoluteImageURL(logo, base string) string {
	switch {
	case strings.HasPrefix(logo, "https://"), strings.HasPrefix(logo, "http://"):
		return logo
	case strings.HasPrefix(logo, "/") && base != "":
		return base + logo
	}
	return ""
}

// requestBase is the absolute address of this shop as the client sees it. The
// configured domain is optional and a container usually has none, while the
// proxy in front of it reports both halves. Anything that does not look like a
// host is refused rather than repeated back to the browser.
func requestBase(r *http.Request) string {
	host := firstHop(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if host == "" || strings.ContainsAny(host, "/ \\@") {
		return ""
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if firstHop(r.Header.Get("X-Forwarded-Proto")) == "https" {
		scheme = "https"
	}
	return scheme + "://" + host
}

// firstHop takes the client-facing value out of a header a chain of proxies may
// have appended to.
func firstHop(value string) string {
	value, _, _ = strings.Cut(value, ",")
	return strings.TrimSpace(value)
}
