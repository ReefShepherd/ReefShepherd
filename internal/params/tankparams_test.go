package params

import (
	"net/http"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/units"
)

func TestTankParametersListsEveryActiveParameterWithDefaultsUntilConfigured(t *testing.T) {
	e := newEnv(t)
	tank := e.addTank("Defaults")
	tp := e.tankParams(tank)
	if tp.TankID != tank || tp.Timezone != "UTC" || len(tp.Parameters) != 10 {
		t.Fatalf("tank parameters = %+v", tp)
	}
	alk := tp.find(t, e.preset("alkalinity"))
	if alk.Configured || alk.Enabled || alk.Name != "Alkalinity" || alk.Unit != "dkh" || alk.UnitLabel != "dKH" || alk.Decimals != 1 ||
		!is(alk.IdealLow, 7) || !is(alk.IdealHigh, 10) || alk.Schedule.Kind != "after_last" || !is(alk.Schedule.IntervalDays, 7) ||
		alk.Schedule.DueTime != "09:00" || alk.Schedule.IntervalWeeks != 1 || alk.Latest != nil || alk.NextDue != nil || alk.Overdue {
		t.Errorf("alkalinity defaults = %+v", alk)
	}
	if tp.Parameters[0].Name != "Temperature" || tp.Parameters[9].Name != "Nitrite" {
		t.Errorf("order = %s ... %s", tp.Parameters[0].Name, tp.Parameters[9].Name)
	}
	nitrite := tp.find(t, e.preset("nitrite"))
	if nitrite.IdealLow != nil || !is(nitrite.IdealHigh, 0.1) {
		t.Errorf("nitrite range = %v..%v", nitrite.IdealLow, nitrite.IdealHigh)
	}
	// A custom parameter without a default interval starts with no schedule.
	custom := e.customParameter(map[string]any{"name": "Iodine"})
	if got := e.tankParams(tank).find(t, custom); got.Schedule.Kind != "none" || got.IdealLow != nil || got.Schedule.IntervalDays != nil {
		t.Errorf("custom defaults = %+v", got)
	}
	e.fail(http.StatusNotFound, "not_found", http.MethodGet, tankPath(9999, "/parameters"), nil)
	e.fail(http.StatusNotFound, "not_found", http.MethodGet, "/api/v1/tanks/abc/parameters", nil)
}

func TestTankParametersReadSettingsRowsFromThePresetInsert(t *testing.T) {
	e := newEnv(t)
	tank, nitrate := e.addTank("Preset"), e.preset("nitrate")
	e.exec(`INSERT INTO tank_parameters (tank_id, parameter_id, enabled, ideal_low, ideal_high, schedule_kind, interval_days, sort_order, schedule_since, updated_at)
		SELECT ?, id, 1, default_low, default_high, 'after_last', default_test_interval_days, sort_order, '2026-10-01T08:00:00Z', '2026-10-20T08:00:00Z'
		FROM parameter_definitions WHERE preset_key = 'nitrate'`, tank)

	got := e.tankParams(tank).find(t, nitrate)
	if !got.Configured || !got.Enabled || !is(got.IdealLow, 2) || !is(got.IdealHigh, 15) || got.Schedule.Kind != "after_last" ||
		!is(got.Schedule.IntervalDays, 7) || got.Schedule.DueTime != "09:00" {
		t.Errorf("nitrate = %+v", got)
	}
	// Never tested, so due from when it was enabled, and long overdue by the clock of 2026-10-24.
	if !is(got.NextDue, "2026-10-01T08:00:00Z") || !got.Overdue {
		t.Errorf("due = %v overdue = %v", got.NextDue, got.Overdue)
	}
}

