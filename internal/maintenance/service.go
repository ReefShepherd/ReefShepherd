package maintenance

import (
	"context"
	"time"
)

// Service is the maintenance logic behind the HTTP handlers, the scheduler and the reminders.
type Service struct {
	Deps
	now func() time.Time
}

// NewService returns the service for deps.
func NewService(deps Deps) *Service {
	return &Service{Deps: deps, now: time.Now}
}

// DueTask is a maintenance task that is due for a reminder.
type DueTask struct {
	ID       int64
	TankID   *int64
	TankName string // empty for whole-system tasks
	Name     string
	DueAt    time.Time
}

// DueForReminders lists tasks that are not paused, have notify on, do not belong to an archived tank, and are due at or before now.
func (s *Service) DueForReminders(_ context.Context, _ time.Time) ([]DueTask, error) {
	return nil, nil
}
