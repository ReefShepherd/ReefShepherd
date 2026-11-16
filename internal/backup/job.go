package backup

import (
	"context"
	"fmt"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/i18n"
	"github.com/reefshepherd/reefshepherd/internal/notify"
	"github.com/reefshepherd/reefshepherd/internal/scheduler"
)

const (
	jobTimeout = 10 * time.Minute
	// slotLayout is how a backup slot reads in a notification.
	slotLayout  = "2006-01-02 15:04"
	clockLayout = "15:04"
)

// Jobs returns the background jobs of the package: the scheduled backup at the time set in the settings.
func Jobs(deps Deps) []scheduler.Job {
	s := NewService(deps)
	return []scheduler.Job{{
		Name:    "backup",
		Trigger: scheduler.DailyAt(deps.Settings.GetBackupTime, s.location),
		Timeout: jobTimeout,
		Run:     s.runScheduled,
	}}
}

// location is the instance timezone.
func (s *Service) location(ctx context.Context) (*time.Location, error) {
	name, err := s.Settings.GetTimezone(ctx)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("load timezone %q: %w", name, err)
	}
	return loc, nil
}

// slot is the instant of today's scheduled backup, in the instance timezone's day that contains now.
func (s *Service) slot(ctx context.Context, now time.Time) (time.Time, *time.Location, error) {
	loc, err := s.location(ctx)
	if err != nil {
		return time.Time{}, nil, err
	}
	clock, err := s.Settings.GetBackupTime(ctx)
	if err != nil {
		return time.Time{}, nil, err
	}
	t, err := time.Parse(clockLayout, clock)
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("backup time %q: %w", clock, err)
	}
	local := now.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), t.Hour(), t.Minute(), 0, 0, loc).UTC(), loc, nil
}

// runScheduled makes today's backup unless backups are off or one was already made since today's slot, so a restart never doubles it.
func (s *Service) runScheduled(ctx context.Context, now time.Time) error {
	enabled, err := s.Settings.GetBackupEnabled(ctx)
	if err != nil || !enabled {
		return err
	}
	slot, loc, err := s.slot(ctx, now)
	if err != nil {
		return err
	}
	if err := s.backUpUnlessDone(ctx, now, slot); err != nil {
		s.raiseFailure(ctx, slot, loc)
		return err
	}
	return nil
}

// backUpUnlessDone makes a scheduled backup unless one at or after slot exists.
func (s *Service) backUpUnlessDone(ctx context.Context, now, slot time.Time) error {
	existing, err := s.List()
	if err != nil {
		return err
	}
	for _, b := range existing {
		if b.Kind == KindScheduled && !b.at.Before(slot) {
			return nil
		}
	}
	_, err = s.create(ctx, now)
	return err
}

// raiseFailure tells the admins that the backup of a slot failed; the notification is raised once per slot.
func (s *Service) raiseFailure(ctx context.Context, slot time.Time, loc *time.Location) {
	t := i18n.Default()
	_, err := s.Notify.Raise(ctx, notify.Event{
		Type:       "backup_failed",
		Severity:   "critical",
		Title:      t.T(i18n.DefaultLocale, "backup_failed_title"),
		Body:       t.T(i18n.DefaultLocale, "backup_failed_body", slot.In(loc).Format(slotLayout)),
		Link:       "/settings/backups",
		SourceKind: "backup",
		Occurrence: db.FormatTime(slot),
	})
	if err != nil {
		s.Logger.Error("raise backup failure notification failed", "error", err)
	}
}
