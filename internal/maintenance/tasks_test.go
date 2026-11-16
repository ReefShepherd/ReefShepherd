package maintenance

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/schedule"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

func TestCreateWholeSystemTaskWithDefaults(t *testing.T) {
	e := newEnv(t)
	task := e.newTask(map[string]any{"name": "  Replace RO/DI  ", "schedule": afterLast(180)})
	if task.ID == 0 || task.Name != "Replace RO/DI" || task.TankID != nil || task.TankName != nil || task.PresetID != nil {
		t.Errorf("created task = %+v", task)
	}
	if !task.Notify || task.Paused || task.Description != "" || task.LastDoneAt != nil || task.LastOutcome != nil {
		t.Errorf("defaults wrong: %+v", task)
	}
	if task.Schedule.Kind != "after_last" || task.Schedule.DueTime != "09:00" {
		t.Errorf("schedule = %+v", task.Schedule)
	}
	// Never done: due when the schedule started, which is now.
	if !isStr(task.NextDueAt, "2026-10-24T12:00:00Z") || task.Overdue {
		t.Errorf("next_due_at = %v overdue = %v", task.NextDueAt, task.Overdue)
	}
	if task.CreatedAt != "2026-10-24T12:00:00Z" || task.UpdatedAt != task.CreatedAt {
		t.Errorf("timestamps = %s %s", task.CreatedAt, task.UpdatedAt)
	}
	if got := e.getTask(task.ID); got.Name != task.Name || !isStr(got.NextDueAt, *task.NextDueAt) {
		t.Errorf("GET returned %+v", got)
	}
}

func TestCreateTankTaskFromPresetWithOptions(t *testing.T) {
	e := newEnv(t)
	tank := e.addTank("Display")
	preset := presetNamed(t, e.presets(""), "Clean glass")
	task := e.newTask(map[string]any{
		"tank_id": tank, "preset_id": preset.ID, "name": "Glass", "description": "algae scraper",
		"schedule": afterLast(3), "notify": false, "paused": true,
	})
	if task.TankID == nil || *task.TankID != tank || !isStr(task.TankName, "Display") || task.PresetID == nil || *task.PresetID != preset.ID {
		t.Errorf("task = %+v", task)
	}
	if task.Notify || !task.Paused || task.Description != "algae scraper" {
		t.Errorf("options not kept: %+v", task)
	}
}

func TestEmptyDueTimeTakesTheReminderTime(t *testing.T) {
	e := newEnv(t)
	e.setReminderTime("07:30")
	task := e.newTask(map[string]any{"name": "a", "schedule": weeklyOn(1)})
	if task.Schedule.DueTime != "07:30" {
		t.Errorf("due_time = %q, want the reminder time", task.Schedule.DueTime)
	}
	explicit := e.newTask(map[string]any{"name": "b", "schedule": map[string]any{"kind": "weekly", "weekdays": 1, "due_time": "21:15"}})
	if explicit.Schedule.DueTime != "21:15" {
		t.Errorf("explicit due_time = %q, want 21:15", explicit.Schedule.DueTime)
	}
	// The stored time of a task does not follow later changes of the setting.
	e.setReminderTime("10:00")
	if got := e.getTask(task.ID); got.Schedule.DueTime != "07:30" {
		t.Errorf("stored due_time = %q after the setting changed", got.Schedule.DueTime)
	}
}

