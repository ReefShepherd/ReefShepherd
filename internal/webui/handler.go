package webui

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"html"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

const (
	shellFile       = "200.html"
	immutablePrefix = "_app/immutable/"
	appPrefix       = "_app/"
	cacheImmutable  = "public, max-age=31536000, immutable"
	cacheRevalidate = "no-cache"
)

// metaCSP matches the policy the static adapter writes into the app shell.
var metaCSP = regexp.MustCompile(`<meta http-equiv="content-security-policy" content="([^"]*)"`)

// encodings lists the precompressed siblings in order of preference.
var encodings = []struct{ token, ext string }{{"br", ".br"}, {"gzip", ".gz"}}

func init() {
	// Slim container images have no mime.types file, so register what the build emits.
	for ext, typ := range map[string]string{
		".webmanifest": "application/manifest+json",
		".woff2":       "font/woff2",
		".ico":         "image/x-icon",
	} {
		if err := mime.AddExtensionType(ext, typ); err != nil {
			panic(err)
		}
	}
}

// NewHandler serves the UI files in fsys: real files as they are (precompressed when the client accepts it),
// hashed assets with a long cache, and every other extensionless non-API path with the single-page-app shell
// (or a placeholder when the UI is not built), warning through logger once when the shell has no policy.
func NewHandler(fsys fs.FS, logger *slog.Logger) http.Handler {
	return newHandler(fsys, logger)
}

func newHandler(fsys fs.FS, logger *slog.Logger) *handler {
	h := &handler{fsys: fsys}
	if shell, err := fs.ReadFile(fsys, shellFile); err == nil {
		h.built = true
		// The policy carries script hashes that change with every build, so it is read from the shell rather than written here.
		if m := metaCSP.FindSubmatch(shell); m != nil {
			// frame-ancestors is not allowed in a meta element, so it is added to the header form.
			h.csp = html.UnescapeString(string(m[1])) + "; frame-ancestors 'none'"
		} else {
			logger.Warn("web UI shell has no content security policy meta tag, so pages are served without one", slog.String("file", shellFile))
		}
	}
	return h
}

type handler struct {
	fsys  fs.FS
	built bool
	csp   string
	// etags caches the content hash of each file, by name.
	etags sync.Map
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	// The app router answers /api/ first; this keeps the handler from serving files or the shell under /api when mounted on its own.
	if name == "api" || strings.HasPrefix(name, "api/") {
		httpx.WriteError(w, httpx.NotFound("no such API endpoint"))
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if name != shellFile && h.isFile(name) {
		cache := cacheRevalidate
		if strings.HasPrefix(name, immutablePrefix) {
			cache = cacheImmutable
		}
		h.serveFile(w, r, name, cache)
		return
	}

	// A missing build asset or file is a real 404, not the shell, so a stale tab or broken link fails loudly.
	if strings.HasPrefix(name, appPrefix) || path.Ext(name) != "" {
		http.NotFound(w, r)
		return
	}

	if !h.built {
		servePlaceholder(w, r)
		return
	}
	if h.csp != "" {
		w.Header().Set("Content-Security-Policy", h.csp)
	}
	h.serveFile(w, r, shellFile, cacheRevalidate)
}

func (h *handler) isFile(name string) bool {
	if name == "" || !fs.ValidPath(name) || hasHiddenSegment(name) {
		return false
	}
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

// serveFile serves name, or its precompressed sibling when the client accepts that encoding.
func (h *handler) serveFile(w http.ResponseWriter, r *http.Request, name, cache string) {
	hdr := w.Header()
	hdr.Add("Vary", "Accept-Encoding")
	hdr.Set("Cache-Control", cache)

	file, enc := name, ""
	for _, c := range encodings {
		if acceptsEncoding(r.Header.Get("Accept-Encoding"), c.token) && h.isFile(name+c.ext) {
			file, enc = name+c.ext, c.token
			break
		}
	}

	tag, ok := h.etag(name)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if enc != "" {
		tag += "-" + enc
		hdr.Set("Content-Encoding", enc)
	}
	hdr.Set("ETag", `"`+tag+`"`)

	// ServeContent types a file by its original name, but would sniff the compressed bytes when that name is unknown.
	if enc != "" && mime.TypeByExtension(path.Ext(name)) == "" {
		hdr.Set("Content-Type", "application/octet-stream")
	}

	f, err := h.fsys.Open(file)
	if err != nil {
		hdr.Del("Content-Encoding")
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	body, ok := f.(io.ReadSeeker)
	if !ok {
		data, err := io.ReadAll(f)
		if err != nil {
			hdr.Del("Content-Encoding")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		body = bytes.NewReader(data)
	}
	http.ServeContent(w, r, name, time.Time{}, body)
}

// etag is a hash of the uncompressed file, so every encoding of it shares one stem.
func (h *handler) etag(name string) (string, bool) {
	if v, ok := h.etags.Load(name); ok {
		return v.(string), true
	}
	data, err := fs.ReadFile(h.fsys, name)
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(data)
	tag := hex.EncodeToString(sum[:16])
	h.etags.Store(name, tag)
	return tag, true
}

// acceptsEncoding reports whether an Accept-Encoding header allows the coding; a quality of 0 refuses it.
func acceptsEncoding(header, coding string) bool {
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(part, ";")
		if !strings.EqualFold(strings.TrimSpace(fields[0]), coding) {
			continue
		}
		for _, param := range fields[1:] {
			param = strings.TrimSpace(param)
			if len(param) > 2 && strings.EqualFold(param[:2], "q=") {
				if v, err := strconv.ParseFloat(param[2:], 64); err == nil && v <= 0 {
					return false
				}
			}
		}
		return true
	}
	return false
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
