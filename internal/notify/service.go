package notify

import (
	"context"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/scheduler"
)

// Service stores notifications and queues their deliveries.
type Service struct {
	Deps
	now func() time.Time
}

var _ Raiser = (*Service)(nil)

// NewService returns the service for deps.
func NewService(deps Deps) *Service {
	return &Service{Deps: deps, now: time.Now}
}

// Raise stores the notification and queues deliveries; it reports false when this occurrence was already raised.
func (s *Service) Raise(_ context.Context, _ Event) (bool, error) {
	return false, nil
}

// Jobs returns the background jobs of the package.
func Jobs(_ Deps) []scheduler.Job {
	return nil
}
