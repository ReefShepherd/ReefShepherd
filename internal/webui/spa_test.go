package webui

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

const (
	shellBody = `<html><head><meta http-equiv="content-security-policy" content="default-src 'none'; script-src 'self' 'sha256-abc='; style-src-attr 'unsafe-hashes' 'sha256-def='"></head><body>shell</body></html>`
	shellCSP  = "default-src 'none'; script-src 'self' 'sha256-abc='; style-src-attr 'unsafe-hashes' 'sha256-def='; frame-ancestors 'none'"
)

func get(h http.Handler, method, target string, headers ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func builtUI() fstest.MapFS {
	return fstest.MapFS{
		"200.html":                             {Data: []byte(shellBody)},
		"200.html.br":                          {Data: []byte("shell-br")},
		"200.html.gz":                          {Data: []byte("shell-gz")},
		"favicon.svg":                          {Data: []byte("<svg/>")},
		"favicon.svg.gz":                       {Data: []byte("svg-gz")},
		"icon.png":                             {Data: []byte("png")},
		"blob.zzz":                             {Data: []byte("plain")},
		"blob.zzz.gz":                          {Data: []byte("<html>looks like html")},
		"manifest.webmanifest":                 {Data: []byte("{}")},
		"_app/version.json":                    {Data: []byte(`{"version":"1"}`)},
		"_app/immutable/entry/start.abc.js":    {Data: []byte("console.log(1)")},
		"_app/immutable/entry/start.abc.js.br": {Data: []byte("js-br")},
		"_app/immutable/entry/start.abc.js.gz": {Data: []byte("js-gz")},
		"api/leak.txt":                         {Data: []byte("leak")},
		".gitkeep":                             {Data: []byte("secret")},
		"sub/.hidden":                          {Data: []byte("secret")},
	}
}

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:16])
}

func TestPlaceholderWhenNotBuilt(t *testing.T) {
	h := NewHandler(fstest.MapFS{".gitkeep": {}})
	for _, target := range []string{"/", "/tanks/3/charts/5", "/login"} {
		rec := get(h, http.MethodGet, target)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d", target, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "make web") {
			t.Errorf("%s: body is not the placeholder: %q", target, rec.Body.String())
		}
		if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
			t.Errorf("%s: content type = %q", target, got)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: cache control = %q", target, got)
		}
		if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "style-src 'unsafe-inline'") || !strings.Contains(got, "frame-ancestors 'none'") {
			t.Errorf("%s: csp = %q", target, got)
		}
	}
}

