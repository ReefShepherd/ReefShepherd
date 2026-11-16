package maintenance

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

func dueIDs(due []DueTask) []int64 {
	ids := make([]int64, len(due))
	for i, d := range due {
		ids[i] = d.ID
	}
	return ids
}

func TestDueForRemindersListsOnlyTasksThatShouldRemind(t *testing.T) {
	e := newEnv(t)
	tank, old := e.addTank("Display"), e.addTank("Old")
	insert := func(name string, tankID any, since string, notify, paused int) int64 {
		return testutil.Insert(t, e.db, `INSERT INTO maintenance_tasks (name, tank_id, schedule_kind, interval_days, schedule_since, notify, paused)
			VALUES (?, ?, 'after_last', 1, ?, ?, ?)`, name, tankID, since, notify, paused)
	}
	tankTask := insert("tank task", tank, "2026-10-22T00:00:00Z", 1, 0)
	systemTask := insert("system task", nil, "2026-10-21T00:00:00Z", 1, 0)
	exactlyNow := insert("exactly now", nil, "2026-10-24T12:00:00Z", 1, 0)
	insert("not yet", tank, "2026-10-24T12:00:01Z", 1, 0)
	insert("paused", tank, "2026-10-20T00:00:00Z", 1, 1)
	insert("silent", tank, "2026-10-20T00:00:00Z", 0, 0)
	oldTask := insert("old tank", old, "2026-10-20T00:00:00Z", 1, 0)
	e.archiveTank(old)

	due, err := e.svc.DueForReminders(t.Context(), e.now)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := dueIDs(due), []int64{systemTask, tankTask, exactlyNow}; !slices.Equal(got, want) {
		t.Fatalf("due tasks = %v, want %v (paused, silent, future and archived-tank tasks left out)", got, want)
	}
	if d := due[0]; d.TankID != nil || d.TankName != "" || d.Name != "system task" || !d.DueAt.Equal(at("2026-10-21T00:00:00Z")) {
		t.Errorf("whole-system task = %+v", d)
	}
	if d := due[1]; d.TankID == nil || *d.TankID != tank || d.TankName != "Display" || !d.DueAt.Equal(at("2026-10-22T00:00:00Z")) {
		t.Errorf("tank task = %+v", d)
	}

	// The comparison uses the instant it is given.
	early, err := e.svc.DueForReminders(t.Context(), at("2026-10-21T12:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if got := dueIDs(early); !slices.Equal(got, []int64{systemTask}) {
		t.Errorf("due on the 21st = %v", got)
	}

	testutil.Exec(t, e.db, `UPDATE tanks SET archived_at = NULL WHERE id = ?`, old)
	due, err = e.svc.DueForReminders(t.Context(), e.now)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(dueIDs(due), oldTask) {
		t.Errorf("task of the unarchived tank missing from %v", dueIDs(due))
	}
}

func TestDueForRemindersFollowsPauseAndNotifyEdits(t *testing.T) {
	e := newEnv(t)
	task := e.newTask(map[string]any{"name": "a", "schedule": afterLast(1)})
	count := func() int {
		due, err := e.svc.DueForReminders(t.Context(), e.now.Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		return len(due)
	}
	if count() != 1 {
		t.Fatal("a new never-done task is not due for a reminder")
	}
	e.must(http.StatusOK, http.MethodPatch, taskPath(task.ID, ""), map[string]any{"paused": true})
	if count() != 0 {
		t.Error("a paused task is due for a reminder")
	}
	e.must(http.StatusOK, http.MethodPatch, taskPath(task.ID, ""), map[string]any{"paused": false, "notify": false})
	if count() != 0 {
		t.Error("a task with notify off is due for a reminder")
	}
	e.must(http.StatusOK, http.MethodPatch, taskPath(task.ID, ""), map[string]any{"notify": true})
	e.complete(task.ID, nil)
	if count() != 0 {
		t.Error("a task that was just completed is due for a reminder")
	}
}
