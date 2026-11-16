// Package backup creates, lists, prunes and serves database backups.
package backup

import (
	"log/slog"
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/notify"
	"github.com/reefshepherd/reefshepherd/internal/settings"
)

// Deps are the dependencies the package's handlers, service and jobs need.
type Deps struct {
	DB       *db.DB
	Settings *settings.Store
	// Logger receives unexpected failures.
	Logger *slog.Logger
	// Dir is the backups folder inside the data directory.
	Dir    string
	Notify notify.Raiser
}

// Routes registers the backup endpoints behind the caller's sign-in check.
func Routes(_ *http.ServeMux, _ Deps) {}
