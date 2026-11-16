package reminders

import "github.com/reefshepherd/reefshepherd/internal/scheduler"

// Jobs returns the background jobs of the package: the reminders for due maintenance and for due tests.
func Jobs(deps Deps) []scheduler.Job {
	svc := NewService(deps)
	return []scheduler.Job{
		{Name: "reminders-maintenance", Trigger: scheduler.EveryTick(), Run: svc.RaiseMaintenanceDue},
		{Name: "reminders-tests", Trigger: scheduler.EveryTick(), Run: svc.RaiseTestsDue},
	}
}
