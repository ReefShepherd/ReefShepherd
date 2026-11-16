package maintenance

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/settings"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

// env serves the maintenance routes on a real temporary database with a controllable clock and a signed-in admin.
type env struct {
	t      *testing.T
	db     *db.DB
	store  *settings.Store
	svc    *Service
	mux    http.Handler
	now    time.Time
	userID int64
}

func newEnv(t *testing.T) *env {
	t.Helper()
	database := testutil.OpenDB(t)
	e := &env{t: t, db: database, store: settings.NewStore(database), now: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)}
	e.svc = NewService(Deps{DB: database, Settings: e.store, Logger: testutil.Logger(t)})
	e.svc.now = func() time.Time { return e.now }
	e.userID = testutil.Insert(t, database, `INSERT INTO users (username, display_name, role) VALUES ('tester', 'Tess', 'admin')`)
	mux := http.NewServeMux()
	e.svc.register(mux)
	e.mux = mux
	return e
}

func (e *env) addTank(name string) int64 {
	e.t.Helper()
	return testutil.Insert(e.t, e.db, `INSERT INTO tanks (name) VALUES (?)`, name)
}

func (e *env) archiveTank(id int64) {
	e.t.Helper()
	testutil.Exec(e.t, e.db, `UPDATE tanks SET archived_at = '2026-01-01T00:00:00Z' WHERE id = ?`, id)
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

// do sends a request signed in as the admin test user.
func (e *env) do(method, path string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	return testutil.DoJSON(e.t, e.mux, method, path, body, testutil.AsUser(e.userID, httpx.RoleAdmin))
}

// must sends a request and fails unless the status matches.
func (e *env) must(status int, method, path string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	r := e.do(method, path, body)
	if r.Code != status {
		e.t.Fatalf("%s %s: status=%d, want %d; body=%s", method, path, r.Code, status, r.Body)
	}
	return r
}

// fail sends a request and expects the given status and error code.
func (e *env) fail(status int, code, method, path string, body any) testutil.ErrorBody {
	e.t.Helper()
	return testutil.ExpectError(e.t, e.do(method, path, body), status, code)
}

// invalid expects a 422 validation error naming exactly the given fields, each written as field=code.
func (e *env) invalid(method, path string, body any, want ...string) {
	e.t.Helper()
	names := make([]string, len(want))
	for i, w := range want {
		names[i], _, _ = strings.Cut(w, "=")
	}
	b := testutil.ExpectError(e.t, e.do(method, path, body), http.StatusUnprocessableEntity, "validation", names...)
	for _, w := range want {
		field, code, _ := strings.Cut(w, "=")
		if b.Error.Fields[field] != code {
			e.t.Errorf("%s: code %q, want %q (fields %v)", field, b.Error.Fields[field], code, b.Error.Fields)
		}
	}
}

const tasksPath = "/api/v1/maintenance/tasks"

func taskPath(id int64, suffix string) string {
	return tasksPath + "/" + itoa(id) + suffix
}

func completionPath(id int64) string {
	return "/api/v1/maintenance/completions/" + itoa(id)
}

func presetPath(id int64) string {
	return presetsPath + "/" + itoa(id)
}

// afterLast is a "N days after last done" schedule at the default due time.
func afterLast(days int64) map[string]any {
	return map[string]any{"kind": "after_last", "interval_days": days}
}

// weeklyOn is a weekly schedule on the given weekday bits.
func weeklyOn(days int64) map[string]any {
	return map[string]any{"kind": "weekly", "weekdays": days}
}

// newTask creates a task through the API and returns it.
func (e *env) newTask(body map[string]any) Task {
	e.t.Helper()
	var t Task
	testutil.DecodeInto(e.t, e.must(http.StatusCreated, http.MethodPost, tasksPath, body), &t)
	return t
}

func (e *env) getTask(id int64) Task {
	e.t.Helper()
	var t Task
	testutil.DecodeInto(e.t, e.must(http.StatusOK, http.MethodGet, taskPath(id, ""), nil), &t)
	return t
}

// tasks lists tasks with a query string such as "?status=all".
func (e *env) tasks(query string) []Task {
	e.t.Helper()
	return testutil.DecodeItems[Task](e.t, e.must(http.StatusOK, http.MethodGet, tasksPath+query, nil))
}

func taskNames(tasks []Task) []string {
	names := make([]string, len(tasks))
	for i, t := range tasks {
		names[i] = t.Name
	}
	return names
}

func (e *env) complete(taskID int64, body any) CompleteResult {
	e.t.Helper()
	var r CompleteResult
	testutil.DecodeInto(e.t, e.must(http.StatusCreated, http.MethodPost, taskPath(taskID, "/complete"), body), &r)
	return r
}

// scheduleSince reads a task's stored baseline.
func (e *env) scheduleSince(id int64) string {
	e.t.Helper()
	var s string
	if err := e.db.R.QueryRowContext(e.t.Context(), `SELECT schedule_since FROM maintenance_tasks WHERE id = ?`, id).Scan(&s); err != nil {
		e.t.Fatal(err)
	}
	return s
}

func ptr[T any](v T) *T { return &v }

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func isStr(p *string, want string) bool { return p != nil && *p == want }

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// str shows an optional string in a failure message.
func str(p *string) string {
	if p == nil {
		return "<null>"
	}
	return *p
}
