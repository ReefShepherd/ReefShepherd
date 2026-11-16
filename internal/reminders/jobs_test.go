package reminders

import (
	"slices"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/scheduler"
)

func TestJobsRunEveryTickThroughTheScheduler(t *testing.T) {
	e := newEnv(t)
	tank := e.addTank("Display")
	e.addTask("Water change", tank, "2026-10-24T09:00:00Z")
	e.enableTest(tank, "alkalinity", "2026-10-20T00:00:00Z", 1)

	jobs := Jobs(e.deps)
	var names []string
	for _, j := range jobs {
		names = append(names, j.Name)
		if due, err := j.Trigger.Due(t.Context(), e.now, e.now); err != nil || !due {
			t.Errorf("job %s due on every tick = %v, %v", j.Name, due, err)
		}
	}
	if want := []string{"reminders-maintenance", "reminders-tests"}; !slices.Equal(names, want) {
		t.Fatalf("jobs = %v, want %v", names, want)
	}

	now := at("2026-10-24T10:00:00Z")
	e.now = now
	sched := scheduler.New(e.deps.Logger, jobs, scheduler.WithClock(func() time.Time { return now }))
	sched.Tick(t.Context(), now)
	sched.Tick(t.Context(), now.Add(time.Minute))
	sched.Tick(t.Context(), now.Add(2*time.Minute))

	// The jobs run side by side, so their order is not fixed.
	got := e.titles()
	slices.Sort(got)
	if want := []string{"1 test due on Display", "Water change is due on Display"}; !slices.Equal(got, want) {
		t.Errorf("notifications = %v, want %v", got, want)
	}
}
