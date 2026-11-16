package reminders

import (
	"net/http"
	"slices"
	"strconv"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/notify"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

// summaries lists the stored tests-due notifications as "title: body".
func (e *env) summaries() []string {
	var out []string
	for _, n := range e.notes() {
		out = append(out, n.Title+": "+n.Body)
	}
	return out
}

func (e *env) addResult(tank, parameter int64, measuredAt string) int64 {
	e.t.Helper()
	return testutil.Insert(e.t, e.db, `INSERT INTO test_results (tank_id, parameter_id, measured_at, value) VALUES (?, ?, ?, 8)`, tank, parameter, measuredAt)
}

func (e *env) wantSummaries(want ...string) {
	e.t.Helper()
	if got := e.summaries(); !slices.Equal(got, want) {
		e.t.Fatalf("summaries = %q, want %q", got, want)
	}
}

// listedEnv is a New York tank with a reminder time of 09:00 (13:00 UTC in October) whose calcium test is overdue.
func listedEnv(t *testing.T) (e *env, tank int64) {
	e = newEnv(t)
	e.setTimezone("America/New_York")
	e.setReminderTime("09:00")
	tank = e.addTank("Display")
	e.enableTest(tank, "calcium", "2026-10-23T12:00:00Z", 1)
	return e, tank
}

func TestTestsSummaryListsAParameterWhoseDueTimeMovedBeforeThePreviousSummary(t *testing.T) {
	e, tank := listedEnv(t)
	alk := e.enableTest(tank, "alkalinity", "2026-10-01T00:00:00Z", 2)
	testutil.Exec(t, e.db, `UPDATE tank_parameters SET interval_days = 30 WHERE tank_id = ? AND parameter_id = ?`, tank, alk)
	e.addResult(tank, alk, "2026-10-20T12:00:00Z")

	// Alkalinity was last tested on the 20th with a 30-day schedule, so it is not due on the 24th.
	e.tickTests("2026-10-24T13:00:00Z")
	e.wantSummaries("1 test due on Display: Calcium")

	// A shorter schedule makes it due on the 22nd, before that summary.
	testutil.Exec(t, e.db, `UPDATE tank_parameters SET interval_days = 2 WHERE tank_id = ? AND parameter_id = ?`, tank, alk)
	e.tickTests("2026-10-25T13:00:00Z")
	e.wantSummaries("1 test due on Display: Calcium", "1 test due on Display: Alkalinity")

	// Unchanged, it is not listed again.
	e.tickTests("2026-10-26T13:00:00Z")
	e.tickTests("2026-10-27T13:00:00Z")
	e.wantSummaries("1 test due on Display: Calcium", "1 test due on Display: Alkalinity")
}

func TestTestsSummaryDoesNotRepeatAParameterDueExactlyAtThePreviousSummary(t *testing.T) {
	e, tank := listedEnv(t)
	// Due at 09:00 local time, the reminder time itself.
	e.enableTest(tank, "alkalinity", "2026-10-24T13:00:00Z", 2)

	e.tickTests("2026-10-24T13:00:00Z")
	e.wantSummaries("2 tests due on Display: Calcium, Alkalinity")
	for _, day := range []string{"25", "26", "27"} {
		e.tickTests("2026-10-" + day + "T13:00:00Z")
	}
	e.wantSummaries("2 tests due on Display: Calcium, Alkalinity")
}

func TestTestsSummaryListsAReenabledParameterWithAnEarlierDueTimeAgain(t *testing.T) {
	e, tank := listedEnv(t)
	calcium := e.presetID("calcium")

	e.tickTests("2026-10-24T13:00:00Z")
	// Switched off, it is not due and nothing is listed.
	testutil.Exec(t, e.db, `UPDATE tank_parameters SET enabled = 0 WHERE tank_id = ? AND parameter_id = ?`, tank, calcium)
	e.tickTests("2026-10-25T13:00:00Z")
	// Switched back on with the same baseline it is the same overdue test and stays quiet.
	testutil.Exec(t, e.db, `UPDATE tank_parameters SET enabled = 1 WHERE tank_id = ? AND parameter_id = ?`, tank, calcium)
	e.tickTests("2026-10-26T13:00:00Z")
	e.wantSummaries("1 test due on Display: Calcium")

	// Switched on with a baseline before the earlier summary, it is a new due time.
	testutil.Exec(t, e.db, `UPDATE tank_parameters SET schedule_since = '2026-10-20T00:00:00Z' WHERE tank_id = ? AND parameter_id = ?`, tank, calcium)
	e.tickTests("2026-10-27T13:00:00Z")
	e.wantSummaries("1 test due on Display: Calcium", "1 test due on Display: Calcium")
}

func TestTestsSummaryListsAParameterAgainWhenItsLatestResultIsDeleted(t *testing.T) {
	e, tank := listedEnv(t)
	alk := e.enableTest(tank, "alkalinity", "2026-10-01T00:00:00Z", 2)
	testutil.Exec(t, e.db, `UPDATE tank_parameters SET interval_days = 2 WHERE tank_id = ? AND parameter_id = ?`, tank, alk)
	e.addResult(tank, alk, "2026-10-20T12:00:00Z")
	latest := e.addResult(tank, alk, "2026-10-23T12:00:00Z")

	// Tested on the 23rd, alkalinity falls due on the 25th.
	e.tickTests("2026-10-24T13:00:00Z")
	e.tickTests("2026-10-25T13:00:00Z")
	e.wantSummaries("1 test due on Display: Calcium", "1 test due on Display: Alkalinity")

	// Without the latest result it was due on the 22nd, before that summary.
	testutil.Exec(t, e.db, `DELETE FROM test_results WHERE id = ?`, latest)
	e.tickTests("2026-10-26T13:00:00Z")
	e.wantSummaries("1 test due on Display: Calcium", "1 test due on Display: Alkalinity", "1 test due on Display: Alkalinity")
	e.tickTests("2026-10-27T13:00:00Z")
	e.wantSummaries("1 test due on Display: Calcium", "1 test due on Display: Alkalinity", "1 test due on Display: Alkalinity")
}

func TestTestsSummaryRecordsTheDueTimeThatWasListedNotTheOneAfterIt(t *testing.T) {
	e, tank := listedEnv(t)
	// Falls due 30 seconds after the tick, so it is not in this summary.
	e.enableTest(tank, "alkalinity", "2026-10-24T13:00:30Z", 2)
	calcium := e.presetID("calcium")
	// Calcium's baseline moves to just after the tick while the summary is being raised.
	e.raiser.during = func(notify.Event) {
		testutil.Exec(t, e.db, `UPDATE tank_parameters SET schedule_since = '2026-10-24T13:00:10Z' WHERE tank_id = ? AND parameter_id = ?`, tank, calcium)
	}

	e.tickTests("2026-10-24T13:00:00Z")
	e.wantSummaries("1 test due on Display: Calcium")

	// Both fell due after the first summary, so both are listed the next day.
	e.raiser.during = nil
	e.tickTests("2026-10-25T13:00:00Z")
	e.wantSummaries("1 test due on Display: Calcium", "2 tests due on Display: Calcium, Alkalinity")
	e.tickTests("2026-10-26T13:00:00Z")
	e.wantSummaries("1 test due on Display: Calcium", "2 tests due on Display: Calcium, Alkalinity")
}

func TestTestsSummaryAfterARestartMidDayLeavesLaterTestsForTheNextDay(t *testing.T) {
	e, tank := listedEnv(t)
	e.tickTests("2026-10-24T13:00:00Z")
	// Falls due at 11:00 local time, after the summary, then the server restarts.
	e.enableTest(tank, "alkalinity", "2026-10-24T15:00:00Z", 2)

	restarted := NewService(e.deps)
	if err := restarted.RaiseTestsDue(t.Context(), at("2026-10-24T16:00:00Z")); err != nil {
		t.Fatal(err)
	}
	e.wantSummaries("1 test due on Display: Calcium")
	if err := restarted.RaiseTestsDue(t.Context(), at("2026-10-25T13:00:00Z")); err != nil {
		t.Fatal(err)
	}
	e.wantSummaries("1 test due on Display: Calcium", "1 test due on Display: Alkalinity")
}

func TestTestsSummaryListsASecondParameterDueAtTheSameInstantAsAListedOne(t *testing.T) {
	e, tank := listedEnv(t)
	e.tickTests("2026-10-24T13:00:00Z")
	// Switched on later with the same baseline, so it is due at the instant calcium was listed for.
	e.enableTest(tank, "alkalinity", "2026-10-23T12:00:00Z", 2)

	e.tickTests("2026-10-25T13:00:00Z")
	e.wantSummaries("1 test due on Display: Calcium", "1 test due on Display: Alkalinity")
	e.tickTests("2026-10-26T13:00:00Z")
	e.wantSummaries("1 test due on Display: Calcium", "1 test due on Display: Alkalinity")
}

func TestDeletingAListedCustomParameterRemovesItsListedTests(t *testing.T) {
	e, tank := listedEnv(t)
	var def struct {
		ID int64 `json:"id"`
	}
	rec := testutil.DoJSON(t, e.bare, http.MethodPost, "/api/v1/parameters", map[string]any{"name": "Strontium"}, testutil.AsUser(e.userID, httpx.RoleAdmin))
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	testutil.DecodeInto(t, rec, &def)
	testutil.Exec(t, e.db, `INSERT INTO tank_parameters (tank_id, parameter_id, schedule_kind, interval_days, schedule_since, sort_order)
		VALUES (?, ?, 'after_last', 1, '2026-10-22T12:00:00Z', 2)`, tank, def.ID)

	e.tickTests("2026-10-24T13:00:00Z")
	e.wantSummaries("2 tests due on Display: Calcium, Strontium")
	if n := testutil.Count(t, e.db, `SELECT COUNT(*) FROM test_reminders WHERE parameter_id = ?`, def.ID); n != 1 {
		t.Fatalf("listed rows of the custom parameter = %d, want 1", n)
	}

	rec = testutil.DoJSON(t, e.bare, http.MethodDelete, "/api/v1/parameters/"+strconv.FormatInt(def.ID, 10), nil, testutil.AsUser(e.userID, httpx.RoleAdmin))
	testutil.ExpectStatus(t, rec, http.StatusNoContent)
	if n := testutil.Count(t, e.db, `SELECT COUNT(*) FROM test_reminders WHERE parameter_id = ?`, def.ID); n != 0 {
		t.Errorf("listed rows left after deleting the parameter = %d, want 0", n)
	}
	if n := testutil.Count(t, e.db, `SELECT COUNT(*) FROM test_reminders`); n != 1 {
		t.Errorf("rows of other parameters = %d, want calcium's 1", n)
	}
}
