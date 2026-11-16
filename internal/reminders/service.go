// Package reminders turns due maintenance, due tests and out-of-range results into notifications.
package reminders

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/i18n"
	"github.com/reefshepherd/reefshepherd/internal/maintenance"
	"github.com/reefshepherd/reefshepherd/internal/notify"
	"github.com/reefshepherd/reefshepherd/internal/params"
	"github.com/reefshepherd/reefshepherd/internal/settings"
)

// Notification event types and the kinds of source row they are raised for.
const (
	eventMaintenanceDue = "maintenance_due"
	eventTestsDue       = "tests_due"
	eventOutOfRange     = "out_of_range"

	sourceTask   = "maintenance_task"
	sourceTank   = "tank_tests"
	sourceResult = "test_result"
)

// Deps are the dependencies the package's service and jobs need.
type Deps struct {
	DB       *db.DB
	Settings *settings.Store
	Logger   *slog.Logger
	Notify   notify.Raiser
	// Maintenance lists the tasks that are due.
	Maintenance *maintenance.Service
	// Params is built without the result hook, so raising from the hook cannot recurse.
	Params *params.Service
}

// Service raises reminders and alerts.
type Service struct {
	Deps
	now func() time.Time

	// digestDay is the local date whose tests summary pass has finished, so later ticks that day skip the pass.
	digestDay string
}

// NewService returns the service for deps.
func NewService(deps Deps) *Service {
	return &Service{Deps: deps, now: time.Now}
}

// location returns the instance timezone.
func (s *Service) location(ctx context.Context) (*time.Location, error) {
	name, err := s.Settings.GetTimezone(ctx)
	if err != nil {
		return nil, fmt.Errorf("read timezone: %w", err)
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("load timezone %q: %w", name, err)
	}
	return loc, nil
}

// text renders a message of the default locale.
func text(key string, args ...any) string {
	return i18n.Default().T(i18n.DefaultLocale, key, args...)
}