func TestCreateTaskValidatesFields(t *testing.T) {
	e := newEnv(t)
	s := afterLast(3)
	e.invalid(http.MethodPost, tasksPath, map[string]any{"schedule": s}, "name=required")
	e.invalid(http.MethodPost, tasksPath, map[string]any{"name": strings.Repeat("é", 101), "schedule": s}, "name=too_long")
	e.invalid(http.MethodPost, tasksPath, map[string]any{"name": strings.Repeat("é", 100), "description": strings.Repeat("d", 2001), "schedule": s}, "description=too_long")
	e.invalid(http.MethodPost, tasksPath, map[string]any{"name": "a"}, "schedule=required")
	e.invalid(http.MethodPost, tasksPath, map[string]any{"name": "a", "schedule": map[string]any{}}, "schedule.kind=required")
	e.invalid(http.MethodPost, tasksPath, map[string]any{"name": "a", "schedule": map[string]any{"kind": "yearly"}}, "schedule=invalid")
	e.invalid(http.MethodPost, tasksPath, map[string]any{"name": "a", "schedule": map[string]any{"kind": "weekly"}}, "schedule=out_of_range")
	e.invalid(http.MethodPost, tasksPath, map[string]any{"name": "a", "tank_id": 999, "schedule": s}, "tank_id=invalid")
	e.invalid(http.MethodPost, tasksPath, map[string]any{"name": "a", "preset_id": 999, "schedule": s}, "preset_id=invalid")
	e.invalid(http.MethodPost, tasksPath, map[string]any{"name": "a", "schedule": s, "last_done_on": "yesterday"}, "last_done_on=invalid")
	e.invalid(http.MethodPost, tasksPath, map[string]any{"name": "a", "schedule": s, "last_done_on": "2026-02-30"}, "last_done_on=invalid")
	e.invalid(http.MethodPost, tasksPath, map[string]any{"name": "a", "schedule": s, "last_done_on": "2026-10-25"}, "last_done_on=out_of_range")
	e.invalid(http.MethodPost, tasksPath, map[string]any{"schedule": s, "tank_id": 999, "last_done_on": "x"}, "name=required", "tank_id=invalid", "last_done_on=invalid")
	if n := testutil.Count(t, e.db, `SELECT COUNT(*) FROM maintenance_tasks`); n != 0 {
		t.Errorf("%d tasks stored by refused requests", n)
	}
}

func TestLastDoneOnIsRecordedAsACompletionAtTheDueTimeInTheInstanceZone(t *testing.T) {
	e := newEnv(t)
	e.setTimezone("Europe/Berlin")
	task := e.newTask(map[string]any{"name": "a", "schedule": afterLast(7), "last_done_on": "2026-10-20"})
	// 09:00 in Berlin on 20 October is 07:00 UTC (summer time).
	if !isStr(task.LastDoneAt, "2026-10-20T07:00:00Z") || !isStr(task.LastOutcome, "done") {
		t.Errorf("last done = %v %v", task.LastDoneAt, task.LastOutcome)
	}
	// Summer time ends on 25 October, so 09:00 on the 27th is 08:00 UTC.
	if !isStr(task.NextDueAt, "2026-10-27T08:00:00Z") {
		t.Errorf("next_due_at = %s, want a week after the recorded completion", str(task.NextDueAt))
	}
	var page struct {
		Items []Completion `json:"items"`
	}
	testutil.DecodeInto(t, e.must(http.StatusOK, http.MethodGet, taskPath(task.ID, "/completions"), nil), &page)
	if len(page.Items) != 1 || page.Items[0].Outcome != "done" || page.Items[0].DoneAt != "2026-10-20T07:00:00Z" {
		t.Fatalf("completions = %+v", page.Items)
	}
	if page.Items[0].DoneByName == nil || *page.Items[0].DoneByName != "Tess" {
		t.Errorf("done_by_name = %v, want the creating user", page.Items[0].DoneByName)
	}
}

func TestLastDoneOnTodayIsNotDatedInTheFuture(t *testing.T) {
	e := newEnv(t)
	e.setReminderTime("23:00")
	task := e.newTask(map[string]any{"name": "a", "schedule": afterLast(7), "last_done_on": "2026-10-24"})
	if !isStr(task.LastDoneAt, "2026-10-24T12:00:00Z") {
		t.Errorf("last_done_at = %v, want now because 23:00 today has not happened yet", task.LastDoneAt)
	}
}

func TestTaskAndLastDoneOnAreStoredInOneTransaction(t *testing.T) {
	e := newEnv(t)
	// A user id with no row makes the completion insert fail after the task insert succeeded.
	_, err := e.svc.CreateTask(t.Context(), 9999, CreateTaskInput{
		Name: "a", Schedule: &schedule.Columns{Kind: "after_last", IntervalDays: ptr[int64](3), IntervalWeeks: 1}, LastDoneOn: ptr("2026-10-20"),
	})
	if err == nil {
		t.Fatal("create with a completion for an unknown user succeeded")
	}
	if n := testutil.Count(t, e.db, `SELECT COUNT(*) FROM maintenance_tasks`); n != 0 {
		t.Errorf("%d tasks left behind after the completion failed", n)
	}
}

