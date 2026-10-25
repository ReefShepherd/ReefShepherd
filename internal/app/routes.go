package app

import (
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/dashboard"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/params"
	"github.com/reefshepherd/reefshepherd/internal/settings"
	"github.com/reefshepherd/reefshepherd/internal/tanks"
	"github.com/reefshepherd/reefshepherd/internal/users"
	"github.com/reefshepherd/reefshepherd/internal/version"
)

// registerRoutes is the single route table; each feature package fills in its own Routes function.
func registerRoutes(mux *http.ServeMux, database *db.DB, store *settings.Store, ui http.Handler) {
	mux.HandleFunc("GET /healthz", healthz)

	auth.Routes(mux, auth.Deps{DB: database, Settings: store})
	users.Routes(mux, users.Deps{DB: database, Settings: store})
	settings.Routes(mux, settings.Deps{DB: database, Settings: store})
	tanks.Routes(mux, tanks.Deps{DB: database, Settings: store})
	params.Routes(mux, params.Deps{DB: database, Settings: store})
	dashboard.Routes(mux, dashboard.Deps{DB: database, Settings: store})

	// Any API path without a more specific route is a JSON 404, never the single-page-app fallback.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteError(w, httpx.NotFound("no such API endpoint"))
	})
	mux.Handle("/", ui)
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": version.Version()})
}
