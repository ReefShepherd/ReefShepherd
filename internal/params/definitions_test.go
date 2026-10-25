package params

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/units"
)

func paramPath(id int64, suffix string) string {
	return "/api/v1/parameters/" + strconv.FormatInt(id, 10) + suffix
}

// customParameter creates a parameter through the API and returns its id.
func (e *env) customParameter(body map[string]any) int64 {
	e.t.Helper()
	var def Definition
	e.must(http.StatusCreated, http.MethodPost, "/api/v1/parameters", body).decode(&def)
	return def.ID
}

func (e *env) definitions(query string) []Definition {
	e.t.Helper()
	var out struct {
		Parameters []Definition `json:"parameters"`
	}
	e.must(http.StatusOK, http.MethodGet, "/api/v1/parameters"+query, nil).decode(&out)
	return out.Parameters
}

func TestListShipsThePresetsInOrderWithDisplayDefaults(t *testing.T) {
	e := newEnv(t)
	e.setUnit(units.Temperature, units.Fahrenheit)
	defs := e.definitions("")
	if len(defs) != 10 || defs[0].PresetKey == nil || *defs[0].PresetKey != "temperature" || defs[9].Name != "Nitrite" {
		t.Fatalf("defs = %+v", defs)
	}
	temp := defs[0]
	if temp.Unit != "f" || temp.UnitLabel != "°F" || temp.Decimals != 1 || !is(temp.DefaultLow, 76.1) || !is(temp.DefaultHigh, 81.5) || !is(temp.DefaultTestIntervalDays, 7) {
		t.Errorf("temperature = %+v", temp)
	}
	var calcium Definition
	for _, d := range defs {
		if d.PresetKey != nil && *d.PresetKey == "calcium" {
			calcium = d
		}
	}
	if calcium.UnitFamily != "fixed" || calcium.Unit != "" || calcium.UnitLabel != "ppm" || calcium.Decimals != 0 {
		t.Errorf("calcium = %+v", calcium)
	}
}

func TestCreateCustomParameter(t *testing.T) {
	e := newEnv(t)
	var def Definition
	e.must(http.StatusCreated, http.MethodPost, "/api/v1/parameters", map[string]any{
		"name": "  Silicate  ", "unit_label": "ppm", "decimals": 3,
		"default_low": 0, "default_high": 0.5, "default_test_interval_days": 14,
	}).decode(&def)
	if def.ID == 0 || def.PresetKey != nil || def.Name != "Silicate" || def.UnitFamily != "fixed" || def.UnitLabel != "ppm" ||
		def.Decimals != 3 || !is(def.DefaultLow, 0) || !is(def.DefaultHigh, 0.5) || !is(def.DefaultTestIntervalDays, 14) || def.ArchivedAt != nil {
		t.Errorf("created = %+v", def)
	}
	if def.SortOrder <= 100 {
		t.Errorf("a custom parameter sorts after the presets, sort_order = %d", def.SortOrder)
	}

	// Minimal body: decimals default to 2 and there are no defaults.
	var minimal Definition
	e.must(http.StatusCreated, http.MethodPost, "/api/v1/parameters", map[string]any{"name": "ORP"}).decode(&minimal)
	if minimal.Decimals != 2 || minimal.UnitLabel != "" || minimal.DefaultLow != nil || minimal.DefaultHigh != nil || minimal.DefaultTestIntervalDays != nil || minimal.SortOrder <= def.SortOrder {
		t.Errorf("minimal = %+v", minimal)
	}

	defs := e.definitions("")
	if len(defs) != 12 || defs[10].ID != def.ID || defs[11].ID != minimal.ID {
		t.Errorf("list = %d definitions, tail %+v", len(defs), defs[len(defs)-2:])
	}
}

func TestCreateParameterFieldValidation(t *testing.T) {
	e := newEnv(t)
	tests := []struct {
		name   string
		body   map[string]any
		fields []string
	}{
		{"blank name", map[string]any{"name": "  "}, []string{"name"}},
		{"missing name", map[string]any{"unit_label": "ppm"}, []string{"name"}},
		{"long name", map[string]any{"name": repeat("x", 61)}, []string{"name"}},
		{"duplicate of a preset, ignoring case", map[string]any{"name": "pH"}, []string{"name"}},
		{"long label", map[string]any{"name": "A", "unit_label": repeat("u", 17)}, []string{"unit_label"}},
		{"decimals too high", map[string]any{"name": "A", "decimals": 5}, []string{"decimals"}},
		{"decimals negative", map[string]any{"name": "A", "decimals": -1}, []string{"decimals"}},
		{"low above high", map[string]any{"name": "A", "default_low": 5, "default_high": 1}, []string{"default_low"}},
		{"interval zero", map[string]any{"name": "A", "default_test_interval_days": 0}, []string{"default_test_interval_days"}},
		{"interval huge", map[string]any{"name": "A", "default_test_interval_days": 99999}, []string{"default_test_interval_days"}},
		{"several at once", map[string]any{"name": "", "decimals": 9, "default_test_interval_days": -3}, []string{"name", "decimals", "default_test_interval_days"}},
		{"wrong type", map[string]any{"name": "A", "decimals": "two"}, []string{"decimals"}},
		{"family is not settable", map[string]any{"name": "A", "unit_family": "temperature"}, []string{"unit_family"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e.invalid(http.MethodPost, "/api/v1/parameters", tc.body, tc.fields...)
		})
	}
	if n := e.count(`SELECT COUNT(*) FROM parameter_definitions`); n != 10 {
		t.Errorf("%d definitions after rejected requests, want the 10 presets", n)
	}
	e.customParameter(map[string]any{"name": "Taken"})
	e.invalid(http.MethodPost, "/api/v1/parameters", map[string]any{"name": "TAKEN"}, "name")
}

func repeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for range n {
		out = append(out, s...)
	}
	return string(out)
}

func TestPatchParameter(t *testing.T) {
	e := newEnv(t)
	id := e.customParameter(map[string]any{
		"name": "Silicate", "unit_label": "ppm", "decimals": 1, "default_low": 0.1, "default_high": 0.5, "default_test_interval_days": 14,
	})

	var def Definition
	e.must(http.StatusOK, http.MethodPatch, paramPath(id, ""), map[string]any{"name": "Silica", "unit_label": "mg/L", "decimals": 2, "default_low": nil}).decode(&def)
	if def.Name != "Silica" || def.UnitLabel != "mg/L" || def.Decimals != 2 || def.DefaultLow != nil || !is(def.DefaultHigh, 0.5) || !is(def.DefaultTestIntervalDays, 14) {
		t.Errorf("patched = %+v", def)
	}
	// Fields left out stay as they were, and null clears.
	e.must(http.StatusOK, http.MethodPatch, paramPath(id, ""), map[string]any{"default_test_interval_days": nil}).decode(&def)
	if def.Name != "Silica" || !is(def.DefaultHigh, 0.5) || def.DefaultTestIntervalDays != nil {
		t.Errorf("second patch = %+v", def)
	}
	e.must(http.StatusOK, http.MethodPatch, paramPath(id, ""), map[string]any{}).decode(&def)
	if def.Name != "Silica" {
		t.Errorf("empty patch = %+v", def)
	}

	e.invalid(http.MethodPatch, paramPath(id, ""), map[string]any{"name": ""}, "name")
	e.invalid(http.MethodPatch, paramPath(id, ""), map[string]any{"default_low": 9}, "default_low")
	e.invalid(http.MethodPatch, paramPath(id, ""), map[string]any{"name": "pH"}, "name")
	e.invalid(http.MethodPatch, paramPath(id, ""), map[string]any{"default_low": "low"}, "default_low")
	e.fail(http.StatusNotFound, "not_found", http.MethodPatch, paramPath(9999, ""), map[string]any{"name": "x"})
	e.fail(http.StatusNotFound, "not_found", http.MethodPatch, "/api/v1/parameters/abc", map[string]any{"name": "x"})
	// Renaming a parameter to its own name is not a clash.
	e.must(http.StatusOK, http.MethodPatch, paramPath(id, ""), map[string]any{"name": "SILICA"})
}

func TestPatchPresetDefaultsUseDisplayUnits(t *testing.T) {
	e := newEnv(t)
	e.setUnit(units.Temperature, units.Fahrenheit)
	temp := e.preset("temperature")

	var def Definition
	e.must(http.StatusOK, http.MethodPatch, paramPath(temp, ""), map[string]any{"default_low": 77, "default_high": 80.6}).decode(&def)
	if !is(def.DefaultLow, 77) || !is(def.DefaultHigh, 80.6) {
		t.Errorf("patched = %+v", def)
	}
	var low, high float64
	if err := e.db.R.QueryRowContext(t.Context(), `SELECT default_low, default_high FROM parameter_definitions WHERE id = ?`, temp).Scan(&low, &high); err != nil {
		t.Fatal(err)
	}
	near(t, "stored low °C", low, 25, 1e-9)
	near(t, "stored high °C", high, 27, 1e-9)

	// A preset can be renamed, but a unit family parameter keeps the catalog label and precision.
	e.must(http.StatusOK, http.MethodPatch, paramPath(temp, ""), map[string]any{"name": "Water temp"})
	e.invalid(http.MethodPatch, paramPath(temp, ""), map[string]any{"unit_label": "deg", "decimals": 3}, "unit_label", "decimals")
	e.invalid(http.MethodPatch, paramPath(temp, ""), map[string]any{"default_low": 9999}, "default_low")
}

