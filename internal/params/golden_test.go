package params

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// goldenBody fails unless body is byte for byte the content of testdata/name, which pins the wire format.
func goldenBody(t *testing.T, name, body string) {
	t.Helper()
	want, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if body != string(want) {
		t.Errorf("%s changed\ngot:  %s\nwant: %s", name, body, want)
	}
}

func TestTankParametersAndScheduleJSONOnTheWire(t *testing.T) {
	e := newEnv(t)
	tank := e.addTank("Golden")
	put := func(pid int64, body map[string]any) string {
		return e.must(http.StatusOK, http.MethodPut, tpPath(tank, pid, ""), body).Body.String()
	}

	// One parameter per schedule kind, plus untouched presets and a custom one that show the defaults.
	temp, ph, alk, salinity, calcium := e.preset("temperature"), e.preset("ph"), e.preset("alkalinity"), e.preset("salinity"), e.preset("calcium")
	weekly := put(temp, map[string]any{
		"enabled": true, "ideal_low": 24, "ideal_high": 26, "sort_order": 3,
		"schedule": map[string]any{"kind": "weekly", "weekdays": 21, "interval_weeks": 2, "anchor_date": "2026-10-26", "due_time": "18:30"},
	})
	every := put(ph, map[string]any{"enabled": true, "schedule": map[string]any{"kind": "every_n_days", "interval_days": 2, "anchor_date": "2026-10-25"}})
	afterLast := put(alk, map[string]any{"enabled": true, "schedule": map[string]any{"kind": "after_last", "interval_days": 3}})
	monthly := put(salinity, map[string]any{"enabled": false, "schedule": map[string]any{"kind": "monthly", "month_day": 31, "due_time": "07:05"}})
	firstTime := put(calcium, map[string]any{"enabled": true})
	custom := e.customParameter(map[string]any{"name": "Iodine", "default_test_interval_days": 14})
	e.customParameter(map[string]any{"name": "Silicate"})
	e.addResults(tank, "2026-10-24T09:00:00Z", val(alk, 8.1), val(ph, 8.2))
	e.must(http.StatusOK, http.MethodPost, paramPath(e.preset("nitrite"), "/archive"), nil)

	goldenBody(t, "put_weekly.json", weekly)
	goldenBody(t, "put_every_n_days.json", every)
	goldenBody(t, "put_after_last.json", afterLast)
	goldenBody(t, "put_monthly.json", monthly)
	goldenBody(t, "put_first_time.json", firstTime)
	goldenBody(t, "put_custom_default.json", put(custom, map[string]any{"sort_order": 2}))
	goldenBody(t, "tank_parameters.json", e.must(http.StatusOK, http.MethodGet, tankPath(tank, "/parameters"), nil).Body.String())
}
