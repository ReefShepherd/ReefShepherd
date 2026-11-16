// Package backup creates, lists, prunes and serves database backups.
package backup

import (
	"log/slog"
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
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

// Routes registers the backup endpoints, which are for admins only; the caller wraps mux in the sign-in check.
func Routes(mux *http.ServeMux, deps Deps) {
	svc := NewService(deps)
	admin := func(h httpx.HandlerFunc) http.Handler {
		return auth.RequireAdmin(httpx.Handle(svc.Logger, h, apiError))
	}
	mux.Handle("GET /api/v1/backups", admin(svc.listBackups))
	mux.Handle("POST /api/v1/backups", admin(svc.createBackup))
	mux.Handle("GET /api/v1/backups/{name}/download", admin(svc.downloadBackup))
	mux.Handle("DELETE /api/v1/backups/{name}", admin(svc.deleteBackup))
}
