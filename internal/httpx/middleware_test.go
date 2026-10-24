package httpx

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestChainOrder(t *testing.T) {
	var order []string
	mark := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { order = append(order, "handler") }),
		mark("first"), mark("second"))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if strings.Join(order, ",") != "first,second,handler" {
		t.Errorf("order = %v", order)
	}
}

func TestRequestLog(t *testing.T) {
	var buf bytes.Buffer
	h := RequestLog(testLogger(&buf))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("short"))
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/tanks", nil))

	line := buf.String()
	for _, want := range []string{"level=INFO", "method=POST", "path=/api/v1/tanks", "status=418", "bytes=5"} {
		if !strings.Contains(line, want) {
			t.Errorf("log line %q is missing %q", line, want)
		}
	}
}

func TestRequestLogDefaultStatusAndHealthzLevel(t *testing.T) {
	var buf bytes.Buffer
	h := RequestLog(testLogger(&buf))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))

	line := buf.String()
	if !strings.Contains(line, "level=DEBUG") || !strings.Contains(line, "status=200") {
		t.Errorf("log line = %q", line)
	}
}

func TestRecover(t *testing.T) {
	var buf bytes.Buffer
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }),
		RequestLog(testLogger(&buf)), Recover(testLogger(&buf)))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", rec.Code)
	}
	want := `{"error":{"code":"internal","message":"internal error"}}` + "\n"
	if rec.Body.String() != want {
		t.Errorf("body = %q", rec.Body.String())
	}
	logged := buf.String()
	if !strings.Contains(logged, "panic in handler") || !strings.Contains(logged, "panic=boom") {
		t.Errorf("panic not logged: %q", logged)
	}
	if !strings.Contains(logged, "status=500") {
		t.Errorf("request log did not see the 500: %q", logged)
	}
}

func TestRecoverAfterResponseStarted(t *testing.T) {
	var buf bytes.Buffer
	h := Recover(testLogger(&buf))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		panic("late")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusAccepted || rec.Body.Len() != 0 {
		t.Errorf("status=%d body=%q, the started response must be left alone", rec.Code, rec.Body.String())
	}
}

func TestRecoverRepanicsAbortHandler(t *testing.T) {
	h := Recover(testLogger(&bytes.Buffer{}))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	defer func() {
		err, _ := recover().(error)
		if !errors.Is(err, http.ErrAbortHandler) {
			t.Error("http.ErrAbortHandler was swallowed")
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestMaxBody(t *testing.T) {
	var readErr error
	h := MaxBody(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		_, readErr = buf.ReadFrom(r.Body)
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("within limit", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("0123456789")))
		if rec.Code != http.StatusOK || readErr != nil {
			t.Errorf("status=%d err=%v", rec.Code, readErr)
		}
	})
	t.Run("declared length too large", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("0123456789ABC")))
		if rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), CodePayloadTooLarge) {
			t.Errorf("status=%d body=%q", rec.Code, rec.Body.String())
		}
	})
	t.Run("undeclared length too large", func(t *testing.T) {
		readErr = nil
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("0123456789ABC"))
		r.ContentLength = -1
		h.ServeHTTP(httptest.NewRecorder(), r)
		if readErr == nil {
			t.Error("reading past the limit succeeded")
		}
	})
}

func TestSecurityHeaders(t *testing.T) {
	h := SecurityHeaders("default-src 'self'")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hi"))
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	want := map[string]string{
		"Content-Security-Policy": "default-src 'self'",
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "same-origin",
		"X-Frame-Options":         "DENY",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestCrossOrigin(t *testing.T) {
	mw, err := CrossOrigin([]string{"https://trusted.lan"})
	if err != nil {
		t.Fatal(err)
	}
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))

	tests := []struct {
		name    string
		method  string
		headers map[string]string
		want    int
	}{
		{"post without browser headers", http.MethodPost, nil, http.StatusNoContent},
		{"post same-origin", http.MethodPost, map[string]string{"Sec-Fetch-Site": "same-origin"}, http.StatusNoContent},
		{"post cross-site", http.MethodPost, map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"delete same-site", http.MethodDelete, map[string]string{"Sec-Fetch-Site": "same-site"}, http.StatusForbidden},
		{"get cross-site is safe", http.MethodGet, map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusNoContent},
		{"old browser origin matches host", http.MethodPost, map[string]string{"Origin": "http://example.com"}, http.StatusNoContent},
		{"old browser origin differs", http.MethodPost, map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
		{"trusted origin cross-site", http.MethodPost,
			map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://trusted.lan"}, http.StatusNoContent},
		{"untrusted origin cross-site", http.MethodPost,
			map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, "http://example.com/api/v1/x", nil)
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.want, rec.Body.String())
			}
			if tt.want == http.StatusForbidden && !strings.Contains(rec.Body.String(), `"code":"forbidden"`) {
				t.Errorf("deny body is not the JSON error format: %q", rec.Body.String())
			}
		})
	}
}

func TestCrossOriginRejectsBadTrustedOrigin(t *testing.T) {
	if _, err := CrossOrigin([]string{"not-an-origin"}); err == nil {
		t.Error("expected an error for an origin without a scheme")
	}
}

func TestCurrentUser(t *testing.T) {
	if _, ok := CurrentUser(context.Background()); ok {
		t.Error("anonymous context reported a user")
	}
	ctx := WithUser(context.Background(), User{ID: 7, Username: "ann", Role: RoleAdmin})
	u, ok := CurrentUser(ctx)
	if !ok || u.ID != 7 || u.Username != "ann" || !u.IsAdmin() {
		t.Errorf("got %+v, %v", u, ok)
	}
	if (User{Role: RoleMember}).IsAdmin() {
		t.Error("member reported as admin")
	}
}

func TestRecorderKeepsFlusher(t *testing.T) {
	var flushed bool
	h := RequestLog(testLogger(&bytes.Buffer{}))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flushed = http.NewResponseController(w).Flush() == nil
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !flushed {
		t.Error("ResponseController could not flush through the logging wrapper")
	}
}
