package reminders

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/maintenance"
	"github.com/reefshepherd/reefshepherd/internal/notify"
)

// RaiseMaintenanceDue raises one notification for each due task at its current due instant, which stays the same until the task is done or skipped, so reruns and downtime never repeat or flood.
func (s *Service) RaiseMaintenanceDue(ctx context.Context, now time.Time) error {
	tasks, err := s.Maintenance.DueForReminders(ctx, now)
	if err != nil {
		return fmt.Errorf("list due tasks: %w", err)
	}
	if len(tasks) == 0 {
		return nil
	}
	loc, err := s.location(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, t := range tasks {
		if _, err := s.Notify.Raise(ctx, maintenanceEvent(t, loc)); err != nil {
			errs = append(errs, fmt.Errorf("raise reminder for task %d: %w", t.ID, err))
		}
	}
	return errors.Join(errs...)
}

func maintenanceEvent(t maintenance.DueTask, loc *time.Location) notify.Event {
	e := notify.Event{
		Type: eventMaintenanceDue, Severity: "info",
		Title:      text("reminders_maintenance_due_title_system", t.Name),
		Body:       text("reminders_maintenance_due_body", t.DueAt.In(loc).Format("Mon 2 Jan 2006, 15:04 MST")),
		Link:       "/maintenance",
		TankID:     t.TankID,
		SourceKind: sourceTask, SourceID: t.ID, Occurrence: db.FormatTime(t.DueAt),
	}
	if t.TankID != nil {
		e.Title = text("reminders_maintenance_due_title_tank", t.Name, t.TankName)
		e.Link = fmt.Sprintf("/tanks/%d/maintenance", *t.TankID)
	}
	return e
}