func TestPlaceholderHeadHasNoBody(t *testing.T) {
	rec := get(NewHandler(fstest.MapFS{}), http.MethodHead, "/")
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Errorf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestUnbuiltUIStill404sMissingFiles(t *testing.T) {
	rec := get(NewHandler(fstest.MapFS{".gitkeep": {}}), http.MethodGet, "/favicon.ico")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestDeepLinksGetTheAppShell(t *testing.T) {
	h := NewHandler(builtUI())
	for _, target := range []string{"/", "/tanks/3/charts/5", "/login"} {
		rec := get(h, http.MethodGet, target)
		if rec.Code != http.StatusOK || rec.Body.String() != shellBody {
			t.Errorf("%s: status=%d body=%q", target, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s: cache control = %q", target, got)
		}
		if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
			t.Errorf("%s: content type = %q", target, got)
		}
	}
}

func TestShellPolicyComesFromTheMetaTag(t *testing.T) {
	h := NewHandler(builtUI())
	if got := get(h, http.MethodGet, "/settings").Header().Get("Content-Security-Policy"); got != shellCSP {
		t.Errorf("csp = %q, want %q", got, shellCSP)
	}
	// The policy is for the page, not for the assets it loads.
	if got := get(h, http.MethodGet, "/favicon.svg").Header().Get("Content-Security-Policy"); got != "" {
		t.Errorf("asset csp = %q", got)
	}
}

func TestShellPolicyIsDecodedFromHTMLEntities(t *testing.T) {
	page := `<meta http-equiv="content-security-policy" content="script-src &#39;self&#39;">`
	h := NewHandler(fstest.MapFS{"200.html": {Data: []byte(page)}})
	want := "script-src 'self'; frame-ancestors 'none'"
	if got := get(h, http.MethodGet, "/").Header().Get("Content-Security-Policy"); got != want {
		t.Errorf("csp = %q, want %q", got, want)
	}
}

func TestShellWithoutMetaPolicySendsNoHeader(t *testing.T) {
	h := NewHandler(fstest.MapFS{"200.html": {Data: []byte("<html></html>")}})
	if got := get(h, http.MethodGet, "/").Header().Get("Content-Security-Policy"); got != "" {
		t.Errorf("csp = %q", got)
	}
}

func TestCacheControl(t *testing.T) {
	h := NewHandler(builtUI())
	cases := map[string]string{
		"/_app/immutable/entry/start.abc.js": "public, max-age=31536000, immutable",
		"/_app/version.json":                 "no-cache",
		"/favicon.svg":                       "no-cache",
		"/manifest.webmanifest":              "no-cache",
		"/tanks/3":                           "no-cache",
	}
	for target, want := range cases {
		if got := get(h, http.MethodGet, target).Header().Get("Cache-Control"); got != want {
			t.Errorf("%s: cache control = %q, want %q", target, got, want)
		}
	}
}

func TestPrecompressedSelection(t *testing.T) {
	h := NewHandler(builtUI())
	const js = "/_app/immutable/entry/start.abc.js"
	cases := []struct {
		name, target, acceptEncoding, wantEncoding, wantBody string
	}{
		{"identity", js, "", "", "console.log(1)"},
		{"gzip only", js, "gzip", "gzip", "js-gz"},
		{"br only", js, "br", "br", "js-br"},
		{"br preferred over gzip", js, "gzip, deflate, br", "br", "js-br"},
		{"refused with q=0", js, "br;q=0, gzip", "gzip", "js-gz"},
		{"unrelated coding", js, "deflate, zstd", "", "console.log(1)"},
		{"no sibling for the coding", "/favicon.svg", "br", "", "<svg/>"},
		{"sibling for gzip only", "/favicon.svg", "br, gzip", "gzip", "svg-gz"},
		{"never compressed", "/icon.png", "br, gzip", "", "png"},
		{"shell", "/deep/link", "br, gzip", "br", "shell-br"},
		{"shell gzip", "/deep/link", "gzip", "gzip", "shell-gz"},
	}
	for _, c := range cases {
		rec := get(h, http.MethodGet, c.target, "Accept-Encoding", c.acceptEncoding)
		if rec.Code != http.StatusOK || rec.Body.String() != c.wantBody {
			t.Errorf("%s: status=%d body=%q, want %q", c.name, rec.Code, rec.Body.String(), c.wantBody)
		}
		if got := rec.Header().Get("Content-Encoding"); got != c.wantEncoding {
			t.Errorf("%s: content encoding = %q, want %q", c.name, got, c.wantEncoding)
		}
		if got := rec.Header().Get("Vary"); !strings.Contains(got, "Accept-Encoding") {
			t.Errorf("%s: vary = %q", c.name, got)
		}
	}
}

func TestPrecompressedKeepsTheOriginalContentType(t *testing.T) {
	h := NewHandler(builtUI())
	cases := map[string]string{
		"/_app/immutable/entry/start.abc.js": "javascript",
		"/favicon.svg":                       "image/svg+xml",
		"/deep/link":                         "text/html",
	}
	for target, want := range cases {
		rec := get(h, http.MethodGet, target, "Accept-Encoding", "br, gzip")
		if got := rec.Header().Get("Content-Type"); !strings.Contains(got, want) {
			t.Errorf("%s: content type = %q, want it to contain %q", target, got, want)
		}
	}
}

func TestPrecompressedFileOfUnknownTypeIsNotSniffed(t *testing.T) {
	rec := get(NewHandler(builtUI()), http.MethodGet, "/blob.zzz", "Accept-Encoding", "gzip")
	if got := rec.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Errorf("content type = %q", got)
	}
}

func TestPrecompressedHeadHasNoBody(t *testing.T) {
	rec := get(NewHandler(builtUI()), http.MethodHead, "/favicon.svg", "Accept-Encoding", "gzip")
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 || rec.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("status=%d body=%q encoding=%q", rec.Code, rec.Body.String(), rec.Header().Get("Content-Encoding"))
	}
}

func TestETagAndConditionalRequests(t *testing.T) {
	h := NewHandler(builtUI())

	for _, target := range []string{"/deep/link", "/favicon.svg", "/_app/immutable/entry/start.abc.js"} {
		rec := get(h, http.MethodGet, target)
		etag := rec.Header().Get("ETag")
		if etag == "" {
			t.Fatalf("%s: no etag", target)
		}
		if rec.Header().Get("Vary") == "" {
			t.Errorf("%s: no vary", target)
		}
		cond := get(h, http.MethodGet, target, "If-None-Match", etag)
		if cond.Code != http.StatusNotModified || cond.Body.Len() != 0 {
			t.Errorf("%s: conditional status=%d body=%q", target, cond.Code, cond.Body.String())
		}
		if stale := get(h, http.MethodGet, target, "If-None-Match", `"other"`); stale.Code != http.StatusOK {
			t.Errorf("%s: stale validator status = %d", target, stale.Code)
		}
	}
}

func TestShellETagIsTheHashOfTheShell(t *testing.T) {
	h := NewHandler(builtUI())
	if got, want := get(h, http.MethodGet, "/deep/link").Header().Get("ETag"), `"`+sha(shellBody)+`"`; got != want {
		t.Errorf("etag = %q, want %q", got, want)
	}
}

