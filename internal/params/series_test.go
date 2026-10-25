package params

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

func TestSeriesReturnsPointsOldestFirstWithRangeAndUnit(t *testing.T) {
	e := newEnv(t)
	e.setUnitFahrenheit()
	tank, temp, ph := e.addTank("Series"), e.preset("temperature"), e.preset("ph")
	other := e.addTank("Other")
	e.must(http.StatusOK, http.MethodPut, tpPath(tank, temp, ""), map[string]any{"enabled": true, "ideal_low": 77, "ideal_high": 79.7})
	// Entered out of order, with a reading in another tank and one of another parameter that must not show up.
	e.addResults(tank, "2026-10-22T10:00:00Z", val(temp, 80.6), val(ph, 8))
	e.addResults(tank, "2026-10-20T10:00:00Z", val(temp, 75.2))
	e.addResults(tank, "2026-10-21T10:00:00Z", entry{ParameterID: temp, Value: ptr(78.8), Note: "after water change"})
	e.addResults(other, "2026-10-21T11:00:00Z", val(temp, 70))

	var s Series
	e.must(http.StatusOK, http.MethodGet, tpPath(tank, temp, "/series"), nil).decode(&s)
	if s.TankID != tank || s.ParameterID != temp || s.Name != "Temperature" || s.Unit != "f" || s.UnitLabel != "°F" || s.Decimals != 1 {
		t.Errorf("header = %+v", s)
	}
	if !is(s.Range.Low, 77) || !is(s.Range.High, 79.7) {
		t.Errorf("range = %+v", s.Range)
	}
	want := []struct {
		at      string
		value   float64
		inRange bool
		note    string
	}{{"2026-10-20T10:00:00Z", 75.2, false, ""}, {"2026-10-21T10:00:00Z", 78.8, true, "after water change"}, {"2026-10-22T10:00:00Z", 80.6, false, ""}}
	if len(s.Points) != len(want) {
		t.Fatalf("points = %+v", s.Points)
	}
	for i, w := range want {
		p := s.Points[i]
		if p.ResultID == 0 || p.MeasuredAt != w.at || p.Value != w.value || p.InRange != w.inRange || p.Note != w.note {
			t.Errorf("point %d = %+v, want %+v", i, p, w)
		}
	}

	var windowed Series
	e.must(http.StatusOK, http.MethodGet, tpPath(tank, temp, "/series?from=2026-10-21T10:00:00Z&to=2026-10-21T10:00:00Z"), nil).decode(&windowed)
	if len(windowed.Points) != 1 || windowed.Points[0].Value != 78.8 {
		t.Errorf("window = %+v", windowed.Points)
	}
	var fromOnly Series
	e.must(http.StatusOK, http.MethodGet, tpPath(tank, temp, "/series?from=2026-10-21T10:00:01Z"), nil).decode(&fromOnly)
	if len(fromOnly.Points) != 1 || fromOnly.Points[0].Value != 80.6 {
		t.Errorf("from only = %+v", fromOnly.Points)
	}
	var toOnly Series
	e.must(http.StatusOK, http.MethodGet, tpPath(tank, temp, "/series?to=2026-10-20T10:00:00Z"), nil).decode(&toOnly)
	if len(toOnly.Points) != 1 || toOnly.Points[0].Value != 75.2 {
		t.Errorf("to only = %+v", toOnly.Points)
	}

	// No readings, and no settings in this tank: an empty list rather than null, and the default range.
	raw := e.must(http.StatusOK, http.MethodGet, tpPath(tank, e.preset("calcium"), "/series"), nil)
	if !strings.Contains(raw.Body.String(), `"points":[]`) {
		t.Errorf("empty series body = %s", raw.Body)
	}
	var empty Series
	raw.decode(&empty)
	if !is(empty.Range.Low, 380) || !is(empty.Range.High, 450) || empty.UnitLabel != "ppm" {
		t.Errorf("default range = %+v", empty)
	}
	// A parameter with a one-sided range reports null for the missing side.
	var oneSided Series
	e.must(http.StatusOK, http.MethodGet, tpPath(tank, e.preset("nitrite"), "/series"), nil).decode(&oneSided)
	if oneSided.Range.Low != nil || !is(oneSided.Range.High, 0.1) {
		t.Errorf("one-sided range = %+v", oneSided.Range)
	}

	e.fail(http.StatusNotFound, "not_found", http.MethodGet, tpPath(tank, 9999, "/series"), nil)
	e.fail(http.StatusNotFound, "not_found", http.MethodGet, tpPath(9999, temp, "/series"), nil)
	e.invalid(http.MethodGet, tpPath(tank, temp, "/series?from=yesterday"), nil, "from")
	e.invalid(http.MethodGet, tpPath(tank, temp, "/series?from=2026-10-22T00:00:00Z&to=2026-10-21T00:00:00Z"), nil, "from")
	e.invalid(http.MethodGet, tpPath(tank, temp, "/series?to=soon"), nil, "to")
}

