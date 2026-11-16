// Package reminders turns due maintenance, due tests and out-of-range results into notifications.
package reminders

import (
	"context"
	"log/slog"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/maintenance"
	"github.com/reefshepherd/reefshepherd/internal/notify"
	"github.com/reefshepherd/reefshepherd/internal/params"
	"github.com/reefshepherd/reefshepherd/internal/scheduler"
	"github.com/reefshepherd/reefshepherd/internal/settings"
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
}

// NewService returns the service for deps.
func NewService(deps Deps) *Service {
	return &Service{Deps: deps, now: time.Now}
}

// ResultCreated is the hook params calls once for each stored result.
func (s *Service) ResultCreated(_ context.Context, _ params.Result) {}

// Jobs returns the background jobs of the package.
func Jobs(_ Deps) []scheduler.Job {
	return nil
}