func TestETagDiffersPerEncodingAndPerContent(t *testing.T) {
	h := NewHandler(builtUI())
	const target = "/_app/immutable/entry/start.abc.js"
	plain := get(h, http.MethodGet, target).Header().Get("ETag")
	br := get(h, http.MethodGet, target, "Accept-Encoding", "br").Header().Get("ETag")
	gz := get(h, http.MethodGet, target, "Accept-Encoding", "gzip").Header().Get("ETag")
	if plain == br || plain == gz || br == gz {
		t.Errorf("etags collide: plain=%s br=%s gzip=%s", plain, br, gz)
	}
	if !strings.HasSuffix(br, `-br"`) || !strings.HasSuffix(gz, `-gzip"`) {
		t.Errorf("encoding suffix missing: br=%s gzip=%s", br, gz)
	}
	// A validator from one encoding must not match another.
	if rec := get(h, http.MethodGet, target, "Accept-Encoding", "br", "If-None-Match", plain); rec.Code != http.StatusOK {
		t.Errorf("identity validator on a br response: status = %d", rec.Code)
	}
	if other := get(h, http.MethodGet, "/favicon.svg").Header().Get("ETag"); other == plain {
		t.Errorf("different files share an etag: %s", other)
	}
}

func TestMissingFilesAre404(t *testing.T) {
	h := NewHandler(builtUI())
	targets := []string{
		"/favicon.ico", "/missing.png", "/some/dir/missing.css", "/_app/immutable/entry/gone.js",
		"/_app/unknown", "/robots.txt", "/200.html", "/.gitkeep", "/sub/.hidden", "/.env",
	}
	for _, target := range targets {
		rec := get(h, http.MethodGet, target)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d", target, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "secret") || strings.Contains(rec.Body.String(), "shell") {
			t.Errorf("%s: body = %q", target, rec.Body.String())
		}
	}
}

func TestAPIPathsNeverFallThroughToTheShell(t *testing.T) {
	h := NewHandler(builtUI())
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost} {
		for _, target := range []string{"/api", "/api/", "/api/v1/me", "/api/v1/tanks/3", "/api/leak.txt", "/api/../api/x"} {
			rec := get(h, method, target)
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s %s: status = %d", method, target, rec.Code)
			}
			if strings.Contains(rec.Body.String(), "shell") || strings.Contains(rec.Body.String(), "leak") {
				t.Errorf("%s %s: body = %q", method, target, rec.Body.String())
			}
		}
	}
	rec := get(h, http.MethodGet, "/api/v1/me")
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") || !strings.Contains(rec.Body.String(), `"code":"not_found"`) {
		t.Errorf("content type = %q, body = %q", got, rec.Body.String())
	}
}

func TestOnlyGetAndHead(t *testing.T) {
	h := NewHandler(builtUI())
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		for _, target := range []string{"/", "/deep/link", "/favicon.svg"} {
			rec := get(h, method, target)
			if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" {
				t.Errorf("%s %s: status=%d allow=%q", method, target, rec.Code, rec.Header().Get("Allow"))
			}
		}
	}
	rec := get(h, http.MethodHead, "/favicon.svg")
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Errorf("HEAD status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestMIMETypes(t *testing.T) {
	h := NewHandler(fstest.MapFS{
		"200.html":             {Data: []byte("<html></html>")},
		"manifest.webmanifest": {Data: []byte("{}")},
		"font.woff2":           {Data: []byte("font")},
		"favicon.ico":          {Data: []byte("ico")},
		"app.css":              {Data: []byte("a{}")},
		"data.json":            {Data: []byte("{}")},
		"logo.svg":             {Data: []byte("<svg/>")},
	})
	cases := map[string]string{
		"/manifest.webmanifest": "application/manifest+json",
		"/font.woff2":           "font/woff2",
		"/favicon.ico":          "image/x-icon",
		"/app.css":              "text/css; charset=utf-8",
		"/data.json":            "application/json",
		"/logo.svg":             "image/svg+xml",
	}
	for target, want := range cases {
		if got := get(h, http.MethodGet, target).Header().Get("Content-Type"); got != want {
			t.Errorf("%s: content type = %q, want %q", target, got, want)
		}
	}
}

func TestPathTraversalStaysInsideTheFS(t *testing.T) {
	for _, target := range []string{"/../../etc/passwd", "/..%2f..%2fetc/passwd", "/_app/../../etc/passwd"} {
		rec := get(NewHandler(builtUI()), http.MethodGet, target)
		if strings.Contains(rec.Body.String(), "root:") {
			t.Errorf("%s escaped the filesystem", target)
		}
	}
}

func TestAcceptsEncoding(t *testing.T) {
	cases := []struct {
		header, coding string
		want           bool
	}{
		{"", "br", false},
		{"br", "br", true},
		{"BR", "br", true},
		{"gzip, br", "br", true},
		{"gzip;q=0.5, br;q=1", "br", true},
		{"br;q=0", "br", false},
		{"br; q=0.0", "br", false},
		{"br;q=0.1", "br", true},
		{"gzip", "br", false},
		{"brotli", "br", false},
		{"*", "br", false},
	}
	for _, c := range cases {
		if got := acceptsEncoding(c.header, c.coding); got != c.want {
			t.Errorf("acceptsEncoding(%q, %q) = %v, want %v", c.header, c.coding, got, c.want)
		}
	}
}
