// Package maintenance owns maintenance tasks, their presets and completion history.
package maintenance

import (
	"log/slog"
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/settings"
)

// Deps are the dependencies the package's handlers and service need.
type Deps struct {
	DB       *db.DB
	Settings *settings.Store
	// Logger receives unexpected failures.
	Logger *slog.Logger
}

// Routes registers the maintenance endpoints behind the caller's sign-in check.
func Routes(_ *http.ServeMux, _ Deps) {}
