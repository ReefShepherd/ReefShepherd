// Package webui serves the built web UI that is embedded in the binary.
package webui

import (
	"bytes"
	"embed"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed all:dist
var distFS embed.FS

// Handler serves the embedded UI.
func Handler() http.Handler {
	// The dist directory is guaranteed by the embed directive, so Sub cannot fail.
	sub, _ := fs.Sub(distFS, "dist")
	return NewHandler(sub)
}

// NewHandler serves the UI files in fsys: real files as they are, hashed assets with a long cache,
// and every other path with the single-page-app fallback page (or a placeholder when the UI is not built).
func NewHandler(fsys fs.FS) http.Handler {
	return &handler{fsys: fsys}
}

type handler struct {
	fsys fs.FS
}

const (
	immutablePrefix = "_app/immutable/"
	appPrefix       = "_app/"
	cacheImmutable  = "public, max-age=31536000, immutable"
	cacheRevalidate = "no-cache"
)

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name != "" && !hasHiddenSegment(name) && h.isFile(name) {
		cache := cacheRevalidate
		if strings.HasPrefix(name, immutablePrefix) {
			cache = cacheImmutable
		}
		h.serveFile(w, r, name, cache)
		return
	}

	// A missing build asset is a real 404, not a page, so a stale tab fails loudly.
	if strings.HasPrefix(name, appPrefix) {
		http.NotFound(w, r)
		return
	}

	// 200.html is what SvelteKit's static adapter emits for single-page-app mode.
	for _, page := range []string{"200.html", "index.html"} {
		if h.isFile(page) {
			h.serveFile(w, r, page, cacheRevalidate)
			return
		}
	}
	servePlaceholder(w, r)
}

func (h *handler) isFile(name string) bool {
	info, err := fs.Stat(h.fsys, name)
	return err == nil && info.Mode().IsRegular()
}

func hasHiddenSegment(name string) bool {
	for _, seg := range strings.Split(name, "/") {
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

func (h *handler) serveFile(w http.ResponseWriter, r *http.Request, name, cache string) {
	f, err := h.fsys.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	body, ok := f.(io.ReadSeeker)
	if !ok {
		data, err := io.ReadAll(f)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		body = bytes.NewReader(data)
	}

	if path.Ext(name) == ".webmanifest" {
		w.Header().Set("Content-Type", "application/manifest+json")
	}
	w.Header().Set("Cache-Control", cache)
	http.ServeContent(w, r, name, time.Time{}, body)
}

const placeholderPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>ReefShepherd</title>
<style>
body { font-family: system-ui, sans-serif; max-width: 36rem; margin: 4rem auto; padding: 0 1rem; line-height: 1.5; }
code { background: rgba(127, 127, 127, 0.2); padding: 0.1em 0.3em; border-radius: 3px; }
</style>
</head>
<body>
<h1>ReefShepherd</h1>
<p>The server is running, but the web UI has not been built into this binary.</p>
<p>Run <code>make web</code> and then <code>make build</code>.</p>
</body>
</html>
`

func servePlaceholder(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	// The page carries its own inline style, which the application-wide policy would block.
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.WriteString(w, placeholderPage)
}
