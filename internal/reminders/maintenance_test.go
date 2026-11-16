package reminders

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/notify"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

func TestMaintenanceDueIsRaisedOncePerTaskPerDueInstantAcrossManyTicks(t *testing.T) {
	e := newEnv(t)
	e.setTimezone("Europe/Berlin")
	tank := e.addTank("Display")
	tankTask := e.addTask("Water change", tank, "2026-10-24T07:00:00Z")
	systemTask := e.addTask("RO/DI cartridges", nil, "2026-10-24T08:30:00Z")

	e.tickMaintenance("2026-10-24T06:59:00Z")
	if got := e.notes(); len(got) != 0 {
		t.Fatalf("a minute before anything is due: %+v", got)
	}
	// A tick every minute for five hours: the task due at 07:00 and the one due at 08:30 are each raised once.
	for now := at("2026-10-24T07:00:00Z"); !now.After(at("2026-10-24T12:00:00Z")); now = now.Add(time.Minute) {
		e.tickMaintenance(now.Format(time.RFC3339))
	}

	got := e.notes()
	if len(got) != 2 {
		t.Fatalf("notifications = %+v, want 2", got)
	}
	want := note{
		EventType: "maintenance_due", Severity: "info", Title: "Water change is due on Display", Body: "Due Sat 24 Oct 2026, 09:00 CEST.",
		Link: "/tanks/1/maintenance", SourceKind: "maintenance_task", SourceID: tankTask, Occurrence: "2026-10-24T07:00:00Z",
	}
	if g := got[0]; g.TankID == nil || *g.TankID != tank {
		t.Errorf("tank task notification tank = %v, want %d", g.TankID, tank)
	} else if withoutTank(g) != want {
		t.Errorf("tank task notification = %+v, want %+v", g, want)
	}
	want = note{
		EventType: "maintenance_due", Severity: "info", Title: "RO/DI cartridges is due", Body: "Due Sat 24 Oct 2026, 10:30 CEST.",
		Link: "/maintenance", SourceKind: "maintenance_task", SourceID: systemTask, Occurrence: "2026-10-24T08:30:00Z",
	}
	if g := got[1]; g.TankID != nil || g != want {
		t.Errorf("whole-system notification = %+v, want %+v", g, want)
	}
}

func TestMaintenanceDueRaisesAgainWhenTheTaskIsDoneAndFallsDueAgain(t *testing.T) {
	e := newEnv(t)
	e.setTimezone("Europe/Berlin")
	task := e.addTask("Water change", e.addTank("Display"), "2026-10-24T07:00:00Z")

	e.tickMaintenance("2026-10-24T07:00:00Z")
	e.complete(task, "2026-10-24T10:00:00Z")
	// Done on the 24th, so the next one is due on the 25th at 09:00 local time, which is 08:00 UTC after the clocks go back.
	e.tickMaintenance("2026-10-25T07:59:00Z")
	if n := len(e.notes()); n != 1 {
		t.Fatalf("notifications before the next due time = %d, want 1", n)
	}
	e.tickMaintenance("2026-10-25T08:00:00Z")
	e.tickMaintenance("2026-10-25T20:00:00Z")

	got := e.notes()
	if len(got) != 2 || got[1].Occurrence != "2026-10-25T08:00:00Z" || got[1].Body != "Due Sun 25 Oct 2026, 09:00 CET." {
		t.Errorf("notifications = %+v, want a second one for the 25th", got)
	}
}