func TestEditingTheScheduleMovesTheBaselineOnlyForRealChanges(t *testing.T) {
	e := newEnv(t)
	task := e.newTask(map[string]any{"name": "a", "schedule": afterLast(7)})
	created := e.scheduleSince(task.ID)
	if created != "2026-10-24T12:00:00Z" {
		t.Fatalf("schedule_since = %s", created)
	}

	e.now = e.now.Add(48 * time.Hour)
	patch := func(body map[string]any) Task {
		var got Task
		testutil.DecodeInto(t, e.must(http.StatusOK, http.MethodPatch, taskPath(task.ID, ""), body), &got)
		return got
	}
	patch(map[string]any{"name": "renamed", "notify": false, "paused": true, "description": "d"})
	if got := e.scheduleSince(task.ID); got != created {
		t.Errorf("a rename moved schedule_since to %s", got)
	}
	patch(map[string]any{"schedule": afterLast(7)})
	if got := e.scheduleSince(task.ID); got != created {
		t.Errorf("an identical schedule moved schedule_since to %s", got)
	}

	got := patch(map[string]any{"schedule": afterLast(10)})
	if since := e.scheduleSince(task.ID); since != "2026-10-26T12:00:00Z" {
		t.Errorf("schedule_since = %s after a real change, want the edit time", since)
	}
	// A never-done task is due when its schedule started, so editing moves the due date.
	if !isStr(got.NextDueAt, "2026-10-26T12:00:00Z") || got.Schedule.IntervalDays == nil || *got.Schedule.IntervalDays != 10 {
		t.Errorf("after the edit: next_due_at = %v schedule = %+v", got.NextDueAt, got.Schedule)
	}
	if got.UpdatedAt != "2026-10-26T12:00:00Z" {
		t.Errorf("updated_at = %s", got.UpdatedAt)
	}
}

func TestPatchTaskFields(t *testing.T) {
	e := newEnv(t)
	a, b := e.addTank("A"), e.addTank("B")
	task := e.newTask(map[string]any{"tank_id": a, "name": "a", "schedule": afterLast(7)})
	var got Task

	testutil.DecodeInto(t, e.must(http.StatusOK, http.MethodPatch, taskPath(task.ID, ""), map[string]any{"tank_id": b}), &got)
	if got.TankID == nil || *got.TankID != b || !isStr(got.TankName, "B") {
		t.Errorf("moved task = %+v", got)
	}
	testutil.DecodeInto(t, e.must(http.StatusOK, http.MethodPatch, taskPath(task.ID, ""), map[string]any{"tank_id": nil}), &got)
	if got.TankID != nil || got.TankName != nil {
		t.Errorf("task still has a tank after tank_id null: %+v", got)
	}
	testutil.DecodeInto(t, e.must(http.StatusOK, http.MethodPatch, taskPath(task.ID, ""), map[string]any{"paused": true, "notify": false}), &got)
	if !got.Paused || got.Notify || got.TankID != nil {
		t.Errorf("flags = %+v", got)
	}

	path := taskPath(task.ID, "")
	e.invalid(http.MethodPatch, path, map[string]any{"tank_id": 999}, "tank_id=invalid")
	e.invalid(http.MethodPatch, path, map[string]any{"name": nil}, "name=required")
	e.invalid(http.MethodPatch, path, map[string]any{"name": ""}, "name=required")
	e.invalid(http.MethodPatch, path, map[string]any{"name": strings.Repeat("x", 101)}, "name=too_long")
	e.invalid(http.MethodPatch, path, map[string]any{"description": nil}, "description=required")
	e.invalid(http.MethodPatch, path, map[string]any{"description": strings.Repeat("x", 2001)}, "description=too_long")
	e.invalid(http.MethodPatch, path, map[string]any{"schedule": nil}, "schedule=required")
	e.invalid(http.MethodPatch, path, map[string]any{"schedule": map[string]any{}}, "schedule.kind=required")
	e.invalid(http.MethodPatch, path, map[string]any{"schedule": map[string]any{"kind": "never"}}, "schedule=invalid")
	e.invalid(http.MethodPatch, path, map[string]any{"notify": nil, "paused": nil}, "notify=required", "paused=required")
	e.fail(http.StatusNotFound, "not_found", http.MethodPatch, taskPath(9999, ""), map[string]any{"name": "x"})
	e.fail(http.StatusNotFound, "not_found", http.MethodPatch, tasksPath+"/abc", map[string]any{"name": "x"})
	// A refused edit changes nothing.
	if got := e.getTask(task.ID); got.Name != "a" {
		t.Errorf("name = %q after refused edits", got.Name)
	}
}