func TestPutTankParameterEnablesAndMergesFields(t *testing.T) {
	e := newEnv(t)
	e.setUnit(units.Temperature, units.Fahrenheit)
	tank, temp := e.addTank("Put"), e.preset("temperature")
	other := e.addTank("Other")

	var got TankParameter
	e.must(http.StatusOK, http.MethodPut, tpPath(tank, temp, ""), map[string]any{
		"enabled": true, "ideal_low": 76, "ideal_high": 80, "sort_order": 3,
		"schedule": map[string]any{"kind": "every_n_days", "interval_days": 2, "anchor_date": "2026-10-25", "due_time": "18:30"},
	}).decode(&got)
	if !got.Configured || !got.Enabled || !is(got.IdealLow, 76) || !is(got.IdealHigh, 80) || got.SortOrder != 3 || got.Schedule.Kind != "every_n_days" ||
		!is(got.Schedule.IntervalDays, 2) || !is(got.Schedule.AnchorDate, "2026-10-25") || got.Schedule.DueTime != "18:30" || got.Schedule.IntervalWeeks != 1 {
		t.Fatalf("put = %+v", got)
	}
	var low, high float64
	if err := e.db.R.QueryRowContext(t.Context(), `SELECT ideal_low, ideal_high FROM tank_parameters WHERE tank_id = ? AND parameter_id = ?`, tank, temp).Scan(&low, &high); err != nil {
		t.Fatal(err)
	}
	near(t, "stored low °C", low, 24.4444444444, 1e-6)
	near(t, "stored high °C", high, 26.6666666667, 1e-6)

	// Fields left out keep their value; null clears a limit.
	e.must(http.StatusOK, http.MethodPut, tpPath(tank, temp, ""), map[string]any{"enabled": false, "ideal_low": nil}).decode(&got)
	if got.Enabled || got.IdealLow != nil || !is(got.IdealHigh, 80) || got.Schedule.Kind != "every_n_days" || got.SortOrder != 3 || got.NextDue != nil {
		t.Errorf("second put = %+v", got)
	}
	e.must(http.StatusOK, http.MethodPut, tpPath(tank, temp, ""), map[string]any{}).decode(&got)
	if got.Enabled || !is(got.IdealHigh, 80) {
		t.Errorf("empty put = %+v", got)
	}
	// Switching to a different schedule kind clears the fields the old one used.
	e.must(http.StatusOK, http.MethodPut, tpPath(tank, temp, ""), map[string]any{"schedule": map[string]any{"kind": "monthly", "month_day": 31}}).decode(&got)
	if got.Schedule.Kind != "monthly" || !is(got.Schedule.MonthDay, 31) || got.Schedule.IntervalDays != nil || got.Schedule.AnchorDate != nil || got.Schedule.DueTime != "09:00" {
		t.Errorf("monthly = %+v", got.Schedule)
	}
	// Another tank is untouched.
	if o := e.tankParams(other).find(t, temp); o.Configured {
		t.Errorf("other tank = %+v", o)
	}
	if n := e.count(`SELECT COUNT(*) FROM tank_parameters`); n != 1 {
		t.Errorf("%d tank_parameters rows, want 1", n)
	}
}

