package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/settings"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

// env is the dashboard routes on a real migrated database; requests are signed in as the user id passed to do.
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
	database := testutil.OpenDB(t)
	mux := http.NewServeMux()
	Routes(mux, Deps{DB: database, Settings: settings.NewStore(database), Logger: testutil.Logger(t)})
	e := &env{t: t, db: database, mux: mux}
	testutil.Exec(t, database, `INSERT INTO users (id, username) VALUES (?, 'alice'), (?, 'bob')`, userA, userB)
	return e
}

// tank inserts a tank and returns its id.
func (e *env) tank(name string) int64 {
	e.t.Helper()
	return testutil.Insert(e.t, e.db, `INSERT INTO tanks (name) VALUES (?)`, name)
}

// param inserts a custom parameter definition and returns its id.
func (e *env) param(name string) int64 {
	e.t.Helper()
	return testutil.Insert(e.t, e.db, `INSERT INTO parameter_definitions (name, sort_order) VALUES (?, 1000)`, name)
}

// enable switches a parameter on (or off) for a tank with the given tank-level sort order.
func (e *env) enable(tankID, paramID int64, enabled bool, sortOrder int) {
	e.t.Helper()
	flag := 0
	if enabled {
		flag = 1
	}
	testutil.Exec(e.t, e.db, `INSERT INTO tank_parameters (tank_id, parameter_id, enabled, sort_order) VALUES (?, ?, ?, ?)
		ON CONFLICT (tank_id, parameter_id) DO UPDATE SET enabled = excluded.enabled, sort_order = excluded.sort_order`, tankID, paramID, flag, sortOrder)
}

func (e *env) do(user int, method, target string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	var opts []testutil.ReqOpt
	if user != 0 {
		opts = append(opts, testutil.AsUser(int64(user), httpx.RoleMember))
	}
	return testutil.DoJSON(e.t, e.mux, method, target, body, opts...)
}

// get fetches the dashboard as a user and returns its cards.
func (e *env) get(user int, tankID int64) []Card {
	e.t.Helper()
	rec := e.do(user, http.MethodGet, dashPath(tankID, ""), nil)
	testutil.ExpectStatus(e.t, rec, http.StatusOK)
	return testutil.Decode[Layout](e.t, rec).Cards
}

// put replaces the dashboard as a user and returns the resulting cards.
func (e *env) put(user int, tankID int64, cards []map[string]any) []Card {
	e.t.Helper()
	rec := e.do(user, http.MethodPut, dashPath(tankID, ""), map[string]any{"cards": cards})
	testutil.ExpectStatus(e.t, rec, http.StatusOK)
	return testutil.Decode[Layout](e.t, rec).Cards
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