func TestDeleteTaskRemovesItsCompletions(t *testing.T) {
	e := newEnv(t)
	task := e.newTask(map[string]any{"name": "a", "schedule": afterLast(7)})
	other := e.newTask(map[string]any{"name": "b", "schedule": afterLast(7)})
	e.complete(task.ID, nil)
	e.complete(other.ID, nil)
	e.must(http.StatusNoContent, http.MethodDelete, taskPath(task.ID, ""), nil)
	if n := testutil.Count(t, e.db, `SELECT COUNT(*) FROM maintenance_completions`); n != 1 {
		t.Errorf("%d completions left, want only the other task's", n)
	}
	e.fail(http.StatusNotFound, "not_found", http.MethodGet, taskPath(task.ID, ""), nil)
	e.fail(http.StatusNotFound, "not_found", http.MethodDelete, taskPath(task.ID, ""), nil)
	e.fail(http.StatusNotFound, "not_found", http.MethodDelete, tasksPath+"/abc", nil)
	e.fail(http.StatusNotFound, "not_found", http.MethodGet, tasksPath+"/abc", nil)
}

func TestTaskListFiltersByTankAndWholeSystem(t *testing.T) {
	e := newEnv(t)
	a, b, archived := e.addTank("A"), e.addTank("B"), e.addTank("Old")
	s := afterLast(7)
	e.newTask(map[string]any{"name": "a-task", "tank_id": a, "schedule": s})
	e.newTask(map[string]any{"name": "b-task", "tank_id": b, "schedule": s})
	e.newTask(map[string]any{"name": "old-task", "tank_id": archived, "schedule": s})
	e.newTask(map[string]any{"name": "system-task", "schedule": s})
	e.archiveTank(archived)

	names := func(query string) []string {
		got := taskNames(e.tasks(query))
		slices.Sort(got)
		return got
	}
	cases := []struct {
		query string
		want  []string
	}{
		{"", []string{"a-task", "b-task", "system-task"}},
		{"?tank_id=" + itoa(a), []string{"a-task"}},
		{"?tank_id=" + itoa(a) + "&include_tankless=true", []string{"a-task", "system-task"}},
		{"?tank_id=" + itoa(a) + "&include_tankless=false", []string{"a-task"}},
		{"?include_tankless=true", []string{"a-task", "b-task", "system-task"}},
		{"?tank_id=" + itoa(archived), []string{"old-task"}},
		{"?tank_id=" + itoa(archived) + "&include_tankless=true", []string{"old-task", "system-task"}},
	}
	for _, c := range cases {
		if got := names(c.query); !slices.Equal(got, c.want) {
			t.Errorf("GET tasks%s = %v, want %v", c.query, got, c.want)
		}
	}
	// A task of an archived tank can still be fetched directly.
	if got := e.tasks("?tank_id=" + itoa(archived)); len(got) != 1 {
		t.Fatalf("archived tank tasks = %v", taskNames(got))
	}

	e.fail(http.StatusNotFound, "not_found", http.MethodGet, tasksPath+"?tank_id=999", nil)
	e.invalid(http.MethodGet, tasksPath+"?tank_id=abc", nil, "tank_id=invalid")
	e.invalid(http.MethodGet, tasksPath+"?tank_id=0", nil, "tank_id=invalid")
	e.invalid(http.MethodGet, tasksPath+"?include_tankless=maybe", nil, "include_tankless=invalid")
	e.invalid(http.MethodGet, tasksPath+"?status=soon", nil, "status=invalid")
}

