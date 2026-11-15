package tanks

import (
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/settings"
	"github.com/reefshepherd/reefshepherd/internal/units"
)

const (
	asAdmin  = "admin"
	asMember = "member"
)

// env is the tank routes on a real migrated database, with the signed-in user taken from the X-Test-Role header.
type env struct {
	t        *testing.T
	db       *db.DB
	settings *settings.Store
	mux      http.Handler
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(t.Context(), filepath.Join(dir, "backups")); err != nil {
		t.Fatal(err)
	}
	store := settings.NewStore(database)
	mux := http.NewServeMux()
	Routes(mux, Deps{DB: database, Settings: store, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if role := r.Header.Get("X-Test-Role"); role != "" {
			r = r.WithContext(httpx.WithUser(r.Context(), httpx.User{ID: 1, Username: role, Role: role}))
		}
		mux.ServeHTTP(w, r)
	})
	return &env{t: t, db: database, settings: store, mux: handler}
}

func (e *env) do(role, method, path string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	var r *http.Request
	if body == nil {
		r = httptest.NewRequest(method, path, nil)
	} else {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatal(err)
		}
		r = httptest.NewRequest(method, path, strings.NewReader(string(b)))
		r.Header.Set("Content-Type", "application/json")
	}
	if role != "" {
		r.Header.Set("X-Test-Role", role)
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, r)
	return rec
}

// admin sends the request as an admin.
func (e *env) admin(method, path string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.do(asAdmin, method, path, body)
}

func (e *env) status(rec *httptest.ResponseRecorder, want int) {
	e.t.Helper()
	if rec.Code != want {
		e.t.Fatalf("status = %d, want %d; body=%s", rec.Code, want, rec.Body)
	}
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("body %q: %v", rec.Body, err)
	}
	return v
}

type errorBody struct {
	Error struct {
		Code   string            `json:"code"`
		Fields map[string]string `json:"fields"`
	} `json:"error"`
}

// fail checks the response is the given error and returns its field messages.
func (e *env) fail(rec *httptest.ResponseRecorder, status int, code string) map[string]string {
	e.t.Helper()
	var b errorBody
	_ = json.Unmarshal(rec.Body.Bytes(), &b)
	if rec.Code != status || b.Error.Code != code {
		e.t.Fatalf("status=%d code=%q, want %d %q; body=%s", rec.Code, b.Error.Code, status, code, rec.Body)
	}
	return b.Error.Fields
}

// create posts a tank as an admin and returns it.
func (e *env) create(body map[string]any) Tank {
	e.t.Helper()
	rec := e.admin(http.MethodPost, "/api/v1/tanks", body)
	e.status(rec, http.StatusCreated)
	return decode[Tank](e.t, rec)
}

func (e *env) count(query string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.db.R.QueryRowContext(e.t.Context(), query, args...).Scan(&n); err != nil {
		e.t.Fatalf("%s: %v", query, err)
	}
	return n
}

func (e *env) exec(query string, args ...any) {
	e.t.Helper()
	if _, err := e.db.W.ExecContext(e.t.Context(), query, args...); err != nil {
		e.t.Fatalf("%s: %v", query, err)
	}
}

func near(got, want, tol float64) bool { return math.Abs(got-want) <= tol }

func path(id int64, suffix string) string {
	return "/api/v1/tanks/" + strconv.FormatInt(id, 10) + suffix
}

// httptestRequest sends a raw body with the given content type as an admin.
func httptestRequest(e *env, method, target, contentType, body string) *httptest.ResponseRecorder {
	e.t.Helper()
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	r.Header.Set("X-Test-Role", asAdmin)
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, r)
	return rec
}

func unitOf(s string) units.Unit { return units.Unit(s) }
