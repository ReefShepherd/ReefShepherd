package reminders

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/notify"
	"github.com/reefshepherd/reefshepherd/internal/params"
)

// result is a stored reading of a parameter measured at the given time.
func (e *env) result(tank, parameter int64, value float64, measuredAt time.Time) params.Result {
	e.t.Helper()
	id := insertResult(e, tank, parameter, value, measuredAt)
	return params.Result{ID: id, TankID: tank, ParameterID: parameter, MeasuredAt: measuredAt, Value: value, Source: "manual"}
}

func insertResult(e *env, tank, parameter int64, value float64, measuredAt time.Time) int64 {
	e.t.Helper()
	res, err := e.db.W.ExecContext(e.t.Context(), `INSERT INTO test_results (tank_id, parameter_id, measured_at, value) VALUES (?, ?, ?, ?)`,
		tank, parameter, measuredAt.UTC().Format(time.RFC3339), value)
	if err != nil {
		e.t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

func TestOutOfRangeAlertVariants(t *testing.T) {
	tests := []struct {
		name      string
		low, high any
		value     float64
		body      string
	}{
		{"below a two-sided range", 25.0, 27.0, 23, "Measured 23.0 °C, below the ideal range of 25.0 °C to 27.0 °C."},
		{"above a two-sided range", 25.0, 27.0, 29, "Measured 29.0 °C, above the ideal range of 25.0 °C to 27.0 °C."},
		{"below a range with only a minimum", 25.0, nil, 23, "Measured 23.0 °C, below the ideal minimum of 25.0 °C."},
		{"above a range with only a maximum", nil, 27.0, 29, "Measured 29.0 °C, above the ideal maximum of 27.0 °C."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			tank, temp := e.addTank("Display"), e.presetID("temperature")
			e.setRange(tank, temp, tc.low, tc.high)
			measured := e.now.Add(-time.Hour)
			r := e.result(tank, temp, tc.value, measured)

			e.svc.ResultCreated(t.Context(), r)

			got := e.notes()
			if len(got) != 1 {
				t.Fatalf("notifications = %+v, want one", got)
			}
			n := got[0]
			if n.TankID == nil || *n.TankID != tank {
				t.Errorf("tank = %v, want %d", n.TankID, tank)
			}
			want := note{
				EventType: "out_of_range", Severity: "warning", Title: "Temperature is out of range on Display", Body: tc.body,
				Link: "/tanks/1/charts/" + strconv.FormatInt(temp, 10), SourceKind: "test_result", SourceID: r.ID, Occurrence: "2026-10-24T11:00:00Z",
			}
			if withoutTank(n) != want {
				t.Errorf("alert = %+v, want %+v", withoutTank(n), want)
			}
		})
	}
}

func TestOutOfRangeAlertIgnoresInRangeResults(t *testing.T) {
	e := newEnv(t)
	tank, temp := e.addTank("Display"), e.presetID("temperature")
	e.setRange(tank, temp, 25.0, 27.0)
	for _, v := range []float64{25, 26, 27} {
		e.svc.ResultCreated(t.Context(), e.result(tank, temp, v, e.now))
	}
	if got := e.notes(); len(got) != 0 {
		t.Errorf("notifications = %+v, want none", got)
	}
}

func TestOutOfRangeAlertOnlyForResultsMeasuredInTheLastDay(t *testing.T) {
	e := newEnv(t)
	tank, temp := e.addTank("Display"), e.presetID("temperature")
	e.setRange(tank, temp, 25.0, 27.0)

	older := e.result(tank, temp, 20, e.now.Add(-24*time.Hour-time.Second))
	e.svc.ResultCreated(t.Context(), older)
	if got := e.notes(); len(got) != 0 {
		t.Fatalf("a result measured more than 24 hours ago alerted: %+v", got)
	}
	e.svc.ResultCreated(t.Context(), e.result(tank, temp, 20, e.now.Add(-24*time.Hour)))
	e.svc.ResultCreated(t.Context(), e.result(tank, temp, 20, e.now))
	if got := e.notes(); len(got) != 2 {
		t.Errorf("notifications = %d, want the two results from the last 24 hours", len(got))
	}
}

func TestOutOfRangeAlertIsRaisedOncePerResult(t *testing.T) {
	e := newEnv(t)
	tank, temp := e.addTank("Display"), e.presetID("temperature")
	e.setRange(tank, temp, 25.0, 27.0)
	r := e.result(tank, temp, 20, e.now)
	e.svc.ResultCreated(t.Context(), r)
	e.svc.ResultCreated(t.Context(), r)
	if n := len(e.notes()); n != 1 {
		t.Errorf("notifications = %d, want 1", n)
	}
}

func TestOutOfRangeAlertFailuresAreOnlyLogged(t *testing.T) {
	t.Run("raise error", func(t *testing.T) {
		e := newEnv(t)
		tank, temp := e.addTank("Display"), e.presetID("temperature")
		e.setRange(tank, temp, 25.0, 27.0)
		e.raiser.fail = func(notify.Event) error { return errors.New("store is full") }
		e.svc.ResultCreated(t.Context(), e.result(tank, temp, 20, e.now))
		if !strings.Contains(e.logs.String(), "out-of-range alert failed") || !strings.Contains(e.logs.String(), "store is full") {
			t.Errorf("log = %q, want the failure", e.logs)
		}
	})
	t.Run("unknown tank", func(t *testing.T) {
		e := newEnv(t)
		e.svc.ResultCreated(t.Context(), params.Result{ID: 1, TankID: 99, ParameterID: e.presetID("temperature"), MeasuredAt: e.now, Value: 20})
		if !strings.Contains(e.logs.String(), "out-of-range alert failed") {
			t.Errorf("log = %q, want the failure", e.logs)
		}
	})
	t.Run("panic", func(t *testing.T) {
		e := newEnv(t)
		tank, temp := e.addTank("Display"), e.presetID("temperature")
		e.setRange(tank, temp, 25.0, 27.0)
		e.raiser.boom = "kaboom"
		e.svc.ResultCreated(t.Context(), e.result(tank, temp, 20, e.now))
		if !strings.Contains(e.logs.String(), "out-of-range alert panicked") || !strings.Contains(e.logs.String(), "kaboom") {
			t.Errorf("log = %q, want the panic", e.logs)
		}
	})
}

// The hook fires from the real params routes, and the same routes without it raise nothing.
func TestOutOfRangeAlertFiresFromTheParamsHook(t *testing.T) {
	e := newEnv(t)
	tank, temp := e.addTank("Display"), e.presetID("temperature")
	e.setRange(tank, temp, 25.0, 27.0)
	body := map[string]any{
		"measured_at": "2026-10-24T11:30:00Z",
		"results":     []map[string]any{{"parameter_id": temp, "value": 22.0}},
	}

	e.post(e.bare, "/api/v1/tanks/1/results", body)
	if n := len(e.notes()); n != 0 {
		t.Fatalf("the params routes without the hook raised %d notifications", n)
	}
	e.post(e.hooked, "/api/v1/tanks/1/results", body)
	got := e.notes()
	if len(got) != 1 || got[0].EventType != "out_of_range" || got[0].Occurrence != "2026-10-24T11:30:00Z" {
		t.Errorf("notifications = %+v, want one out-of-range alert", got)
	}
}