func TestPutTankParameterValidation(t *testing.T) {
	e := newEnv(t)
	tank, temp := e.addTank("Invalid"), e.preset("temperature")
	path := tpPath(tank, temp, "")
	e.must(http.StatusOK, http.MethodPut, path, map[string]any{"enabled": true, "ideal_low": 25, "ideal_high": 26})

	sched := func(s map[string]any) map[string]any { return map[string]any{"schedule": s} }
	tests := []struct {
		name   string
		body   map[string]any
		fields []string
	}{
		{"low above high", map[string]any{"ideal_low": 27}, []string{"ideal_low"}},
		{"high below the kept low", map[string]any{"ideal_high": 20}, []string{"ideal_low"}},
		{"low outside the physical range", map[string]any{"ideal_low": -500}, []string{"ideal_low"}},
		{"high wrong type", map[string]any{"ideal_high": "hot"}, []string{"ideal_high"}},
		{"sort order negative", map[string]any{"sort_order": -1}, []string{"sort_order"}},
		{"sort order huge", map[string]any{"sort_order": 1000000}, []string{"sort_order"}},
		{"enabled wrong type", map[string]any{"enabled": "yes"}, []string{"enabled"}},
		{"schedule without kind", sched(map[string]any{"interval_days": 3}), []string{"schedule.kind"}},
		{"unknown kind", sched(map[string]any{"kind": "sometimes"}), []string{"schedule"}},
		{"after_last without interval", sched(map[string]any{"kind": "after_last"}), []string{"schedule"}},
		{"interval zero", sched(map[string]any{"kind": "after_last", "interval_days": 0}), []string{"schedule"}},
		{"every_n_days without anchor", sched(map[string]any{"kind": "every_n_days", "interval_days": 3}), []string{"schedule"}},
		{"weekdays out of range", sched(map[string]any{"kind": "weekly", "weekdays": 128}), []string{"schedule"}},
		{"month day 32", sched(map[string]any{"kind": "monthly", "month_day": 32}), []string{"schedule"}},
		{"bad due time", sched(map[string]any{"kind": "none", "due_time": "9am"}), []string{"schedule"}},
		{"bad anchor date", sched(map[string]any{"kind": "every_n_days", "interval_days": 2, "anchor_date": "2026-13-40"}), []string{"schedule"}},
		{"unknown schedule field", sched(map[string]any{"kind": "none", "bogus": 1}), []string{"bogus"}},
		{"two problems", map[string]any{"sort_order": -1, "schedule": map[string]any{"kind": "weekly"}}, []string{"sort_order", "schedule"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e.invalid(http.MethodPut, path, tc.body, tc.fields...)
		})
	}
	// Nothing was applied by the rejected requests.
	if got := e.tankParams(tank).find(t, temp); !is(got.IdealLow, 25) || !is(got.IdealHigh, 26) || got.Schedule.Kind != "after_last" || got.SortOrder != 10 {
		t.Errorf("settings changed by rejected requests: %+v", got)
	}

	e.fail(http.StatusNotFound, "not_found", http.MethodPut, tpPath(9999, temp, ""), map[string]any{"enabled": true})
	e.fail(http.StatusNotFound, "not_found", http.MethodPut, tpPath(tank, 9999, ""), map[string]any{"enabled": true})
	e.fail(http.StatusNotFound, "not_found", http.MethodPut, tankPath(tank, "/parameters/abc"), map[string]any{"enabled": true})
	e.must(http.StatusOK, http.MethodPost, paramPath(temp, "/archive"), nil)
	e.fail(http.StatusConflict, "conflict", http.MethodPut, path, map[string]any{"enabled": false})
}

func TestStatusUsesTheTanksCurrentRange(t *testing.T) {
	e := newEnv(t)
	tank, alk := e.addTank("Range"), e.preset("alkalinity")
	e.must(http.StatusOK, http.MethodPut, tpPath(tank, alk, ""), map[string]any{"enabled": true, "ideal_low": 7, "ideal_high": 10})
	e.addResults(tank, "2026-10-24T09:00:00Z", val(alk, 8.1))

	if got := e.tankParams(tank).find(t, alk); got.Latest == nil || got.Latest.Value != 8.1 || !got.Latest.InRange || got.Latest.MeasuredAt != "2026-10-24T09:00:00Z" {
		t.Fatalf("latest = %+v", got.Latest)
	}
	// Narrowing the range flips the stored reading to out of range without touching it.
	e.must(http.StatusOK, http.MethodPut, tpPath(tank, alk, ""), map[string]any{"ideal_low": 8.5})
	got := e.tankParams(tank).find(t, alk)
	if got.Latest == nil || got.Latest.InRange || got.Latest.Value != 8.1 {
		t.Errorf("latest after narrowing = %+v", got.Latest)
	}
	var page ResultPage
	e.must(http.StatusOK, http.MethodGet, tankPath(tank, "/results"), nil).decode(&page)
	if page.Results[0].InRange {
		t.Error("the listed result still counts as in range")
	}
	var series Series
	e.must(http.StatusOK, http.MethodGet, tpPath(tank, alk, "/series"), nil).decode(&series)
	if series.Points[0].InRange {
		t.Error("the series point still counts as in range")
	}
	// A limit with no other side: only a high bound.
	e.must(http.StatusOK, http.MethodPut, tpPath(tank, alk, ""), map[string]any{"ideal_low": nil, "ideal_high": 8})
	if got := e.tankParams(tank).find(t, alk); got.Latest.InRange {
		t.Error("8.1 is above a high limit of 8")
	}
	e.must(http.StatusOK, http.MethodPut, tpPath(tank, alk, ""), map[string]any{"ideal_high": nil})
	if got := e.tankParams(tank).find(t, alk); !got.Latest.InRange || got.IdealLow != nil || got.IdealHigh != nil {
		t.Errorf("no limits = %+v", got)
	}
}

