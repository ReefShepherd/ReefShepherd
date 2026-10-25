package params

import (
	"context"
	"database/sql"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/settings"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
	"github.com/reefshepherd/reefshepherd/internal/units"
)

// env serves the parameter routes on a real temporary database with a controllable clock and a signed-in user.
type env struct {
	t      *testing.T
	db     *db.DB
	store  *settings.Store
	svc    *Service
	bare   http.Handler
	now    time.Time
	userID int64
}

func newEnv(t *testing.T) *env {
	t.Helper()
	database := testutil.OpenDB(t)
	e := &env{t: t, db: database, store: settings.NewStore(database), now: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)}
	e.svc = NewService(Deps{DB: database, Settings: e.store, Logger: testutil.Logger(t)})
	e.svc.now = func() time.Time { return e.now }
	e.userID = testutil.Insert(t, database, `INSERT INTO users (username, role) VALUES ('tester', 'admin')`)

	mux := http.NewServeMux()
	e.svc.register(mux)
	e.bare = mux
	return e
}

func (e *env) addTank(name string) int64 {
	e.t.Helper()
	return testutil.Insert(e.t, e.db, `INSERT INTO tanks (name) VALUES (?)`, name)
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

// do sends a request signed in as the admin test user.
func (e *env) do(method, path string, body any) *resp {
	e.t.Helper()
	return e.doOn(e.bare, method, path, body, testutil.AsUser(e.userID, httpx.RoleAdmin))
}

// doOn serves a request on h, which is anonymous unless opts sign it in.
func (e *env) doOn(h http.Handler, method, path string, body any, opts ...testutil.ReqOpt) *resp {
	e.t.Helper()
	return &resp{ResponseRecorder: testutil.DoJSON(e.t, h, method, path, body, opts...), t: e.t}
}

// doRaw sends a request with an unparsed body and the given content type.
func (e *env) doRaw(method, path, contentType, body string) *resp {
	e.t.Helper()
	return &resp{ResponseRecorder: testutil.DoRaw(e.t, e.bare, method, path, contentType, body, testutil.AsUser(e.userID, httpx.RoleAdmin)), t: e.t}
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
	testutil.DecodeInto(r.t, r.ResponseRecorder, v)
}

// fail sends a request and expects the given status and error code.
func (e *env) fail(status int, code, method, path string, body any) testutil.ErrorBody {
	e.t.Helper()
	return testutil.ExpectError(e.t, e.do(method, path, body).ResponseRecorder, status, code)
}

// invalid expects a 422 validation error naming exactly the given fields.
func (e *env) invalid(method, path string, body any, fields ...string) testutil.ErrorBody {
	e.t.Helper()
	return testutil.ExpectError(e.t, e.do(method, path, body).ResponseRecorder, http.StatusUnprocessableEntity, "validation", fields...)
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