func TestArchiveHidesAParameterWithoutLosingItsResults(t *testing.T) {
	e := newEnv(t)
	tank := e.addTank("Archive")
	id := e.customParameter(map[string]any{"name": "Iodine", "unit_label": "ppb"})
	e.must(http.StatusOK, http.MethodPut, tpPath(tank, id, ""), map[string]any{"enabled": true})
	e.addResults(tank, "2026-10-24T09:00:00Z", val(id, 60))

	var def Definition
	e.must(http.StatusOK, http.MethodPost, paramPath(id, "/archive"), nil).decode(&def)
	if !is(def.ArchivedAt, "2026-10-24T12:00:00Z") {
		t.Fatalf("archived = %+v", def)
	}
	// Archiving again keeps the first time.
	e.now = e.now.Add(time.Hour)
	e.must(http.StatusOK, http.MethodPost, paramPath(id, "/archive"), nil).decode(&def)
	if !is(def.ArchivedAt, "2026-10-24T12:00:00Z") {
		t.Errorf("archived again = %+v", def)
	}

	if defs := e.definitions(""); len(defs) != 10 {
		t.Errorf("archived parameter still listed: %d definitions", len(defs))
	}
	if defs := e.definitions("?include_archived=true"); len(defs) != 11 || defs[10].ID != id {
		t.Errorf("include_archived: %d definitions", len(defs))
	}
	for _, p := range e.tankParams(tank).Parameters {
		if p.ParameterID == id {
			t.Error("archived parameter is still offered for the tank")
		}
	}
	if n := e.count(`SELECT COUNT(*) FROM test_results WHERE parameter_id = ?`, id); n != 1 {
		t.Errorf("%d results kept, want 1", n)
	}
	// History stays readable and the series still works.
	var series Series
	e.must(http.StatusOK, http.MethodGet, tpPath(tank, id, "/series"), nil).decode(&series)
	if len(series.Points) != 1 || series.Points[0].Value != 60 || series.UnitLabel != "ppb" {
		t.Errorf("series = %+v", series)
	}
	var page ResultPage
	e.must(http.StatusOK, http.MethodGet, tankPath(tank, "/results"), nil).decode(&page)
	if len(page.Results) != 1 || page.Results[0].ParameterName != "Iodine" {
		t.Errorf("results = %+v", page.Results)
	}

	e.must(http.StatusOK, http.MethodPost, paramPath(id, "/unarchive"), nil).decode(&def)
	if def.ArchivedAt != nil {
		t.Errorf("unarchived = %+v", def)
	}
	e.must(http.StatusOK, http.MethodPost, paramPath(id, "/unarchive"), nil)
	if defs := e.definitions(""); len(defs) != 11 {
		t.Errorf("unarchived parameter missing: %d definitions", len(defs))
	}
	e.fail(http.StatusNotFound, "not_found", http.MethodPost, paramPath(9999, "/archive"), nil)
	e.fail(http.StatusNotFound, "not_found", http.MethodPost, paramPath(9999, "/unarchive"), nil)
	e.invalid(http.MethodGet, "/api/v1/parameters?include_archived=maybe", nil, "include_archived")
}

func TestDeleteOnlyWithoutResults(t *testing.T) {
	e := newEnv(t)
	tank := e.addTank("Delete")
	unused := e.customParameter(map[string]any{"name": "Unused"})
	used := e.customParameter(map[string]any{"name": "Used"})
	e.must(http.StatusOK, http.MethodPut, tpPath(tank, unused, ""), map[string]any{"enabled": true})
	e.addResults(tank, "2026-10-24T09:00:00Z", val(used, 1))

	e.fail(http.StatusConflict, "conflict", http.MethodDelete, paramPath(used, ""), nil)
	e.fail(http.StatusConflict, "conflict", http.MethodDelete, paramPath(e.preset("ph"), ""), nil)
	if n := e.count(`SELECT COUNT(*) FROM parameter_definitions`); n != 12 {
		t.Fatalf("%d definitions after refused deletes, want 12", n)
	}

	e.must(http.StatusNoContent, http.MethodDelete, paramPath(unused, ""), nil)
	if n := e.count(`SELECT COUNT(*) FROM parameter_definitions WHERE id = ?`, unused); n != 0 {
		t.Error("the unused parameter is still there")
	}
	if n := e.count(`SELECT COUNT(*) FROM tank_parameters WHERE parameter_id = ?`, unused); n != 0 {
		t.Error("the tank's settings for the deleted parameter are still there")
	}
	e.fail(http.StatusNotFound, "not_found", http.MethodDelete, paramPath(unused, ""), nil)

	// Once its results are gone the parameter can be deleted.
	var page ResultPage
	e.must(http.StatusOK, http.MethodGet, tankPath(tank, "/results"), nil).decode(&page)
	e.must(http.StatusNoContent, http.MethodDelete, "/api/v1/results/"+strconv.FormatInt(page.Results[0].ID, 10), nil)
	e.must(http.StatusNoContent, http.MethodDelete, paramPath(used, ""), nil)
}