func TestGetTaskOfAnArchivedTank(t *testing.T) {
	e := newEnv(t)
	tank := e.addTank("Old")
	task := e.newTask(map[string]any{"name": "a", "tank_id": tank, "schedule": afterLast(7)})
	e.archiveTank(tank)
	if got := e.getTask(task.ID); got.ID != task.ID || !isStr(got.TankName, "Old") {
		t.Errorf("task of an archived tank = %+v", got)
	}
}

func TestStatusFiltersJudgeTheEndOfTodayInTheInstanceZone(t *testing.T) {
	e := newEnv(t)
	// 12:00 UTC is 05:00 on Saturday 24 October in Los Angeles, so today ends at 07:00 UTC on the 25th.
	e.setTimezone("America/Los_Angeles")
	insert := func(name, kind, since string, paused int) {
		var interval any
		if kind == "after_last" {
			interval = 1
		}
		testutil.Exec(t, e.db, `INSERT INTO maintenance_tasks (name, schedule_kind, interval_days, schedule_since, paused) VALUES (?, ?, ?, ?, ?)`,
			name, kind, interval, since, paused)
	}
	insert("overdue", "after_last", "2026-10-22T00:00:00Z", 0)
	insert("late tonight", "after_last", "2026-10-25T06:59:00Z", 0)
	insert("after midnight", "after_last", "2026-10-25T07:00:00Z", 0)
	insert("next week", "after_last", "2026-11-01T00:00:00Z", 0)
	insert("paused", "after_last", "2026-10-20T00:00:00Z", 1)
	insert("unscheduled", "none", "2026-10-20T00:00:00Z", 0)

	cases := []struct {
		status string
		want   []string
	}{
		{"", []string{"overdue", "late tonight"}},
		{"?status=due", []string{"overdue", "late tonight"}},
		{"?status=upcoming", []string{"after midnight", "next week"}},
		{"?status=all", []string{"overdue", "late tonight", "after midnight", "next week", "unscheduled", "paused"}},
	}
	for _, c := range cases {
		if got := taskNames(e.tasks(c.status)); !slices.Equal(got, c.want) {
			t.Errorf("status %q = %v, want %v", c.status, got, c.want)
		}
	}
	all := e.tasks("?status=all")
	if !all[0].Overdue || all[1].Overdue {
		t.Errorf("overdue flags wrong: %v %v", all[0].Overdue, all[1].Overdue)
	}
	if all[4].NextDueAt != nil || all[4].Overdue {
		t.Errorf("unscheduled task has a due date: %+v", all[4])
	}
}

