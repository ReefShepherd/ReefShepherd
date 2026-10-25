package params

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/units"
)

func TestFahrenheitEntryIsStoredAsCelsiusAndReturnedAsFahrenheit(t *testing.T) {
	e := newEnv(t)
	e.setUnit(units.Temperature, units.Fahrenheit)
	tank, temp := e.addTank("Display"), e.preset("temperature")

	created := e.addResults(tank, "2026-10-24T09:00:00Z", val(temp, 77))
	if len(created) != 1 || created[0].Value != 77 || created[0].Unit != "f" || created[0].UnitLabel != "°F" {
		t.Fatalf("created = %+v", created)
	}
	stored := e.storedValues()
	if len(stored) != 1 {
		t.Fatalf("stored = %v", stored)
	}
	near(t, "stored value in °C", stored[0], 25, 1e-9)

	var page ResultPage
	e.must(http.StatusOK, http.MethodGet, tankPath(tank, "/results"), nil).decode(&page)
	if len(page.Results) != 1 || page.Results[0].Value != 77 || page.Results[0].Unit != "f" {
		t.Errorf("listed = %+v", page.Results)
	}
	var series Series
	e.must(http.StatusOK, http.MethodGet, tpPath(tank, temp, "/series"), nil).decode(&series)
	if series.Unit != "f" || len(series.Points) != 1 || series.Points[0].Value != 77 {
		t.Errorf("series = %+v", series)
	}
	if got := e.tankParams(tank).find(t, temp); got.Latest == nil || got.Latest.Value != 77 || got.UnitLabel != "°F" {
		t.Errorf("latest = %+v", got)
	}
}

func TestSwitchingUnitsChangesOutputButNotStoredRows(t *testing.T) {
	e := newEnv(t)
	e.setUnit(units.Temperature, units.Fahrenheit)
	e.setUnit(units.Alkalinity, units.MeqPerL)
	tank, temp, alk := e.addTank("Switch"), e.preset("temperature"), e.preset("alkalinity")
	e.must(http.StatusOK, http.MethodPut, tpPath(tank, temp, ""), map[string]any{"enabled": true})
	e.addResults(tank, "2026-10-24T09:00:00Z", val(temp, 77), val(alk, 2.85))
	before := e.storedValues()
	var asF TankParameters
	e.must(http.StatusOK, http.MethodGet, tankPath(tank, "/parameters"), nil).decode(&asF)
	if got := asF.find(t, temp); got.Latest.Value != 77 || !is(got.IdealLow, 77) || !is(got.IdealHigh, 79.7) {
		t.Errorf("in °F: %+v low=%v high=%v", got.Latest, got.IdealLow, got.IdealHigh)
	}

	e.setUnit(units.Temperature, units.Celsius)
	e.setUnit(units.Alkalinity, units.DKH)
	var page ResultPage
	e.must(http.StatusOK, http.MethodGet, tankPath(tank, "/results"), nil).decode(&page)
	byParam := map[int64]ResultView{}
	for _, r := range page.Results {
		byParam[r.ParameterID] = r
	}
	if r := byParam[temp]; r.Value != 25 || r.Unit != "c" || r.UnitLabel != "°C" {
		t.Errorf("temperature after switch = %+v", r)
	}
	if r := byParam[alk]; r.Value != 8 || r.Unit != "dkh" {
		t.Errorf("alkalinity after switch = %+v", r)
	}
	var asC TankParameters
	e.must(http.StatusOK, http.MethodGet, tankPath(tank, "/parameters"), nil).decode(&asC)
	if got := asC.find(t, temp); !is(got.IdealLow, 25) || !is(got.IdealHigh, 26.5) {
		t.Errorf("range in °C: low=%v high=%v", got.IdealLow, got.IdealHigh)
	}
	if after := e.storedValues(); !slices.Equal(before, after) {
		t.Errorf("stored values changed: %v -> %v", before, after)
	}
}

