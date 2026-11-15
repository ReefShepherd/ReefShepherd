package params

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/settings"
	"github.com/reefshepherd/reefshepherd/internal/units"
)

// env serves the parameter routes on a real temporary database with a controllable clock and a signed-in user.
type env struct {
	t      *testing.T
	db     *db.DB
	store  *settings.Store
	svc    *Service
	h      http.Handler
	bare   http.Handler
	now    time.Time
	userID int64
}

func newEnv(t *testing.T) *env {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "reef.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(t.Context(), filepath.Join(t.TempDir(), "backups")); err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, db: database, store: settings.NewStore(database), now: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)}
	e.svc = NewService(Deps{DB: database, Settings: e.store, Logger: slog.New(slog.DiscardHandler)})
	e.svc.now = func() time.Time { return e.now }
	e.userID = e.insertID(`INSERT INTO users (username, role) VALUES ('tester', 'admin')`)

	mux := http.NewServeMux()
	e.svc.register(mux)
	e.bare = mux
	e.h = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := httpx.WithUser(r.Context(), httpx.User{ID: e.userID, Username: "tester", Role: httpx.RoleAdmin})
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
	return e
}

func (e *env) insertID(query string, args ...any) int64 {
	e.t.Helper()
	res, err := e.db.W.ExecContext(e.t.Context(), query, args...)
	if err != nil {
		e.t.Fatalf("%s: %v", query, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		e.t.Fatal(err)
	}
	return id
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

func (e *env) addTank(name string) int64 {
	e.t.Helper()
	return e.insertID(`INSERT INTO tanks (name) VALUES (?)`, name)
}

func (e *env) preset(key string) int64 {
	e.t.Helper()
	var id int64
	if err := e.db.R.QueryRowContext(e.t.Context(), `SELECT id FROM parameter_definitions WHERE preset_key = ?`, key).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

func (e *env) setUnit(family units.Family, unit units.Unit) {
	e.t.Helper()
	if err := e.store.SetUnit(e.t.Context(), family, unit); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) setTimezone(name string) {
	e.t.Helper()
	if err := e.store.SetTimezone(e.t.Context(), name); err != nil {
		e.t.Fatal(err)
	}
}

// storedValues returns the raw stored result values in insertion order.
func (e *env) storedValues() []float64 {
	e.t.Helper()
	rows, err := e.db.R.QueryContext(e.t.Context(), `SELECT value FROM test_results ORDER BY id`)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []float64
	for rows.Next() {
		var v float64
		if err := rows.Scan(&v); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		e.t.Fatal(err)
	}
	return out
}

// resp is a recorded response.
type resp struct {
	*httptest.ResponseRecorder
	t *testing.T
}

func (e *env) do(method, path string, body any) *resp {
	e.t.Helper()
	return e.doOn(e.h, method, path, body)
}

func (e *env) doOn(h http.Handler, method, path string, body any) *resp {
	e.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			e.t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return &resp{ResponseRecorder: rec, t: e.t}
}

// doRaw sends a request with an unparsed body and the given content type.
func (e *env) doRaw(method, path, contentType, body string) *resp {
	e.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return &resp{ResponseRecorder: rec, t: e.t}
}

// must sends a request and fails unless the status matches.
func (e *env) must(status int, method, path string, body any) *resp {
	e.t.Helper()
	r := e.do(method, path, body)
	if r.Code != status {
		e.t.Fatalf("%s %s: status=%d, want %d; body=%s", method, path, r.Code, status, r.Body)
	}
	return r
}

func (r *resp) decode(v any) {
	r.t.Helper()
	if err := json.Unmarshal(r.Body.Bytes(), v); err != nil {
		r.t.Fatalf("body %q: %v", r.Body.String(), err)
	}
}

type errBody struct {
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields"`
	} `json:"error"`
}

// fail expects the given status and error code and returns the error body.
func (e *env) fail(status int, code, method, path string, body any) errBody {
	e.t.Helper()
	r := e.do(method, path, body)
	var b errBody
	_ = json.Unmarshal(r.Body.Bytes(), &b)
	if r.Code != status || b.Error.Code != code {
		e.t.Fatalf("%s %s: status=%d code=%q, want %d %q; body=%s", method, path, r.Code, b.Error.Code, status, code, r.Body)
	}
	return b
}

// invalid expects a 422 validation error naming exactly the given fields.
func (e *env) invalid(method, path string, body any, fields ...string) errBody {
	e.t.Helper()
	b := e.fail(http.StatusUnprocessableEntity, "validation", method, path, body)
	if len(b.Error.Fields) != len(fields) {
		e.t.Fatalf("%s %s: fields=%v, want exactly %v", method, path, b.Error.Fields, fields)
	}
	for _, f := range fields {
		if b.Error.Fields[f] == "" {
			e.t.Fatalf("%s %s: no message for field %q in %v", method, path, f, b.Error.Fields)
		}
	}
	return b
}

type entry struct {
	ParameterID int64    `json:"parameter_id"`
	Value       *float64 `json:"value"`
	Note        string   `json:"note,omitempty"`
}

func val(pid int64, v float64) entry { return entry{ParameterID: pid, Value: &v} }

// addResults posts one multi-parameter entry and returns the stored results.
func (e *env) addResults(tankID int64, measuredAt string, entries ...entry) []ResultView {
	e.t.Helper()
	body := map[string]any{"results": entries}
	if measuredAt != "" {
		body["measured_at"] = measuredAt
	}
	var out struct {
		Results []ResultView `json:"results"`
	}
	e.must(http.StatusCreated, http.MethodPost, tankPath(tankID, "/results"), body).decode(&out)
	return out.Results
}

func (e *env) tankParams(tankID int64) TankParameters {
	e.t.Helper()
	var out TankParameters
	e.must(http.StatusOK, http.MethodGet, tankPath(tankID, "/parameters"), nil).decode(&out)
	return out
}

func (tp TankParameters) find(t *testing.T, pid int64) TankParameter {
	t.Helper()
	for _, p := range tp.Parameters {
		if p.ParameterID == pid {
			return p
		}
	}
	t.Fatalf("parameter %d not in %+v", pid, tp)
	return TankParameter{}
}

func tankPath(tankID int64, suffix string) string {
	return "/api/v1/tanks/" + strconv.FormatInt(tankID, 10) + suffix
}

// tpPath is the path of one parameter's settings in a tank, with an optional suffix such as /series.
func tpPath(tankID, parameterID int64, suffix string) string {
	return tankPath(tankID, "/parameters/"+strconv.FormatInt(parameterID, 10)+suffix)
}

func near(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %v, want %v (tolerance %v)", what, got, want, tol)
	}
}

// is reports whether an optional value is set and equal to want.
func is[T comparable](p *T, want T) bool { return p != nil && *p == want }

func ptr[T any](v T) *T { return &v }

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// countingDB records every statement sent through it.
type countingDB struct {
	dbgen.DBTX
	statements []string
}

func (c *countingDB) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	c.statements = append(c.statements, q)
	return c.DBTX.ExecContext(ctx, q, args...)
}

func (c *countingDB) PrepareContext(ctx context.Context, q string) (*sql.Stmt, error) {
	c.statements = append(c.statements, q)
	return c.DBTX.PrepareContext(ctx, q)
}

func (c *countingDB) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	c.statements = append(c.statements, q)
	return c.DBTX.QueryContext(ctx, q, args...)
}

func (c *countingDB) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	c.statements = append(c.statements, q)
	return c.DBTX.QueryRowContext(ctx, q, args...)
}
