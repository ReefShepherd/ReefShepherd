package reminders

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/maintenance"
	"github.com/reefshepherd/reefshepherd/internal/notify"
	"github.com/reefshepherd/reefshepherd/internal/params"
	"github.com/reefshepherd/reefshepherd/internal/settings"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

// testRaiser raises through the real notification service, with hooks to make Raise fail, panic or run code while it is called.
type testRaiser struct {
	inner notify.Raiser
	// fail, when set, decides per event whether Raise returns an error instead of raising.
	fail func(notify.Event) error
	// during, when set, runs just before the event is raised.
	during func(notify.Event)
	// boom, when set, makes Raise panic with this value.
	boom any
}

func (r *testRaiser) Raise(ctx context.Context, e notify.Event) (bool, error) {
	if r.boom != nil {
		panic(r.boom)
	}
	if r.fail != nil {
		if err := r.fail(e); err != nil {
			return false, err
		}
	}
	if r.during != nil {
		r.during(e)
	}
	return r.inner.Raise(ctx, e)
}

// env is a reminders service on a real temporary database with a controllable clock.
type env struct {
	t      *testing.T
	db     *db.DB
	store  *settings.Store
	deps   Deps
	svc    *Service
	raiser *testRaiser
	logs   *bytes.Buffer
	// bare serves the params routes without the result hook and hooked serves them with the reminders wired in.
	bare, hooked http.Handler
	userID       int64
	now          time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	database := testutil.OpenDB(t)
	store := settings.NewStore(database)
	e := &env{t: t, db: database, store: store, logs: &bytes.Buffer{}, now: at("2026-10-24T12:00:00Z")}
	logger := slog.New(slog.NewTextHandler(e.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	e.raiser = &testRaiser{inner: notify.NewService(notify.Deps{DB: database, Settings: store, Logger: testutil.Logger(t)})}
	e.deps = Deps{
		DB: database, Settings: store, Logger: logger, Notify: e.raiser,
		Maintenance: maintenance.NewService(maintenance.Deps{DB: database, Settings: store, Logger: logger}),
		Params:      params.NewService(params.Deps{DB: database, Settings: store, Logger: logger}),
	}
	e.svc = NewService(e.deps)
	e.svc.now = func() time.Time { return e.now }

	bare, hooked := http.NewServeMux(), http.NewServeMux()
	params.Routes(bare, params.Deps{DB: database, Settings: store, Logger: logger})
	params.Routes(hooked, params.Deps{DB: database, Settings: store, Logger: logger, OnResultCreated: e.svc.ResultCreated})
	e.bare, e.hooked = bare, hooked
	e.userID = testutil.Insert(t, database, `INSERT INTO users (username, role) VALUES ('tester', 'admin')`)
	e.setTimezone("UTC")
	return e
}

func (e *env) setTimezone(name string) {
	e.t.Helper()
	if err := e.store.SetTimezone(e.t.Context(), name); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) setReminderTime(hhmm string) {
	e.t.Helper()
	if err := e.store.SetReminderDefaultTime(e.t.Context(), hhmm); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) addTank(name string) int64 {
	e.t.Helper()
	return testutil.Insert(e.t, e.db, `INSERT INTO tanks (name) VALUES (?)`, name)
}

func (e *env) archiveTank(id int64) {
	e.t.Helper()
	testutil.Exec(e.t, e.db, `UPDATE tanks SET archived_at = '2026-01-01T00:00:00Z' WHERE id = ?`, id)
}

// addTask inserts a task that is due once a day after its last completion, and for the first time at since; tank is nil for a whole-system task.
func (e *env) addTask(name string, tank any, since string) int64 {
	e.t.Helper()
	return testutil.Insert(e.t, e.db, `INSERT INTO maintenance_tasks (name, tank_id, schedule_kind, interval_days, schedule_since)
		VALUES (?, ?, 'after_last', 1, ?)`, name, tank, since)
}

// addWeeklyTask inserts a task due on Mondays at 09:00 local time, counted from since.
func (e *env) addWeeklyTask(name string, tank int64, since string) int64 {
	e.t.Helper()
	return testutil.Insert(e.t, e.db, `INSERT INTO maintenance_tasks (name, tank_id, schedule_kind, weekdays, due_time, schedule_since)
		VALUES (?, ?, 'weekly', 1, '09:00', ?)`, name, tank, since)
}

func (e *env) complete(task int64, doneAt string) {
	e.t.Helper()
	testutil.Exec(e.t, e.db, `INSERT INTO maintenance_completions (task_id, done_at) VALUES (?, ?)`, task, doneAt)
}

// presetID returns the id of a shipped parameter definition.
func (e *env) presetID(key string) int64 {
	e.t.Helper()
	var id int64
	if err := e.db.R.QueryRowContext(e.t.Context(), `SELECT id FROM parameter_definitions WHERE preset_key = ?`, key).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

// enableTest turns a parameter on in a tank, due once a day after its last test and for the first time at since, shown at position order.
func (e *env) enableTest(tank int64, key, since string, order int) int64 {
	e.t.Helper()
	id := e.presetID(key)
	testutil.Exec(e.t, e.db, `INSERT INTO tank_parameters (tank_id, parameter_id, schedule_kind, interval_days, schedule_since, sort_order)
		VALUES (?, ?, 'after_last', 1, ?, ?)`, tank, id, since, order)
	return id
}

// setRange sets a tank parameter's ideal range in canonical units, nil for no limit.
func (e *env) setRange(tank, parameter int64, low, high any) {
	e.t.Helper()
	testutil.Exec(e.t, e.db, `INSERT INTO tank_parameters (tank_id, parameter_id, ideal_low, ideal_high) VALUES (?, ?, ?, ?)
		ON CONFLICT (tank_id, parameter_id) DO UPDATE SET ideal_low = excluded.ideal_low, ideal_high = excluded.ideal_high`, tank, parameter, low, high)
}

// note is a stored notification.
type note struct {
	EventType, Severity, Title, Body, Link, SourceKind, Occurrence string
	TankID                                                         *int64
	SourceID                                                       int64
}

func (e *env) notes() []note {
	e.t.Helper()
	rows, err := e.db.R.QueryContext(e.t.Context(), `SELECT event_type, severity, title, body, link, tank_id, source_kind, source_id, occurrence FROM notifications ORDER BY id`)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []note
	for rows.Next() {
		var n note
		if err := rows.Scan(&n.EventType, &n.Severity, &n.Title, &n.Body, &n.Link, &n.TankID, &n.SourceKind, &n.SourceID, &n.Occurrence); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		e.t.Fatal(err)
	}
	return out
}

// withoutTank returns n with its tank id cleared, so notes can be compared with == once the tank id has been checked.
func withoutTank(n note) note {
	n.TankID = nil
	return n
}

// titles lists the stored notification titles in order.
func (e *env) titles() []string {
	var out []string
	for _, n := range e.notes() {
		out = append(out, n.Title)
	}
	return out
}

// tickMaintenance runs the maintenance reminders at now.
func (e *env) tickMaintenance(now string) {
	e.t.Helper()
	e.now = at(now)
	if err := e.svc.RaiseMaintenanceDue(e.t.Context(), e.now); err != nil {
		e.t.Fatalf("maintenance reminders at %s: %v", now, err)
	}
}

// tickTests runs the tests summaries at now.
func (e *env) tickTests(now string) {
	e.t.Helper()
	e.now = at(now)
	if err := e.svc.RaiseTestsDue(e.t.Context(), e.now); err != nil {
		e.t.Fatalf("tests summaries at %s: %v", now, err)
	}
}

// post sends a request to the params routes signed in as the test user.
func (e *env) post(h http.Handler, path string, body any) {
	e.t.Helper()
	testutil.ExpectStatus(e.t, testutil.DoJSON(e.t, h, http.MethodPost, path, body, testutil.AsUser(e.userID, httpx.RoleAdmin)), http.StatusCreated)
}

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
