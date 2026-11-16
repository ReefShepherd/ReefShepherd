package notify

import (
	"log/slog"
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/settings"
)

// Deps are the dependencies the package's handlers, service and jobs need.
type Deps struct {
	DB       *db.DB
	Settings *settings.Store
	// Logger receives unexpected failures.
	Logger *slog.Logger
	// PublicURL is the external URL without a trailing slash, empty when unset.
	PublicURL string
	// HTTP is the client providers send with; nil means a client with a 10 s timeout.
	HTTP *http.Client
}

// Routes registers the notification endpoints behind the caller's sign-in check.
func Routes(_ *http.ServeMux, _ Deps) {}