func TestLatestPerParameterIsOneStatement(t *testing.T) {
	e := newEnv(t)
	tank := e.addTank("Latest")
	temp, ph, calcium := e.preset("temperature"), e.preset("ph"), e.preset("calcium")
	for _, pid := range []int64{temp, ph, calcium} {
		e.must(http.StatusOK, http.MethodPut, tpPath(tank, pid, ""), map[string]any{"enabled": true})
	}
	insert := func(pid int64, measuredAt string, value float64) int64 {
		return e.insertID(`INSERT INTO test_results (tank_id, parameter_id, measured_at, value) VALUES (?, ?, ?, ?)`, tank, pid, measuredAt, value)
	}
	otherTank := e.addTank("Newer elsewhere")
	e.insertID(`INSERT INTO test_results (tank_id, parameter_id, measured_at, value) VALUES (?, ?, '2026-12-01T00:00:00Z', 99)`, otherTank, temp)
	insert(temp, "2026-10-20T10:00:00Z", 24)
	insert(temp, "2026-10-22T10:00:00Z", 25)
	insert(temp, "2026-10-21T10:00:00Z", 26) // entered last but measured earlier
	insert(ph, "2026-10-23T10:00:00Z", 8.1)
	tiedLater := insert(ph, "2026-10-23T10:00:00Z", 8.2) // same time: the later row wins

	counter := &countingDB{DBTX: e.svc.r}
	items, _, err := e.svc.tankParameters(t.Context(), counter, tank, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(counter.statements) != 1 {
		t.Errorf("%d statements for the tank parameters:\n%v", len(counter.statements), counter.statements)
	}
	byID := map[int64]TankParameter{}
	for _, it := range items {
		byID[it.ParameterID] = it
	}
	if l := byID[temp].Latest; l == nil || l.Value != 25 || l.MeasuredAt != "2026-10-22T10:00:00Z" {
		t.Errorf("temperature latest = %+v", l)
	}
	if l := byID[ph].Latest; l == nil || l.Value != 8.2 || l.ResultID != tiedLater {
		t.Errorf("pH latest = %+v", l)
	}
	if l := byID[calcium].Latest; l != nil {
		t.Errorf("calcium has no results but latest = %+v", l)
	}
}

func TestSettingsRowIsStampedWhenWritten(t *testing.T) {
	e := newEnv(t)
	tank, ph := e.addTank("Stamp"), e.preset("ph")
	e.now = time.Date(2026, 10, 24, 12, 0, 5, 700, time.UTC)
	e.must(http.StatusOK, http.MethodPut, tpPath(tank, ph, ""), map[string]any{"enabled": true})
	var stamp, since string
	if err := e.db.R.QueryRowContext(t.Context(), `SELECT updated_at, schedule_since FROM tank_parameters`).Scan(&stamp, &since); err != nil {
		t.Fatal(err)
	}
	if stamp != "2026-10-24T12:00:05Z" || since != "2026-10-24T12:00:05Z" {
		t.Errorf("updated_at = %q, schedule_since = %q", stamp, since)
	}

	// A row created without switching the parameter on is stamped as well.
	calcium := e.preset("calcium")
	e.must(http.StatusOK, http.MethodPut, tpPath(tank, calcium, ""), map[string]any{"sort_order": 5})
	if got := e.scheduleSince(tank, calcium); got != "2026-10-24T12:00:05Z" {
		t.Errorf("schedule_since of a disabled new row = %q", got)
	}
}

func (e *env) scheduleSince(tank, parameter int64) string {
	e.t.Helper()
	var since string
	if err := e.db.R.QueryRowContext(e.t.Context(), `SELECT schedule_since FROM tank_parameters WHERE tank_id = ? AND parameter_id = ?`, tank, parameter).Scan(&since); err != nil {
		e.t.Fatal(err)
	}
	return since
}

func TestNeverTestedBaselineMovesOnlyWhenEnabledOrRescheduled(t *testing.T) {
	e := newEnv(t)
	tank, ph := e.addTank("Baseline"), e.preset("ph")
	path := tpPath(tank, ph, "")
	sundayNine := map[string]any{"kind": "weekly", "weekdays": 64, "due_time": "09:00"}

	// Set up on Saturday, so the first Sunday 09:00 is the first due time.
	e.now = at("2026-10-24T12:00:00Z")
	e.must(http.StatusOK, http.MethodPut, path, map[string]any{"enabled": true, "schedule": sundayNine})
	e.now = at("2026-10-25T10:00:00Z")
	check := func(step, wantDue string, wantOverdue bool) {
		t.Helper()
		got := e.tankParams(tank).find(t, ph)
		if !is(got.NextDue, wantDue) || got.Overdue != wantOverdue {
			t.Errorf("%s: next_due = %v overdue = %v, want %s %v", step, got.NextDue, got.Overdue, wantDue, wantOverdue)
		}
	}
	check("after setup", "2026-10-25T09:00:00Z", true)

	for step, body := range map[string]map[string]any{
		"sort_order only":          {"sort_order": 3},
		"range only":               {"ideal_low": 7.9, "ideal_high": 8.4},
		"same schedule sent again": {"schedule": sundayNine},
		"enabled sent again":       {"enabled": true},
	} {
		e.must(http.StatusOK, http.MethodPut, path, body)
		check(step, "2026-10-25T09:00:00Z", true)
		if since := e.scheduleSince(tank, ph); since != "2026-10-24T12:00:00Z" {
			t.Errorf("%s: schedule_since = %q, want the setup time", step, since)
		}
	}

	// Switching off keeps the baseline, and switching on again restarts it.
	e.must(http.StatusOK, http.MethodPut, path, map[string]any{"enabled": false})
	if since := e.scheduleSince(tank, ph); since != "2026-10-24T12:00:00Z" {
		t.Errorf("after disabling: schedule_since = %q", since)
	}
	e.now = at("2026-10-26T10:00:00Z")
	e.must(http.StatusOK, http.MethodPut, path, map[string]any{"enabled": true})
	check("after re-enabling", "2026-11-01T09:00:00Z", false)

	// A different schedule restarts it as well.
	e.now = at("2026-11-02T10:00:00Z")
	e.must(http.StatusOK, http.MethodPut, path, map[string]any{"schedule": map[string]any{"kind": "weekly", "weekdays": 64, "due_time": "08:00"}})
	check("after rescheduling", "2026-11-08T08:00:00Z", false)
	if since := e.scheduleSince(tank, ph); since != "2026-11-02T10:00:00Z" {
		t.Errorf("after rescheduling: schedule_since = %q", since)
	}
}
