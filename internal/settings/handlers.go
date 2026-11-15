package settings

import (
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/units"
)

// PublicRoutes registers the endpoints that work without a session; the unit catalog holds nothing private and the setup wizard needs it before anyone signs in.
func PublicRoutes(mux *http.ServeMux, deps Deps) {
	mux.Handle("GET /api/v1/units", httpx.Handle(deps.Logger, NewService(deps).getUnits))
}

// Routes registers the settings endpoints; the caller wraps mux in the sign-in check, and the settings routes also require an admin.
func Routes(mux *http.ServeMux, deps Deps) {
	svc := NewService(deps)
	admin := func(h httpx.HandlerFunc) http.Handler {
		return auth.RequireAdmin(httpx.Handle(deps.Logger, h, auth.APIError))
	}
	mux.Handle("GET /api/v1/settings", admin(svc.getSettings))
	mux.Handle("PATCH /api/v1/settings", admin(svc.patchSettings))
}

func (s *Service) getUnits(w http.ResponseWriter, _ *http.Request) error {
	httpx.List(w, http.StatusOK, units.Catalog())
	return nil
}

func (s *Service) getSettings(w http.ResponseWriter, r *http.Request) error {
	out, err := s.Get(r.Context())
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
	Password            string            `json:"password"`
}

func (s *Service) patchSettings(w http.ResponseWriter, r *http.Request) error {
	var req patchSettingsRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	user, _ := httpx.CurrentUser(r.Context())
	out, err := s.Patch(r.Context(), r, user.ID, PatchInput(req))
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, out)
	return nil
}
