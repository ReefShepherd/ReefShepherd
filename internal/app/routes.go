package app

import (
	"log/slog"
	"net/http"
	"path/filepath"

	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/backup"
	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/dashboard"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/maintenance"
	"github.com/reefshepherd/reefshepherd/internal/notify"
	"github.com/reefshepherd/reefshepherd/internal/params"
	"github.com/reefshepherd/reefshepherd/internal/reminders"
	"github.com/reefshepherd/reefshepherd/internal/settings"
	"github.com/reefshepherd/reefshepherd/internal/tanks"
	"github.com/reefshepherd/reefshepherd/internal/users"
	"github.com/reefshepherd/reefshepherd/internal/version"
)

// routeDeps are the shared services every feature package's routes and jobs are built from.
type routeDeps struct {
	Config   config.Config
	Logger   *slog.Logger
	DB       *db.DB
	Settings *settings.Store
	Auth     *auth.Service

	Notify      notify.Deps
	Maintenance maintenance.Deps
	Backup      backup.Deps
	// Reminders is the service behind params' result hook, and RemindersDeps is what its jobs are built from.
	Reminders     *reminders.Service
	RemindersDeps reminders.Deps
}

// newRouteDeps builds the services the packages share, so that routes and jobs use the same notification service.
func newRouteDeps(cfg config.Config, logger *slog.Logger, database *db.DB, store *settings.Store, authService *auth.Service) routeDeps {
	notifyDeps := notify.Deps{DB: database, Settings: store, Logger: logger, PublicURL: cfg.PublicURL}
	notifyService := notify.NewService(notifyDeps)
	maintenanceDeps := maintenance.Deps{DB: database, Settings: store, Logger: logger}
	remindersDeps := reminders.Deps{
		DB: database, Settings: store, Logger: logger,
		Notify:      notifyService,
		Maintenance: maintenance.NewService(maintenanceDeps),
		// Built without the result hook, because the hook is what calls the reminders.
		Params: params.NewService(params.Deps{DB: database, Settings: store, Logger: logger}),
	}
	return routeDeps{
		Config: cfg, Logger: logger, DB: database, Settings: store, Auth: authService,
		Notify:      notifyDeps,
		Maintenance: maintenanceDeps,
		Backup: backup.Deps{
			DB: database, Settings: store, Logger: logger,
			Dir:    filepath.Join(cfg.DataDir, backupsDirName),
			Notify: notifyService,
		},
		Reminders:     reminders.NewService(remindersDeps),
		RemindersDeps: remindersDeps,
	}
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
	params.Routes(protected, params.Deps{DB: d.DB, Settings: d.Settings, Logger: d.Logger, OnResultCreated: d.Reminders.ResultCreated})
	dashboard.Routes(protected, dashboard.Deps{DB: d.DB, Logger: d.Logger})
	maintenance.Routes(protected, d.Maintenance)
	notify.Routes(protected, d.Notify)
	backup.Routes(protected, d.Backup)

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
