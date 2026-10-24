package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func get(h http.Handler, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func builtUI() fstest.MapFS {
	return fstest.MapFS{
		"200.html":                          {Data: []byte("<html>app shell</html>")},
		"favicon.svg":                       {Data: []byte("<svg/>")},
		"manifest.webmanifest":              {Data: []byte("{}")},
		"_app/immutable/entry/start.abc.js": {Data: []byte("console.log(1)")},
		".gitkeep":                          {Data: []byte("secret")},
		"sub/.hidden":                       {Data: []byte("secret")},
	}
}

func TestPlaceholderWhenNotBuilt(t *testing.T) {
	h := NewHandler(fstest.MapFS{".gitkeep": {}})
	for _, target := range []string{"/", "/tanks/3/charts/5", "/.gitkeep"} {
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
	}
}

func TestPlaceholderHeadHasNoBody(t *testing.T) {
	rec := get(NewHandler(fstest.MapFS{}), http.MethodHead, "/")
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Errorf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestDeepLinkFallsBackToAppShell(t *testing.T) {
	h := NewHandler(builtUI())
	for _, target := range []string{"/", "/tanks/3/charts/5", "/login", "/missing.png"} {
		rec := get(h, http.MethodGet, target)
		if rec.Code != http.StatusOK || rec.Body.String() != "<html>app shell</html>" {
			t.Errorf("%s: status=%d body=%q", target, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s: cache control = %q", target, got)
		}
		if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
			t.Errorf("%s: content type = %q", target, got)
		}
	}
}

func TestFallbackUsesIndexWhenNo200(t *testing.T) {
	h := NewHandler(fstest.MapFS{"index.html": {Data: []byte("<html>index</html>")}})
	rec := get(h, http.MethodGet, "/deep/link")
	if rec.Body.String() != "<html>index</html>" {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestRealFilesAreServed(t *testing.T) {
	h := NewHandler(builtUI())

	rec := get(h, http.MethodGet, "/favicon.svg")
	if rec.Code != http.StatusOK || rec.Body.String() != "<svg/>" {
		t.Errorf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("cache control = %q", got)
	}

	rec = get(h, http.MethodGet, "/manifest.webmanifest")
	if got := rec.Header().Get("Content-Type"); got != "application/manifest+json" {
		t.Errorf("manifest content type = %q", got)
	}
}

func TestImmutableAssetsGetLongCache(t *testing.T) {
	rec := get(NewHandler(builtUI()), http.MethodGet, "/_app/immutable/entry/start.abc.js")
	if rec.Code != http.StatusOK || rec.Body.String() != "console.log(1)" {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("cache control = %q", got)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Errorf("content type = %q", got)
	}
}

func TestMissingBuildAssetIs404(t *testing.T) {
	rec := get(NewHandler(builtUI()), http.MethodGet, "/_app/immutable/entry/gone.js")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestHiddenFilesAreNotServed(t *testing.T) {
	h := NewHandler(builtUI())
	for _, target := range []string{"/.gitkeep", "/sub/.hidden"} {
		rec := get(h, http.MethodGet, target)
		if strings.Contains(rec.Body.String(), "secret") {
			t.Errorf("%s leaked a hidden file", target)
		}
	}
}

func TestPathTraversalStaysInsideTheFS(t *testing.T) {
	rec := get(NewHandler(builtUI()), http.MethodGet, "/../../etc/passwd")
	if strings.Contains(rec.Body.String(), "root:") {
		t.Error("escaped the filesystem")
	}
}

func TestOnlyGetAndHead(t *testing.T) {
	h := NewHandler(builtUI())
	rec := get(h, http.MethodPost, "/")
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Errorf("status=%d allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
	rec = get(h, http.MethodHead, "/favicon.svg")
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Errorf("HEAD status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestEmbeddedHandlerServesPlaceholder(t *testing.T) {
	// The committed dist holds only .gitkeep until the web UI is built.
	rec := get(Handler(), http.MethodGet, "/some/page")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ReefShepherd") {
		t.Errorf("body = %q", rec.Body.String())
	}
}
