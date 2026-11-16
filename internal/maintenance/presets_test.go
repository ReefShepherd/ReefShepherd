package maintenance

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

const presetsPath = "/api/v1/maintenance/presets"

func (e *env) presets(query string) []Preset {
	e.t.Helper()
	return testutil.DecodeItems[Preset](e.t, e.must(http.StatusOK, http.MethodGet, presetsPath+query, nil))
}

func presetNamed(t *testing.T, list []Preset, name string) Preset {
	t.Helper()
	for _, p := range list {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("no preset %q in %+v", name, list)
	return Preset{}
}

func TestDefaultsReturnTheReminderTime(t *testing.T) {
	e := newEnv(t)
	var d Defaults
	testutil.DecodeInto(t, e.must(http.StatusOK, http.MethodGet, "/api/v1/maintenance/defaults", nil), &d)
	if d.DueTime != "09:00" {
		t.Errorf("due_time = %q, want 09:00", d.DueTime)
	}
	e.setReminderTime("07:30")
	testutil.DecodeInto(t, e.must(http.StatusOK, http.MethodGet, "/api/v1/maintenance/defaults", nil), &d)
	if d.DueTime != "07:30" {
		t.Errorf("due_time = %q, want 07:30 after the setting changed", d.DueTime)
	}
}

func TestShippedPresetsAreListedInOrderWithTheReminderTime(t *testing.T) {
	e := newEnv(t)
	e.setReminderTime("07:30")
	list := e.presets("")
	if len(list) != 10 {
		t.Fatalf("%d presets, want 10", len(list))
	}
	if list[0].Name != "Water change" || list[0].PresetKey == nil || *list[0].PresetKey != "water_change" {
		t.Errorf("first preset = %+v", list[0])
	}
	if last := list[len(list)-1]; last.Name != "Replace RO/DI cartridges" || !last.Tankless {
		t.Errorf("last preset = %+v, want the tankless RO/DI cartridges", last)
	}
	for i, p := range list {
		if p.Schedule.Kind != "after_last" || p.Schedule.DueTime != "07:30" || p.Schedule.IntervalDays == nil {
			t.Errorf("preset %d schedule = %+v, want after_last at the reminder time", i, p.Schedule)
		}
		if i > 0 && p.SortOrder <= list[i-1].SortOrder {
			t.Errorf("preset %d sort_order %d not after %d", i, p.SortOrder, list[i-1].SortOrder)
		}
	}
	if wc := presetNamed(t, list, "Water change"); *wc.Schedule.IntervalDays != 7 {
		t.Errorf("water change interval = %d, want 7", *wc.Schedule.IntervalDays)
	}
}

func TestPresetsCanBeHiddenAndShownAgain(t *testing.T) {
	e := newEnv(t)
	wc := presetNamed(t, e.presets(""), "Water change")

	var got Preset
	testutil.DecodeInto(t, e.must(http.StatusOK, http.MethodPatch, presetPath(wc.ID), map[string]any{"archived": true}), &got)
	if got.ArchivedAt == nil {
		t.Fatal("archived_at not set after hiding")
	}
	if n := len(e.presets("")); n != 9 {
		t.Errorf("%d presets listed after hiding one, want 9", n)
	}
	all := e.presets("?include_archived=true")
	if len(all) != 10 || presetNamed(t, all, "Water change").ArchivedAt == nil {
		t.Errorf("include_archived list = %d presets, hidden one %+v", len(all), presetNamed(t, all, "Water change"))
	}

	firstStamp := *got.ArchivedAt
	e.now = e.now.Add(time.Hour)
	testutil.DecodeInto(t, e.must(http.StatusOK, http.MethodPatch, presetPath(wc.ID), map[string]any{"archived": true}), &got)
	if *got.ArchivedAt != firstStamp {
		t.Errorf("hiding twice moved archived_at from %s to %s", firstStamp, *got.ArchivedAt)
	}

	testutil.DecodeInto(t, e.must(http.StatusOK, http.MethodPatch, presetPath(wc.ID), map[string]any{"archived": false}), &got)
	if got.ArchivedAt != nil || len(e.presets("")) != 10 {
		t.Errorf("preset still hidden after showing it: %+v", got)
	}
	e.invalid(http.MethodPatch, presetPath(wc.ID), map[string]any{"archived": nil}, "archived=invalid")
}

func TestPresetsCanBeEdited(t *testing.T) {
	e := newEnv(t)
	wc := presetNamed(t, e.presets(""), "Water change")
	var got Preset
	testutil.DecodeInto(t, e.must(http.StatusOK, http.MethodPatch, presetPath(wc.ID), map[string]any{
		"name": "  Big water change ", "description": "20 percent", "tankless": true, "schedule": weeklyOn(64),
	}), &got)
	if got.Name != "Big water change" || got.Description != "20 percent" || !got.Tankless {
		t.Errorf("edited preset = %+v", got)
	}
	if got.Schedule.Kind != "weekly" || got.Schedule.Weekdays == nil || *got.Schedule.Weekdays != 64 || got.Schedule.DueTime != "09:00" {
		t.Errorf("edited schedule = %+v", got.Schedule)
	}
	if got.PresetKey == nil || *got.PresetKey != "water_change" || got.SortOrder != wc.SortOrder {
		t.Errorf("edit changed identity: %+v", got)
	}

	e.invalid(http.MethodPatch, presetPath(wc.ID), map[string]any{"name": nil}, "name=required")
	e.invalid(http.MethodPatch, presetPath(wc.ID), map[string]any{"name": "  "}, "name=required")
	e.invalid(http.MethodPatch, presetPath(wc.ID), map[string]any{"schedule": nil}, "schedule=required")
	e.invalid(http.MethodPatch, presetPath(wc.ID), map[string]any{"description": strings.Repeat("x", 2001)}, "description=too_long")
	e.invalid(http.MethodPatch, presetPath(wc.ID), map[string]any{"schedule": map[string]any{"kind": "sometimes"}}, "schedule=invalid")
	e.fail(http.StatusNotFound, "not_found", http.MethodPatch, presetPath(9999), map[string]any{"name": "x"})
	e.fail(http.StatusNotFound, "not_found", http.MethodPatch, presetsPath+"/abc", map[string]any{"name": "x"})
}

func TestPresetsCanBeCreatedWithTheReminderTime(t *testing.T) {
	e := newEnv(t)
	e.setReminderTime("06:45")
	before := e.presets("")
	var got Preset
	testutil.DecodeInto(t, e.must(http.StatusCreated, http.MethodPost, presetsPath, map[string]any{
		"name": " Clean sump ", "description": "monthly", "tankless": false,
		"schedule": map[string]any{"kind": "monthly", "month_day": 15, "due_time": "23:00"},
	}), &got)
	if got.Name != "Clean sump" || got.PresetKey != nil || got.ArchivedAt != nil || got.Description != "monthly" {
		t.Errorf("created preset = %+v", got)
	}
	if got.Schedule.DueTime != "06:45" {
		t.Errorf("preset due_time = %q, want the reminder time 06:45 because presets store none", got.Schedule.DueTime)
	}
	if got.SortOrder <= before[len(before)-1].SortOrder {
		t.Errorf("sort_order %d not after the last shipped one %d", got.SortOrder, before[len(before)-1].SortOrder)
	}
	list := e.presets("")
	if len(list) != 11 || list[10].ID != got.ID {
		t.Errorf("new preset not listed last: %d presets", len(list))
	}
}

func TestPresetCreateValidatesFields(t *testing.T) {
	e := newEnv(t)
	ok := map[string]any{"kind": "after_last", "interval_days": 3}
	e.invalid(http.MethodPost, presetsPath, map[string]any{"schedule": ok}, "name=required")
	e.invalid(http.MethodPost, presetsPath, map[string]any{"name": strings.Repeat("n", 101), "schedule": ok}, "name=too_long")
	e.invalid(http.MethodPost, presetsPath, map[string]any{"name": "a", "description": strings.Repeat("d", 2001), "schedule": ok}, "description=too_long")
	e.invalid(http.MethodPost, presetsPath, map[string]any{"name": "a"}, "schedule=required")
	e.invalid(http.MethodPost, presetsPath, map[string]any{"name": "a", "schedule": map[string]any{}}, "schedule.kind=required")
	e.invalid(http.MethodPost, presetsPath, map[string]any{"name": "a", "schedule": map[string]any{"kind": "bogus"}}, "schedule=invalid")
	e.invalid(http.MethodPost, presetsPath, map[string]any{"name": "a", "schedule": map[string]any{"kind": "after_last"}}, "schedule=out_of_range")
	if n := len(e.presets("?include_archived=true")); n != 10 {
		t.Errorf("%d presets after refused creates, want 10", n)
	}
}

func TestPresetListOrdersBySortOrderThenName(t *testing.T) {
	e := newEnv(t)
	testutil.Exec(t, e.db, `UPDATE maintenance_presets SET sort_order = 5`)
	var names []string
	for _, p := range e.presets("") {
		names = append(names, p.Name)
	}
	if !slices.IsSorted(names) {
		t.Errorf("equal sort_order presets not ordered by name: %v", names)
	}
}
