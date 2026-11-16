package params

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/tanks"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
	"github.com/reefshepherd/reefshepherd/internal/units"
)

// enable turns a parameter on in a tank with the given extra settings.
func (e *env) enable(tankID, parameterID int64, settings map[string]any) {
	e.t.Helper()
	body := map[string]any{"enabled": true}
	for k, v := range settings {
		body[k] = v
	}
	e.must(http.StatusOK, http.MethodPut, tpPath(tankID, parameterID, ""), body)
}

// afterLast is a schedule of n days after the last test at 08:00.
func afterLast(n int) map[string]any {
	return map[string]any{"schedule": map[string]any{"kind": "after_last", "interval_days": n, "due_time": "08:00"}}
}

func TestDueTestsListsEnabledScheduledParametersAtOrPastDue(t *testing.T) {
	e := newEnv(t)
	e.setTimezone("America/New_York")
	display, frag, old := e.addTank("Display"), e.addTank("Frag"), e.addTank("Old")
	temp, alk, calcium, nitrate := e.preset("temperature"), e.preset("alkalinity"), e.preset("calcium"), e.preset("nitrate")

	// Display: temperature was tested at 06:00 in New York and is next due two days later at 08:00 local (12:00 UTC); alkalinity was never tested.
	e.now = at("2026-10-24T12:00:00Z")
	e.enable(display, temp, afterLast(2))
	e.addResults(display, "2026-10-24T10:00:00Z", val(temp, 25))
	e.enable(display, alk, afterLast(3))
	// No schedule, switched off and never configured: none of these are listed.
	e.enable(display, calcium, map[string]any{"schedule": map[string]any{"kind": "none"}})
	e.enable(display, nitrate, afterLast(1))
	e.must(http.StatusOK, http.MethodPut, tpPath(display, nitrate, ""), map[string]any{"enabled": false})
	// An archived definition stays enabled on the tank and past due, yet is left out.
	e.enable(display, e.preset("ph"), afterLast(1))
	testutil.Exec(t, e.db, `UPDATE parameter_definitions SET archived_at = '2026-10-24T12:00:00Z' WHERE preset_key = 'ph'`)
	// Frag was enabled earlier, so its never-tested parameter has been due for days.
	e.now = at("2026-10-20T12:00:00Z")
	e.enable(frag, temp, afterLast(7))
	// An archived tank keeps its settings but is left out.
	e.enable(old, temp, afterLast(1))
	testutil.Exec(t, e.db, `UPDATE tanks SET archived_at = '2026-10-21T00:00:00Z' WHERE id = ?`, old)

	ctx := t.Context()
	got, err := e.svc.DueTests(ctx, at("2026-10-26T12:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	want := []DueParameter{
		{TankID: display, TankName: "Display", ParameterID: temp, Name: "Temperature", DueAt: at("2026-10-26T12:00:00Z")},
		{TankID: display, TankName: "Display", ParameterID: alk, Name: "Alkalinity", DueAt: at("2026-10-24T12:00:00Z")},
		{TankID: frag, TankName: "Frag", ParameterID: temp, Name: "Temperature", DueAt: at("2026-10-20T12:00:00Z")},
	}
	if len(got) != len(want) {
		t.Fatalf("DueTests = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("DueTests[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	// One second earlier the tested temperature is not yet due; the never-tested parameters still are.
	got, err = e.svc.DueTests(ctx, at("2026-10-26T11:59:59Z"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ParameterID != alk || got[1].TankID != frag {
		t.Errorf("DueTests a second early = %+v", got)
	}

	// Before anything is due the list is empty.
	if got, err = e.svc.DueTests(ctx, at("2026-10-19T00:00:00Z")); err != nil || len(got) != 0 {
		t.Errorf("DueTests before any due time = %+v, %v", got, err)
	}
}

func TestRangeAlertJudgesAgainstTheTanksRangeInDisplayUnits(t *testing.T) {
	e := newEnv(t)
	e.setUnit(units.Temperature, units.Fahrenheit)
	tank := e.addTank("Display")
	temp, alk, nitrate, ammonia := e.preset("temperature"), e.preset("alkalinity"), e.preset("nitrate"), e.preset("ammonia")
	e.enable(tank, alk, map[string]any{"ideal_low": 8, "ideal_high": 9})
	e.enable(tank, nitrate, map[string]any{"ideal_low": nil, "ideal_high": nil})

	tests := []struct {
		name      string
		parameter int64
		value     float64 // canonical
		want      RangeAlert
		alerts    bool
	}{
		// 7.5 dKH is inside the default 7 to 11 but under the tank's own 8 to 9.
		{"below the tank range", alk, 7.5, RangeAlert{ParameterName: "Alkalinity", Value: "7.5 dKH", Low: "8.0 dKH", High: "9.0 dKH", Below: true}, true},
		{"above the tank range", alk, 10, RangeAlert{ParameterName: "Alkalinity", Value: "10.0 dKH", Low: "8.0 dKH", High: "9.0 dKH"}, true},
		{"inside the tank range", alk, 8.5, RangeAlert{}, false},
		{"exactly on a limit", alk, 9, RangeAlert{}, false},
		// 9.04 dKH is shown as 9.0, the limit, so it is not outside.
		{"rounds onto the limit", alk, 9.04, RangeAlert{}, false},
		{"no limits", nitrate, 999, RangeAlert{}, false},
		// A parameter without settings in the tank uses the definition's range, which for ammonia has only an upper limit.
		{"one-sided above", ammonia, 0.25, RangeAlert{ParameterName: "Ammonia", Value: "0.25 ppm", High: "0.10 ppm"}, true},
		{"one-sided inside", ammonia, 0.05, RangeAlert{}, false},
		// 30 °C is 86.0 °F against the default 24.5 to 27.5 °C, which is 76.1 to 81.5 °F.
		{"display unit follows the instance", temp, 30, RangeAlert{ParameterName: "Temperature", Value: "86.0 °F", Low: "76.1 °F", High: "81.5 °F"}, true},
		{"display unit below", temp, 20, RangeAlert{ParameterName: "Temperature", Value: "68.0 °F", Low: "76.1 °F", High: "81.5 °F", Below: true}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok, err := e.svc.RangeAlert(t.Context(), Result{TankID: tank, ParameterID: tc.parameter, Value: tc.value})
			if err != nil {
				t.Fatal(err)
			}
			tc.want.TankID, tc.want.TankName, tc.want.ParameterID = tank, "Display", tc.parameter
			if ok != tc.alerts {
				t.Fatalf("alerts = %v (%+v), want %v", ok, got, tc.alerts)
			}
			if ok && got != tc.want {
				t.Errorf("alert = %+v, want %+v", got, tc.want)
			}
			if !ok && got != (RangeAlert{}) {
				t.Errorf("no alert but got %+v", got)
			}
		})
	}
}

// The alert and the in_range flag of the results API must always agree.
func TestRangeAlertAgreesWithTheResultInRangeFlag(t *testing.T) {
	e := newEnv(t)
	tank, temp := e.addTank("Display"), e.preset("temperature")
	e.enable(tank, temp, map[string]any{"ideal_low": 25, "ideal_high": 27})
	for _, v := range []float64{24.9, 24.96, 25, 26.5, 27, 27.04, 27.1, 40} {
		stored := e.addResults(tank, "", val(temp, v))[0]
		_, ok, err := e.svc.RangeAlert(t.Context(), Result{TankID: tank, ParameterID: temp, Value: v})
		if err != nil {
			t.Fatal(err)
		}
		if ok == stored.InRange {
			t.Errorf("value %v: alert = %v but in_range = %v", v, ok, stored.InRange)
		}
	}
}

func TestRangeAlertRefusesUnknownTankAndParameter(t *testing.T) {
	e := newEnv(t)
	tank, temp := e.addTank("Display"), e.preset("temperature")
	if _, _, err := e.svc.RangeAlert(t.Context(), Result{TankID: tank + 99, ParameterID: temp, Value: 20}); !errors.Is(err, tanks.ErrNotFound) {
		t.Errorf("unknown tank error = %v", err)
	}
	if _, _, err := e.svc.RangeAlert(t.Context(), Result{TankID: tank, ParameterID: temp + 9999, Value: 20}); !errors.Is(err, errNoParameter) {
		t.Errorf("unknown parameter error = %v", err)
	}
}

func TestDueTestsReadsTheClockFromItsArgument(t *testing.T) {
	e := newEnv(t)
	tank, temp := e.addTank("Display"), e.preset("temperature")
	e.enable(tank, temp, afterLast(1))
	// The service clock is far in the past of the argument and must not matter.
	e.svc.now = func() time.Time { return at("2000-01-01T00:00:00Z") }
	got, err := e.svc.DueTests(t.Context(), at("2026-10-30T00:00:00Z"))
	if err != nil || len(got) != 1 {
		t.Errorf("DueTests = %+v, %v", got, err)
	}
}
