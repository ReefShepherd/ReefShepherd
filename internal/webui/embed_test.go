package webui

import (
	"net/http"
	"strings"
	"testing"
)

func TestEmbeddedHandlerServesAPage(t *testing.T) {
	// The embedded dist holds the placeholder source until the web UI is built, and the app shell afterwards.
	rec := get(Handler(), http.MethodGet, "/some/page")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Errorf("content type = %q", got)
	}
}
