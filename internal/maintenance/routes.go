// Package maintenance owns maintenance tasks, their presets and completion history.
package maintenance

import (
	"log/slog"
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
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
func Routes(mux *http.ServeMux, deps Deps) {
	NewService(deps).register(mux)
}

func (s *Service) register(mux *http.ServeMux) {
	handle := func(h httpx.HandlerFunc) http.Handler { return httpx.Handle(s.Logger, h, apiError) }
	mux.Handle("GET /api/v1/maintenance/defaults", handle(s.getDefaults))

	mux.Handle("GET /api/v1/maintenance/presets", handle(s.listPresets))
	mux.Handle("POST /api/v1/maintenance/presets", handle(s.createPreset))
	mux.Handle("PATCH /api/v1/maintenance/presets/{id}", handle(s.patchPreset))

	mux.Handle("GET /api/v1/maintenance/tasks", handle(s.listTasks))
	mux.Handle("POST /api/v1/maintenance/tasks", handle(s.createTask))
	mux.Handle("GET /api/v1/maintenance/tasks/{id}", handle(s.getTask))
	mux.Handle("PATCH /api/v1/maintenance/tasks/{id}", handle(s.patchTask))
	mux.Handle("DELETE /api/v1/maintenance/tasks/{id}", handle(s.deleteTask))
	mux.Handle("POST /api/v1/maintenance/tasks/{id}/complete", handle(s.completeTask))
	mux.Handle("GET /api/v1/maintenance/tasks/{id}/completions", handle(s.listCompletions))

	mux.Handle("PATCH /api/v1/maintenance/completions/{id}", handle(s.patchCompletion))
	mux.Handle("DELETE /api/v1/maintenance/completions/{id}", handle(s.deleteCompletion))
}
