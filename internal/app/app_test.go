package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/config"
)

func newTestApp(t *testing.T, cfg config.Config) *App {
	t.Helper()
	a, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func do(a *App, method, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, r)
	return rec
}

func TestHealthz(t *testing.T) {
	rec := do(newTestApp(t, config.Config{}), http.MethodGet, "/healthz", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" || body["version"] == "" {
		t.Errorf("body = %v", body)
	}
}

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	a := newTestApp(t, config.Config{})
	for _, target := range []string{"/healthz", "/api/v1/nothing", "/", "/deep/link"} {
		rec := do(a, http.MethodGet, target, "", nil)
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: missing nosniff", target)
		}
		if rec.Header().Get("Content-Security-Policy") == "" {
			t.Errorf("%s: missing CSP", target)
		}
	}
	csp := do(a, http.MethodGet, "/healthz", "", nil).Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-ancestors 'none'") || !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("csp = %q", csp)
	}
}

func TestUnknownAPIPathIsJSON404(t *testing.T) {
	rec := do(newTestApp(t, config.Config{}), http.MethodGet, "/api/v1/nothing", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"code":"not_found"`) || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Errorf("body = %q, content type = %q", rec.Body.String(), rec.Header().Get("Content-Type"))
	}
}

func TestSPAFallbackServesPlaceholder(t *testing.T) {
	rec := do(newTestApp(t, config.Config{}), http.MethodGet, "/tanks/3/charts/5", "", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<html") {
		t.Errorf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestBodyLimit(t *testing.T) {
	rec := do(newTestApp(t, config.Config{}), http.MethodPost, "/api/v1/x", strings.Repeat("x", maxBodyBytes+1), nil)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestCrossOriginProtectionInFullStack(t *testing.T) {
	a := newTestApp(t, config.Config{TrustedOrigins: []string{"https://reef.home.lan"}})

	rec := do(a, http.MethodPost, "/api/v1/x", "{}", map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("cross-site post: status = %d", rec.Code)
	}
	rec = do(a, http.MethodPost, "/api/v1/x", "{}", map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://reef.home.lan"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("trusted-origin post should reach the router (404 here): status = %d", rec.Code)
	}
	rec = do(a, http.MethodPost, "/api/v1/x", "{}", map[string]string{"Sec-Fetch-Site": "same-origin"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("same-origin post should reach the router (404 here): status = %d", rec.Code)
	}
}

func TestNewRejectsBadTrustedOrigin(t *testing.T) {
	if _, err := New(config.Config{TrustedOrigins: []string{"nope"}}, slog.New(slog.NewTextHandler(io.Discard, nil))); err == nil {
		t.Error("expected error")
	}
}

func TestServeAndGracefulShutdown(t *testing.T) {
	a := newTestApp(t, config.Config{})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen on loopback: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Serve(ctx, ln) }()

	resp, err := http.Get("http://" + ln.Addr().String() + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve returned %v after shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after the context was cancelled")
	}
	if resp, err := http.Get("http://" + ln.Addr().String() + "/healthz"); err == nil {
		resp.Body.Close()
		t.Error("server still accepting connections after shutdown")
	}
}

func TestShutdownWaitsForInFlightRequest(t *testing.T) {
	a := newTestApp(t, config.Config{})
	started := make(chan struct{})
	release := make(chan struct{})
	a.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "finished")
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen on loopback: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serveDone := make(chan error, 1)
	go func() { serveDone <- a.Serve(ctx, ln) }()

	type result struct {
		status int
		body   string
		err    error
	}
	reqDone := make(chan result, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/slow")
		if err != nil {
			reqDone <- result{err: err}
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		reqDone <- result{status: resp.StatusCode, body: string(body), err: err}
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("handler never started")
	}
	cancel()

	select {
	case err := <-serveDone:
		t.Fatalf("Serve returned %v while a request was still in flight", err)
	case <-time.After(200 * time.Millisecond):
	}
	close(release)

	select {
	case res := <-reqDone:
		if res.err != nil || res.status != http.StatusOK || res.body != "finished" {
			t.Errorf("in-flight request: status=%d body=%q err=%v", res.status, res.body, res.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight request never completed")
	}
	select {
	case err := <-serveDone:
		if err != nil {
			t.Errorf("Serve returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after the request finished")
	}
}