// keys returns the sorted field names of a decoded JSON object.
func keys(t *testing.T, v any) []string {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%v is not a JSON object", v)
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func wantKeys(t *testing.T, what string, v any, want ...string) {
	t.Helper()
	slices.Sort(want)
	if got := keys(t, v); !slices.Equal(got, want) {
		t.Errorf("%s keys = %v, want %v", what, got, want)
	}
}

func getJSON(e *env, path string) map[string]any {
	e.t.Helper()
	var out map[string]any
	e.must(http.StatusOK, http.MethodGet, path, nil).decode(&out)
	return out
}

// TestJSONShapes pins the field names and nesting of every response, which the web client depends on.
func TestJSONShapes(t *testing.T) {
	e := newEnv(t)
	tank, temp := e.addTank("Shapes"), e.preset("temperature")
	e.must(http.StatusOK, http.MethodPut, tpPath(tank, temp, ""), map[string]any{"enabled": true})
	e.addResults(tank, "2026-10-24T09:00:00Z", val(temp, 25.2))

	defs := getJSON(e, "/api/v1/parameters")
	wantKeys(t, "parameter list", defs, "parameters")
	wantKeys(t, "parameter", defs["parameters"].([]any)[0], "id", "preset_key", "name", "unit_family", "unit", "unit_label", "decimals",
		"default_low", "default_high", "default_test_interval_days", "sort_order", "archived_at")

	tp := getJSON(e, tankPath(tank, "/parameters"))
	wantKeys(t, "tank parameters", tp, "tank_id", "timezone", "parameters")
	param := tp["parameters"].([]any)[0].(map[string]any)
	wantKeys(t, "tank parameter", param, "parameter_id", "preset_key", "name", "unit_family", "unit", "unit_label", "decimals", "configured", "enabled",
		"ideal_low", "ideal_high", "schedule", "sort_order", "latest", "next_due", "overdue")
	wantKeys(t, "schedule", param["schedule"], "kind", "interval_days", "weekdays", "interval_weeks", "month_day", "anchor_date", "due_time")
	wantKeys(t, "latest", param["latest"], "result_id", "value", "measured_at", "in_range")

	page := getJSON(e, tankPath(tank, "/results"))
	wantKeys(t, "result page", page, "results", "next_cursor")
	wantKeys(t, "result", page["results"].([]any)[0], "id", "tank_id", "parameter_id", "parameter_name", "measured_at", "value", "unit", "unit_label",
		"in_range", "note", "source", "created_by", "created_at", "updated_at")

	series := getJSON(e, tpPath(tank, temp, "/series"))
	wantKeys(t, "series", series, "tank_id", "parameter_id", "name", "unit", "unit_label", "decimals", "range", "points")
	wantKeys(t, "series range", series["range"], "low", "high")
	wantKeys(t, "series point", series["points"].([]any)[0], "result_id", "measured_at", "value", "in_range", "note")

	var created map[string]any
	ph := e.preset("ph")
	e.must(http.StatusCreated, http.MethodPost, tankPath(tank, "/results"), map[string]any{"results": []entry{val(ph, 8)}}).decode(&created)
	wantKeys(t, "created results", created, "results")

	// An enabled parameter that was never tested has a null latest and still carries its due time.
	var fresh map[string]any
	e.must(http.StatusOK, http.MethodPut, tpPath(tank, e.preset("calcium"), ""), map[string]any{"enabled": true}).decode(&fresh)
	if v, present := fresh["latest"]; !present || v != nil {
		t.Errorf("latest of an untested parameter = %v (present %v)", v, present)
	}
}

func TestRoutesRegistersTheEndpointsOnAMux(t *testing.T) {
	e := newEnv(t)
	mux := http.NewServeMux()
	var hooked []Result
	Routes(mux, Deps{DB: e.db, Settings: e.store, OnResultCreated: func(_ context.Context, r Result) { hooked = append(hooked, r) }})
	signedIn := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(httpx.WithUser(r.Context(), httpx.User{ID: e.userID})))
	})
	tank, ph := e.addTank("Routes"), e.preset("ph")

	r := e.doOn(signedIn, http.MethodGet, "/api/v1/parameters", nil)
	if r.Code != http.StatusOK {
		t.Fatalf("GET parameters: %d %s", r.Code, r.Body)
	}
	r = e.doOn(signedIn, http.MethodPost, tankPath(tank, "/results"), map[string]any{"results": []entry{val(ph, 8)}})
	if r.Code != http.StatusCreated || len(hooked) != 1 {
		t.Fatalf("POST results: %d %s, hook calls %d", r.Code, r.Body, len(hooked))
	}
	var body struct {
		Results []ResultView `json:"results"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil || len(body.Results) != 1 {
		t.Fatalf("body %s: %v", r.Body, err)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/nothing-here", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("an unknown path = %d", rec.Code)
	}
}
