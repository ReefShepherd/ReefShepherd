// Package dashboard owns the per-user, per-tank dashboard layout and its endpoints.
package dashboard

import (
	"log/slog"
	"net/http"

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

// Routes registers the dashboard endpoints; the caller wraps mux in the sign-in check, and every layout belongs to the signed-in user.
func Routes(mux *http.ServeMux, deps Deps) {
	svc := NewService(deps)
	handle := func(h httpx.HandlerFunc) http.Handler { return httpx.Handle(svc.Logger, h) }
	mux.Handle("GET /api/v1/tanks/{id}/dashboard", handle(svc.getDashboard))
	mux.Handle("PUT /api/v1/tanks/{id}/dashboard", handle(svc.putDashboard))
	mux.Handle("POST /api/v1/tanks/{id}/dashboard/reset", handle(svc.resetDashboard))
}
