package app

import (
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/dashboard"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/params"
	"github.com/reefshepherd/reefshepherd/internal/settings"
	"github.com/reefshepherd/reefshepherd/internal/tanks"
	"github.com/reefshepherd/reefshepherd/internal/users"
	"github.com/reefshepherd/reefshepherd/internal/version"
	"github.com/reefshepherd/reefshepherd/internal/webui"
)

// registerRoutes is the single route table; each feature package fills in its own Routes function.
func registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", healthz)

	auth.Routes(mux, auth.Deps{})
	users.Routes(mux, users.Deps{})
	settings.Routes(mux, settings.Deps{})
	tanks.Routes(mux, tanks.Deps{})
	params.Routes(mux, params.Deps{})
	dashboard.Routes(mux, dashboard.Deps{})

	// Any API path without a more specific route is a JSON 404, never the single-page-app fallback.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteError(w, httpx.NotFound("no such API endpoint"))
	})
	mux.Handle("/", webui.Handler())
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": version.Version()})
}
