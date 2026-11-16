package notify

import (
	"errors"
	"net/http"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/scheduler"
)

// ErrChannelNotFound is the error for a notification channel that does not exist; its text is the message of the 404 the handlers send.
var ErrChannelNotFound = errors.New("no such notification channel")

// ErrInvalidEvent is the error for an event that cannot be raised, which is a mistake of the code raising it.
var ErrInvalidEvent = errors.New("invalid notification event")

// sendTimeout bounds one send to a channel.
const sendTimeout = 10 * time.Second

// Service stores notifications, queues their deliveries and serves the channel and inbox API.
type Service struct {
	Deps
	now func() time.Time
}

var _ Raiser = (*Service)(nil)

// NewService returns the service for deps.
func NewService(deps Deps) *Service {
	if deps.HTTP == nil {
		deps.HTTP = &http.Client{Timeout: sendTimeout}
	}
	return &Service{Deps: deps, now: time.Now}
}

// Jobs returns the background jobs of the package: the dispatcher that sends queued deliveries and the daily prune of old notifications.
func Jobs(deps Deps) []scheduler.Job {
	svc := NewService(deps)
	return []scheduler.Job{
		{Name: "notifications-dispatch", Trigger: scheduler.EveryTick(), Timeout: dispatchTimeout, Run: svc.Dispatch},
		{Name: "notifications-prune", Trigger: scheduler.Every(24 * time.Hour), Run: svc.Prune},
	}
}
