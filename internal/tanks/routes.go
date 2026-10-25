// Package tanks owns tanks and their endpoints.
package tanks

import (
	"log/slog"
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/settings"
)

// Deps are the dependencies the package's handlers need.
type Deps struct {
	DB       *db.DB
	Settings *settings.Store
	// Logger receives unexpected failures; nil means slog.Default.
	Logger *slog.Logger
}

// Routes registers the tank endpoints behind the caller's sign-in check, where only an admin can delete a tank.
func Routes(mux *http.ServeMux, deps Deps) {
	svc := NewService(deps)
	mux.HandleFunc("GET /api/v1/tanks", svc.handle(svc.listTanks))
	mux.HandleFunc("POST /api/v1/tanks", svc.handle(svc.createTank))
	mux.HandleFunc("GET /api/v1/tanks/{id}", svc.handle(svc.getTank))
	mux.HandleFunc("PATCH /api/v1/tanks/{id}", svc.handle(svc.patchTank))
	mux.Handle("DELETE /api/v1/tanks/{id}", auth.RequireAdmin(svc.handle(svc.deleteTank)))
	mux.HandleFunc("POST /api/v1/tanks/{id}/archive", svc.handle(svc.archiveTank))
	mux.HandleFunc("POST /api/v1/tanks/{id}/unarchive", svc.handle(svc.unarchiveTank))
}