func TestSalinityIsShownAsSGByDefault(t *testing.T) {
	e := newEnv(t)
	tank, sal := e.addTank("Salt"), e.preset("salinity")

	created := e.addResults(tank, "2026-10-24T09:00:00Z", val(sal, 1.026))
	if created[0].Unit != "sg" || created[0].UnitLabel != "SG" || created[0].Value != 1.026 {
		t.Fatalf("created = %+v", created[0])
	}
	wantPPT, err := units.SGToSalinity(1.026, units.DefaultSampleC, units.DefaultReferenceC)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "stored ppt", e.storedValues()[0], wantPPT, 1e-9)

	e.setUnit(units.Salinity, units.PPT)
	var page ResultPage
	e.must(http.StatusOK, http.MethodGet, tankPath(tank, "/results"), nil).decode(&page)
	if got := page.Results[0]; got.Unit != "ppt" || got.Value != units.Round(wantPPT, 1) {
		t.Errorf("in ppt = %+v", got)
	}
}

func TestValuesShowWithTheirUnitPrecision(t *testing.T) {
	e := newEnv(t)
	e.setUnit(units.Alkalinity, units.MeqPerL)
	e.setUnit(units.Phosphate, units.PPB)
	tank := e.addTank("Precision")
	custom := e.customParameter(map[string]any{"name": "Silicate", "unit_label": "ppm", "decimals": 3})

	created := e.addResults(tank, "2026-10-24T09:00:00Z",
		val(e.preset("temperature"), 25.04), // family unit: catalog precision 1
		val(e.preset("alkalinity"), 2.8528), // family unit: meq/L has 2 decimals
		val(e.preset("phosphate"), 31.4),    // family unit: ppb has 0 decimals
		val(e.preset("ph"), 8.234),          // fixed family: its own 1 decimal
		val(e.preset("calcium"), 421.6),     // fixed family: its own 0 decimals
		val(custom, 1.23456),                // custom parameter: its own 3 decimals
	)
	want := []float64{25, 2.85, 31, 8.2, 422, 1.235}
	for i, r := range created {
		if r.Value != want[i] {
			t.Errorf("%s shown as %v, want %v", r.ParameterName, r.Value, want[i])
		}
	}
	// Rounding is for display only: the stored value keeps what was entered.
	stored := e.storedValues()
	near(t, "stored pH", stored[3], 8.234, 1e-12)
	near(t, "stored alkalinity dKH", stored[1], 2.8528/0.3566, 1e-9)
}

func TestInRangeJudgesTheValueAsDisplayed(t *testing.T) {
	e := newEnv(t)
	tank, ph := e.addTank("Edge"), e.preset("ph")
	e.must(http.StatusOK, http.MethodPut, tpPath(tank, ph, ""),
		map[string]any{"enabled": true, "ideal_low": 7.9, "ideal_high": 8.4})

	// 8.43 is shown as 8.4, the top of the range, so it must not be flagged.
	created := e.addResults(tank, "2026-10-24T09:00:00Z", val(ph, 8.43))
	if created[0].Value != 8.4 || !created[0].InRange {
		t.Errorf("8.43 = %+v", created[0])
	}
	created = e.addResults(tank, "2026-10-24T10:00:00Z", val(ph, 8.46))
	if created[0].Value != 8.5 || created[0].InRange {
		t.Errorf("8.46 = %+v", created[0])
	}
}