func TestMaintenanceDueAfterDowntimeRaisesOneReminderNotOnePerMissedOccurrence(t *testing.T) {
	e := newEnv(t)
	tank := e.addTank("Display")
	// Mondays at 09:00 counted from 1 October: the first is the 5th, and the 12th, 19th and 26th pass while the server is down.
	weekly := e.addWeeklyTask("Skimmer cup", tank, "2026-10-01T00:00:00Z")
	// A daily task that has been due for three days.
	daily := e.addTask("Check salinity", tank, "2026-10-24T09:00:00Z")

	e.tickMaintenance("2026-10-05T08:00:00Z")
	e.tickMaintenance("2026-10-27T12:00:00Z")
	got := e.notes()
	if len(got) != 2 {
		t.Fatalf("notifications after the downtime = %+v, want one per task", got)
	}
	if got[0].SourceID != weekly || got[0].Occurrence != "2026-10-05T09:00:00Z" {
		t.Errorf("weekly notification = %+v", got[0])
	}
	if got[1].SourceID != daily || got[1].Occurrence != "2026-10-24T09:00:00Z" {
		t.Errorf("daily notification = %+v", got[1])
	}

	// Later ticks stay quiet until a task is done and falls due again.
	for _, now := range []string{"2026-10-27T12:01:00Z", "2026-10-28T09:00:00Z", "2026-10-30T09:00:00Z"} {
		e.tickMaintenance(now)
	}
	if n := len(e.notes()); n != 2 {
		t.Fatalf("notifications after more ticks = %d, want 2", n)
	}
	e.complete(weekly, "2026-10-27T12:05:00Z")
	e.tickMaintenance("2026-11-02T09:00:00Z")
	got = e.notes()
	if len(got) != 3 || got[2].SourceID != weekly || got[2].Occurrence != "2026-11-02T09:00:00Z" {
		t.Errorf("notifications after completing the weekly task = %+v", got)
	}
}

func TestMaintenanceDueSkipsPausedSilentFutureAndArchivedTankTasks(t *testing.T) {
	e := newEnv(t)
	tank, old := e.addTank("Display"), e.addTank("Old")
	listed := e.addTask("listed", tank, "2026-10-24T09:00:00Z")
	e.addTask("not yet", tank, "2026-10-25T12:00:00Z")
	paused := e.addTask("paused", tank, "2026-10-24T09:00:00Z")
	silent := e.addTask("silent", tank, "2026-10-24T09:00:00Z")
	e.addTask("old tank", old, "2026-10-24T09:00:00Z")
	e.archiveTank(old)
	testutil.Exec(t, e.db, `UPDATE maintenance_tasks SET paused = 1 WHERE id = ?`, paused)
	testutil.Exec(t, e.db, `UPDATE maintenance_tasks SET notify = 0 WHERE id = ?`, silent)

	e.tickMaintenance("2026-10-24T12:00:00Z")
	got := e.notes()
	if len(got) != 1 || got[0].SourceID != listed {
		t.Fatalf("notifications = %+v, want only task %d", got, listed)
	}

	// Switching reminders back on raises the pending occurrence.
	testutil.Exec(t, e.db, `UPDATE maintenance_tasks SET paused = 0 WHERE id = ?`, paused)
	e.tickMaintenance("2026-10-24T12:01:00Z")
	if n := len(e.notes()); n != 2 {
		t.Errorf("notifications after resuming = %d, want 2", n)
	}
}

func TestMaintenanceDueRestartRaisesNothingExtra(t *testing.T) {
	e := newEnv(t)
	e.addTask("Water change", e.addTank("Display"), "2026-10-24T09:00:00Z")
	e.tickMaintenance("2026-10-24T09:00:00Z")

	restarted := NewService(e.deps)
	for _, now := range []string{"2026-10-24T09:01:00Z", "2026-10-24T09:02:00Z", "2026-10-25T09:00:00Z"} {
		if err := restarted.RaiseMaintenanceDue(t.Context(), at(now)); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(e.notes()); n != 1 {
		t.Errorf("notifications after a restart = %d, want 1", n)
	}
}

func TestMaintenanceDueRaiseFailureIsReturnedAndDoesNotStopOtherTasks(t *testing.T) {
	e := newEnv(t)
	tank := e.addTank("Display")
	first := e.addTask("first", tank, "2026-10-24T08:00:00Z")
	second := e.addTask("second", tank, "2026-10-24T09:00:00Z")
	boom := errors.New("boom")
	e.raiser.fail = func(ev notify.Event) error {
		if ev.SourceID == first {
			return boom
		}
		return nil
	}

	e.now = at("2026-10-24T10:00:00Z")
	err := e.svc.RaiseMaintenanceDue(t.Context(), e.now)
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "task 1") {
		t.Errorf("error = %v, want the raise failure for task 1", err)
	}
	if got := e.notes(); len(got) != 1 || got[0].SourceID != second {
		t.Errorf("notifications = %+v, want only task %d", got, second)
	}

	// The failed task is retried on the next tick.
	e.raiser.fail = nil
	e.tickMaintenance("2026-10-24T10:01:00Z")
	if n := len(e.notes()); n != 2 {
		t.Errorf("notifications after the retry = %d, want 2", n)
	}
}
