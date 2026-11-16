package backup

import (
	"time"

	"github.com/reefshepherd/reefshepherd/internal/scheduler"
)

// Service creates and prunes backups.
type Service struct {
	Deps
	now func() time.Time
}

// NewService returns the service for deps.
func NewService(deps Deps) *Service {
	return &Service{Deps: deps, now: time.Now}
}

// Jobs returns the background jobs of the package.
func Jobs(_ Deps) []scheduler.Job {
	return nil
}