func TestOverdueFlipsAtTheLocalDueTime(t *testing.T) {
	tests := []struct {
		name     string
		zone     string
		measured string
		due      string
		// notOverdue is a moment before the local due time that would already be past due if the schedule were read in UTC.
		notOverdue string
	}{
		{"New York", "America/New_York", "2026-10-20T15:00:00Z", "2026-10-23T13:00:00Z", "2026-10-23T10:00:00Z"},
		// 22:30 local on the 20th is already the 21st in UTC, and the due date counts local days.
		{"New York late evening", "America/New_York", "2026-10-21T02:30:00Z", "2026-10-23T13:00:00Z", "2026-10-23T10:00:00Z"},
		{"Auckland", "Pacific/Auckland", "2026-10-20T15:00:00Z", "2026-10-23T20:00:00Z", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.setTimezone(tc.zone)
			tank, temp := e.addTank("Due"), e.preset("temperature")
			e.must(http.StatusOK, http.MethodPut, tpPath(tank, temp, ""), map[string]any{
				"enabled":  true,
				"schedule": map[string]any{"kind": "after_last", "interval_days": 3, "due_time": "09:00"},
			})
			e.addResults(tank, tc.measured, val(temp, 25))

			check := func(now string, wantOverdue bool) {
				t.Helper()
				e.now = at(now)
				got := e.tankParams(tank).find(t, temp)
				if got.NextDue == nil || *got.NextDue != tc.due {
					t.Fatalf("next_due = %v, want %s", got.NextDue, tc.due)
				}
				if got.Overdue != wantOverdue {
					t.Errorf("overdue at %s = %v, want %v", now, got.Overdue, wantOverdue)
				}
			}
			due := at(tc.due)
			check(due.Add(-time.Second).Format(time.RFC3339), false)
			check(tc.due, false)
			check(due.Add(time.Second).Format(time.RFC3339), true)
			if tc.notOverdue != "" {
				check(tc.notOverdue, false)
			}
		})
	}
}

func TestNeverTestedParameterIsDueFromTheMomentItIsEnabled(t *testing.T) {
	e := newEnv(t)
	tank, temp := e.addTank("Fresh"), e.preset("temperature")
	got := e.tankParams(tank).find(t, temp)
	if got.Enabled || got.NextDue != nil || got.Overdue {
		t.Errorf("a parameter the tank does not use has no due time: %+v", got)
	}

	e.must(http.StatusOK, http.MethodPut, tpPath(tank, temp, ""), map[string]any{"enabled": true})
	check := func(now time.Time, wantOverdue bool) {
		t.Helper()
		e.now = now
		got := e.tankParams(tank).find(t, temp)
		if !is(got.NextDue, "2026-10-24T12:00:00Z") || got.Latest != nil || got.Overdue != wantOverdue {
			t.Errorf("at %s: next_due=%v latest=%v overdue=%v", now, got.NextDue, got.Latest, got.Overdue)
		}
	}
	check(at("2026-10-24T12:00:00Z"), false)
	check(at("2026-10-24T12:00:01Z"), true)
}

func TestWeeklyScheduleCountsLocalDaysAcrossDaylightSaving(t *testing.T) {
	e := newEnv(t)
	e.setTimezone("America/New_York")
	tank, ph := e.addTank("Weekly"), e.preset("ph")
	// 2026-10-24 12:00 UTC is Saturday 08:00 in New York.
	e.must(http.StatusOK, http.MethodPut, tpPath(tank, ph, ""), map[string]any{
		"enabled":  true,
		"schedule": map[string]any{"kind": "weekly", "weekdays": 64, "due_time": "08:30"},
	})
	got := e.tankParams(tank).find(t, ph)
	if !is(got.NextDue, "2026-10-25T12:30:00Z") || got.Overdue || got.Schedule.Kind != "weekly" || !is(got.Schedule.Weekdays, 64) {
		t.Fatalf("first Sunday: %+v next_due=%v", got.Schedule, got.NextDue)
	}

	// A Saturday test satisfies that Sunday, so the next one is the following Sunday, after clocks go back.
	e.addResults(tank, "2026-10-24T12:00:00Z", val(ph, 8.1))
	got = e.tankParams(tank).find(t, ph)
	if !is(got.NextDue, "2026-11-01T13:30:00Z") {
		t.Errorf("next_due = %v, want 2026-11-01T13:30:00Z", got.NextDue)
	}
}
