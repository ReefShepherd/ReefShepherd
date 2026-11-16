package app

import (
	"context"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/backup"
	"github.com/reefshepherd/reefshepherd/internal/notify"
	"github.com/reefshepherd/reefshepherd/internal/reminders"
	"github.com/reefshepherd/reefshepherd/internal/scheduler"
)

// buildJobs assembles every background job of the application, in the order they are listed.
func buildJobs(d routeDeps) []scheduler.Job {
	jobs := []scheduler.Job{{
		Name:    "sessions-cleanup",
		Trigger: scheduler.Every(time.Hour),
		Run: func(ctx context.Context, _ time.Time) error {
			return d.Auth.CleanupExpired(ctx)
		},
	}}
	jobs = append(jobs, notify.Jobs(d.Notify)...)
	jobs = append(jobs, reminders.Jobs(d.RemindersDeps)...)
	jobs = append(jobs, backup.Jobs(d.Backup)...)
	return jobs
}
