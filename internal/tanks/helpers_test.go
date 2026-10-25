package tanks

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/settings"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
	"github.com/reefshepherd/reefshepherd/internal/units"
)

const (
	asAdmin  = "admin"
	asMember = "member"
)

// env is the tank routes on a real migrated database; requests are signed in as the role passed to do.
type env struct {
	t        *testing.T
	db       *db.DB
	settings *settings.Store
	mux      http.Handler
}

func newEnv(t *testing.T) *env {
	t.Helper()
	database := testutil.OpenDB(t)
	store := settings.NewStore(database)
	mux := http.NewServeMux()
	Routes(mux, Deps{DB: database, Settings: store, Logger: testutil.Logger(t)})
	return &env{t: t, db: database, settings: store, mux: mux}
}

func (e *env) do(role, method, path string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	var opts []testutil.ReqOpt
	if role != "" {
		opts = append(opts, testutil.AsUser(1, role))
	}
	return testutil.DoJSON(e.t, e.mux, method, path, body, opts...)
}

// admin sends the request as an admin.
func (e *env) admin(method, path string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.do(asAdmin, method, path, body)
}

// create posts a tank as an admin and returns it.
func (e *env) create(body map[string]any) Tank {
	e.t.Helper()
	rec := e.admin(http.MethodPost, "/api/v1/tanks", body)
	testutil.ExpectStatus(e.t, rec, http.StatusCreated)
	return testutil.Decode[Tank](e.t, rec)
}

func near(got, want, tol float64) bool { return math.Abs(got-want) <= tol }

func path(id int64, suffix string) string {
	return "/api/v1/tanks/" + strconv.FormatInt(id, 10) + suffix
}

func unitOf(s string) units.Unit { return units.Unit(s) }
