package dashboard

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/settings"
)

// env is the dashboard routes on a real migrated database, with the signed-in user taken from the X-Test-User header.
type env struct {
	t   *testing.T
	db  *db.DB
	mux http.Handler
}

const (
	userA = 1
	userB = 2
)

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
	mux := http.NewServeMux()
	Routes(mux, Deps{DB: database, Settings: settings.NewStore(database), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if raw := r.Header.Get("X-Test-User"); raw != "" {
			id, _ := strconv.ParseInt(raw, 10, 64)
			r = r.WithContext(httpx.WithUser(r.Context(), httpx.User{ID: id, Username: "user" + raw, Role: httpx.RoleMember}))
		}
		mux.ServeHTTP(w, r)
	})
	e := &env{t: t, db: database, mux: handler}
	e.exec(`INSERT INTO users (id, username) VALUES (?, 'alice'), (?, 'bob')`, userA, userB)
	return e
}

func (e *env) exec(query string, args ...any) {
	e.t.Helper()
	if _, err := e.db.W.ExecContext(e.t.Context(), query, args...); err != nil {
		e.t.Fatalf("%s: %v", query, err)
	}
}

func (e *env) count(query string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.db.R.QueryRowContext(e.t.Context(), query, args...).Scan(&n); err != nil {
		e.t.Fatalf("%s: %v", query, err)
	}
	return n
}

// tank inserts a tank and returns its id.
func (e *env) tank(name string) int64 {
	e.t.Helper()
	res, err := e.db.W.ExecContext(e.t.Context(), `INSERT INTO tanks (name) VALUES (?)`, name)
	if err != nil {
		e.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

// param inserts a custom parameter definition and returns its id.
func (e *env) param(name string) int64 {
	e.t.Helper()
	res, err := e.db.W.ExecContext(e.t.Context(), `INSERT INTO parameter_definitions (name, sort_order) VALUES (?, 1000)`, name)
	if err != nil {
		e.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

// enable switches a parameter on (or off) for a tank with the given tank-level sort order.
func (e *env) enable(tankID, paramID int64, enabled bool, sortOrder int) {
	e.t.Helper()
	flag := 0
	if enabled {
		flag = 1
	}
	e.exec(`INSERT INTO tank_parameters (tank_id, parameter_id, enabled, sort_order) VALUES (?, ?, ?, ?)
		ON CONFLICT (tank_id, parameter_id) DO UPDATE SET enabled = excluded.enabled, sort_order = excluded.sort_order`, tankID, paramID, flag, sortOrder)
}

func (e *env) do(user int, method, target string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	var r *http.Request
	if body == nil {
		r = httptest.NewRequest(method, target, nil)
	} else {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatal(err)
		}
		r = httptest.NewRequest(method, target, strings.NewReader(string(b)))
		r.Header.Set("Content-Type", "application/json")
	}
	if user != 0 {
		r.Header.Set("X-Test-User", strconv.Itoa(user))
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, r)
	return rec
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

// get fetches the dashboard as a user and returns its cards.
func (e *env) get(user int, tankID int64) []Card {
	e.t.Helper()
	rec := e.do(user, http.MethodGet, dashPath(tankID, ""), nil)
	e.status(rec, http.StatusOK)
	return decode[Layout](e.t, rec).Cards
}

// put replaces the dashboard as a user and returns the resulting cards.
func (e *env) put(user int, tankID int64, cards []map[string]any) []Card {
	e.t.Helper()
	rec := e.do(user, http.MethodPut, dashPath(tankID, ""), map[string]any{"cards": cards})
	e.status(rec, http.StatusOK)
	return decode[Layout](e.t, rec).Cards
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

func dashPath(tankID int64, suffix string) string {
	return "/api/v1/tanks/" + strconv.FormatInt(tankID, 10) + "/dashboard" + suffix
}

// shape reduces cards to "type:parameter:size" strings so a test can compare whole layouts at once.
func shape(cards []Card) []string {
	out := make([]string, len(cards))
	for i, c := range cards {
		p := "-"
		if c.ParameterID != nil {
			p = strconv.FormatInt(*c.ParameterID, 10)
		}
		out[i] = c.CardType + ":" + p + ":" + c.Size
	}
	return out
}

// newRawRequest sends a raw body with the given content type as the first user.
func newRawRequest(e *env, method, target, contentType, body string) *httptest.ResponseRecorder {
	e.t.Helper()
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	r.Header.Set("X-Test-User", strconv.Itoa(userA))
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, r)
	return rec
}