func TestTaskListOrdersByDueDateThenName(t *testing.T) {
	e := newEnv(t)
	for _, row := range []struct{ name, since string }{
		{"b same", "2026-10-23T00:00:00Z"}, {"A same", "2026-10-23T00:00:00Z"}, {"first", "2026-10-22T00:00:00Z"},
	} {
		testutil.Exec(t, e.db, `INSERT INTO maintenance_tasks (name, schedule_kind, interval_days, schedule_since) VALUES (?, 'after_last', 1, ?)`, row.name, row.since)
	}
	if got, want := taskNames(e.tasks("")), []string{"first", "A same", "b same"}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestListEndpointsAnswerWithAnItemsEnvelope(t *testing.T) {
	e := newEnv(t)
	task := e.newTask(map[string]any{"name": "a", "schedule": afterLast(7)})
	for _, path := range []string{
		presetsPath, tasksPath + "?status=upcoming", taskPath(task.ID, "/completions"),
	} {
		var raw map[string]any
		testutil.DecodeInto(t, e.must(http.StatusOK, http.MethodGet, path, nil), &raw)
		if items, ok := raw["items"].([]any); !ok || items == nil {
			t.Errorf("GET %s: items = %#v, want an array", path, raw["items"])
		}
	}
	var raw map[string]any
	testutil.DecodeInto(t, e.must(http.StatusOK, http.MethodGet, taskPath(task.ID, "/completions"), nil), &raw)
	if v, ok := raw["next_cursor"]; !ok || v != nil {
		t.Errorf("next_cursor = %#v, want an explicit null", v)
	}
}

func TestEveryEndpointIsOpenToMembers(t *testing.T) {
	e := newEnv(t)
	member := testutil.Insert(t, e.db, `INSERT INTO users (username, role) VALUES ('member', 'member')`)
	as := testutil.AsUser(member, httpx.RoleMember)
	send := func(status int, method, path string, body any) {
		t.Helper()
		testutil.ExpectStatus(t, testutil.DoJSON(t, e.mux, method, path, body, as), status)
	}
	send(http.StatusOK, http.MethodGet, "/api/v1/maintenance/defaults", nil)
	send(http.StatusOK, http.MethodGet, presetsPath, nil)
	send(http.StatusCreated, http.MethodPost, presetsPath, map[string]any{"name": "p", "schedule": afterLast(1)})
	send(http.StatusCreated, http.MethodPost, tasksPath, map[string]any{"name": "t", "schedule": afterLast(1)})
	task := e.tasks("?status=all")[0]
	send(http.StatusOK, http.MethodGet, tasksPath, nil)
	send(http.StatusOK, http.MethodGet, taskPath(task.ID, ""), nil)
	send(http.StatusOK, http.MethodPatch, taskPath(task.ID, ""), map[string]any{"name": "u"})
	send(http.StatusCreated, http.MethodPost, taskPath(task.ID, "/complete"), map[string]any{})
	send(http.StatusOK, http.MethodGet, taskPath(task.ID, "/completions"), nil)
	var page struct {
		Items []Completion `json:"items"`
	}
	testutil.DecodeInto(t, e.must(http.StatusOK, http.MethodGet, taskPath(task.ID, "/completions"), nil), &page)
	if page.Items[0].DoneByName == nil || *page.Items[0].DoneByName != "member" {
		t.Errorf("done_by_name = %v, want the username when there is no display name", page.Items[0].DoneByName)
	}
	send(http.StatusOK, http.MethodPatch, completionPath(page.Items[0].ID), map[string]any{"note": "n"})
	send(http.StatusNoContent, http.MethodDelete, completionPath(page.Items[0].ID), nil)
	send(http.StatusNoContent, http.MethodDelete, taskPath(task.ID, ""), nil)
}

func TestAStoredScheduleThatCannotBeReadIsAServerFault(t *testing.T) {
	e := newEnv(t)
	testutil.Exec(t, e.db, `INSERT INTO maintenance_tasks (id, name, schedule_kind) VALUES (1, 'broken', 'bogus')`)
	e.fail(http.StatusInternalServerError, "internal", http.MethodGet, taskPath(1, ""), nil)
	testutil.Exec(t, e.db, `UPDATE maintenance_tasks SET schedule_kind = 'none', schedule_since = 'yesterday'`)
	e.fail(http.StatusInternalServerError, "internal", http.MethodGet, taskPath(1, ""), nil)
	testutil.Exec(t, e.db, `UPDATE maintenance_tasks SET schedule_since = '2026-10-20T00:00:00Z'; INSERT INTO maintenance_completions (task_id, done_at) VALUES (1, 'never')`)
	e.fail(http.StatusInternalServerError, "internal", http.MethodGet, taskPath(1, ""), nil)
	testutil.Exec(t, e.db, `UPDATE maintenance_tasks SET schedule_kind = 'bogus'`)
	testutil.Exec(t, e.db, `INSERT INTO maintenance_presets (name, schedule_kind) VALUES ('broken', 'bogus')`)
	e.fail(http.StatusInternalServerError, "internal", http.MethodGet, presetsPath, nil)
	if _, err := e.svc.DueForReminders(t.Context(), e.now); err == nil {
		t.Error("DueForReminders accepted a stored schedule that cannot be read")
	}
}
