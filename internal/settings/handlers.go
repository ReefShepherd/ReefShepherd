package settings

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/units"
)

// CodeEnvLocked is the error code for a setting that the server's environment decides.
const CodeEnvLocked = "env_locked"

// Deps are the dependencies the package's handlers need.
type Deps struct {
	DB       *db.DB
	Settings *Store
	Auth     *auth.Service
	Logger   *slog.Logger
}

// PublicRoutes registers the endpoints that work without a session; the unit catalog holds nothing private and the setup wizard needs it before anyone signs in.
func PublicRoutes(mux *http.ServeMux, deps Deps) {
	h := &handlers{Deps: deps}
	mux.Handle("GET /api/v1/units", httpx.Handle(deps.Logger, h.getUnits))
}

// Routes registers the settings endpoints; the caller wraps mux in the sign-in check, and the settings routes also require an admin.
func Routes(mux *http.ServeMux, deps Deps) {
	h := &handlers{Deps: deps}
	mux.Handle("GET /api/v1/settings", auth.RequireAdmin(httpx.Handle(deps.Logger, h.getSettings)))
	mux.Handle("PATCH /api/v1/settings", auth.RequireAdmin(httpx.Handle(deps.Logger, h.patchSettings)))
}

type handlers struct {
	Deps
}

func (h *handlers) getUnits(w http.ResponseWriter, _ *http.Request) error {
	httpx.WriteJSON(w, http.StatusOK, units.Catalog())
	return nil
}

type settingsResponse struct {
	InstanceName        string            `json:"instance_name"`
	Timezone            string            `json:"timezone"`
	ReminderDefaultTime string            `json:"reminder_default_time"`
	Units               map[string]string `json:"units"`
	// AuthMode is the mode in force, which an environment override may set.
	AuthMode string `json:"auth_mode"`
	// EnvLocked maps each setting the server environment decides to the variable that does.
	EnvLocked map[string]string `json:"env_locked"`
}

func (h *handlers) snapshot(r *http.Request) (settingsResponse, error) {
	ctx := r.Context()
	v, err := h.Settings.Values(ctx)
	if err != nil {
		return settingsResponse{}, err
	}
	mode, err := h.Auth.EffectiveMode(ctx)
	if err != nil {
		return settingsResponse{}, err
	}
	out := settingsResponse{
		InstanceName: v.InstanceName, Timezone: v.Timezone, ReminderDefaultTime: v.ReminderDefaultTime,
		Units: map[string]string{}, AuthMode: mode, EnvLocked: map[string]string{},
	}
	for family, unit := range v.Units {
		out.Units[string(family)] = string(unit)
	}
	if h.Auth.Config.AuthMode != "" {
		out.EnvLocked["auth_mode"] = config.EnvAuthMode
	}
	return out, nil
}

func (h *handlers) getSettings(w http.ResponseWriter, r *http.Request) error {
	out, err := h.snapshot(r)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, out)
	return nil
}

type patchSettingsRequest struct {
	InstanceName        *string           `json:"instance_name"`
	Timezone            *string           `json:"timezone"`
	ReminderDefaultTime *string           `json:"reminder_default_time"`
	Units               map[string]string `json:"units"`
	AuthMode            *string           `json:"auth_mode"`
	// Password is the admin's own password, required to turn login off.
	Password string `json:"password"`
}

func (h *handlers) patchSettings(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var req patchSettingsRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	user, _ := httpx.CurrentUser(ctx)

	changes := Changes{InstanceName: req.InstanceName, Timezone: req.Timezone, ReminderDefaultTime: req.ReminderDefaultTime, Units: map[units.Family]units.Unit{}}
	for family, unit := range req.Units {
		changes.Units[units.Family(family)] = units.Unit(unit)
	}

	newMode := ""
	if req.AuthMode != nil {
		effective, err := h.Auth.EffectiveMode(ctx)
		if err != nil {
			return err
		}
		if *req.AuthMode != effective {
			if locked := h.Auth.Config.AuthMode; locked != "" {
				return &httpx.APIError{
					Status:  http.StatusConflict,
					Code:    CodeEnvLocked,
					Message: "the login mode is set by " + config.EnvAuthMode + " on the server",
					Fields:  map[string]string{"auth_mode": "set by " + config.EnvAuthMode + " on the server"},
				}
			}
			newMode = *req.AuthMode
		}
	}
	if newMode == config.AuthModeDisabled {
		// Turning login off hands the app to anyone who can reach it, so the admin has to prove who they are first.
		if err := h.Auth.ConfirmPassword(ctx, r, user.ID, req.Password, "password"); err != nil {
			return err
		}
	}

	err := h.DB.InTx(ctx, func(tx *sql.Tx) error {
		store := h.Settings.WithTx(tx)
		rejected, err := store.Apply(ctx, changes)
		if err != nil {
			return err
		}
		if newMode != "" {
			if err := store.SwitchAuthMode(ctx, newMode); errors.Is(err, ErrInvalid) {
				rejected["auth_mode"] = strings.TrimPrefix(err.Error(), ErrInvalid.Error()+": ")
			} else if err != nil {
				return err
			}
		}
		if len(rejected) > 0 {
			return httpx.Validation("invalid settings", rejected)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return h.getSettings(w, r)
}
