// Package tanks owns tanks and their endpoints.
package tanks

import (
	"log/slog"
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/settings"
)

// Deps are the dependencies the package's handlers need.
type Deps struct {
	DB       *db.DB
	Settings *settings.Store
	// Logger receives unexpected failures.
	Logger *slog.Logger
}

// Routes registers the tank endpoints behind the caller's sign-in check, where only an admin can delete a tank.
func Routes(mux *http.ServeMux, deps Deps) {
	svc := NewService(deps)
	handle := func(h httpx.HandlerFunc) http.Handler { return httpx.Handle(svc.Logger, h) }
	mux.Handle("GET /api/v1/tanks", handle(svc.listTanks))
	mux.Handle("POST /api/v1/tanks", handle(svc.createTank))
	mux.Handle("GET /api/v1/tanks/{id}", handle(svc.getTank))
	mux.Handle("PATCH /api/v1/tanks/{id}", handle(svc.patchTank))
	mux.Handle("DELETE /api/v1/tanks/{id}", auth.RequireAdmin(handle(svc.deleteTank)))
	mux.Handle("POST /api/v1/tanks/{id}/archive", handle(svc.archiveTank))
	mux.Handle("POST /api/v1/tanks/{id}/unarchive", handle(svc.unarchiveTank))
}
