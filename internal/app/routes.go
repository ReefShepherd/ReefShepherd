package app

import (
	"log/slog"
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/dashboard"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/params"
	"github.com/reefshepherd/reefshepherd/internal/settings"
	"github.com/reefshepherd/reefshepherd/internal/tanks"
	"github.com/reefshepherd/reefshepherd/internal/users"
	"github.com/reefshepherd/reefshepherd/internal/version"
)

// routeDeps are the shared services every feature package's routes are built from.
type routeDeps struct {
	Config   config.Config
	Logger   *slog.Logger
	DB       *db.DB
	Settings *settings.Store
	Auth     *auth.Service
}

// registerRoutes is the single route table, and everything under /api/ requires a signed-in user except the explicitly public routes.
func registerRoutes(mux *http.ServeMux, d routeDeps, ui http.Handler) {
	mux.HandleFunc("GET /healthz", healthz)

	usersDeps := users.Deps{DB: d.DB, Settings: d.Settings, Auth: d.Auth, Logger: d.Logger}
	d.Auth.PublicRoutes(mux)
	users.PublicRoutes(mux, usersDeps)
	settings.PublicRoutes(mux, settings.Deps{DB: d.DB, Settings: d.Settings, Auth: d.Auth, Logger: d.Logger})

	protected := http.NewServeMux()
	d.Auth.Routes(protected)
	users.Routes(protected, usersDeps)
	settings.Routes(protected, settings.Deps{DB: d.DB, Settings: d.Settings, Auth: d.Auth, Logger: d.Logger})
	tanks.Routes(protected, tanks.Deps{DB: d.DB, Settings: d.Settings, Logger: d.Logger})
	params.Routes(protected, params.Deps{DB: d.DB, Settings: d.Settings, Logger: d.Logger})
	dashboard.Routes(protected, dashboard.Deps{DB: d.DB, Settings: d.Settings, Logger: d.Logger})

	// Any API path without a more specific route is a JSON 404, never the single-page-app fallback.
	protected.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteError(w, httpx.NotFound("no such API endpoint"))
	})
	mux.Handle("/api/", d.Auth.RequireUser(protected))
	mux.Handle("/", ui)
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": version.Version()})
}
