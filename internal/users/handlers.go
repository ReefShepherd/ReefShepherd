package users

import (
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

// PublicRoutes registers the endpoints that work before anyone is signed in.
func PublicRoutes(mux *http.ServeMux, deps Deps) {
	svc := NewService(deps)
	mux.Handle("POST /api/v1/setup", httpx.Handle(svc.Logger, svc.setup))
}

// Routes registers the user endpoints; the caller wraps mux in the sign-in check, and the user management routes also require an admin.
func Routes(mux *http.ServeMux, deps Deps) {
	svc := NewService(deps)
	handle := func(h httpx.HandlerFunc) http.Handler { return httpx.Handle(svc.Logger, h) }
	admin := func(h httpx.HandlerFunc) http.Handler { return auth.RequireAdmin(handle(h)) }

	mux.Handle("GET /api/v1/me", handle(svc.getMe))
	mux.Handle("PATCH /api/v1/me", handle(svc.patchMe))
	mux.Handle("GET /api/v1/users", admin(svc.listUsers))
	mux.Handle("POST /api/v1/users", admin(svc.createUser))
	mux.Handle("PATCH /api/v1/users/{id}", admin(svc.patchUser))
	mux.Handle("DELETE /api/v1/users/{id}", admin(svc.deleteUser))
	mux.Handle("POST /api/v1/users/{id}/reset-mfa", admin(svc.resetMFA))
}

func pathUserID(r *http.Request) (int64, error) {
	id, ok := httpx.PathID(r, "id")
	if !ok {
		return 0, errUserNotFound
	}
	return id, nil
}

func actorOf(r *http.Request) (httpx.User, int64) {
	user, _ := httpx.CurrentUser(r.Context())
	session, _ := auth.SessionID(r.Context())
	return user, session
}

func (s *Service) getMe(w http.ResponseWriter, r *http.Request) error {
	user, _ := actorOf(r)
	me, err := s.Me(r.Context(), user.ID)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, me)
	return nil
}

type patchMeRequest struct {
	DisplayName   *string                `json:"display_name"`
	Email         httpx.Nullable[string] `json:"email"`
	Theme         *string                `json:"theme"`
	Locale        *string                `json:"locale"`
	DefaultTankID httpx.Nullable[int64]  `json:"default_tank_id"`
}

func (s *Service) patchMe(w http.ResponseWriter, r *http.Request) error {
	user, _ := actorOf(r)
	var req patchMeRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	me, err := s.UpdateMe(r.Context(), user.ID, UpdateMeInput(req))
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, me)
	return nil
}

func (s *Service) listUsers(w http.ResponseWriter, r *http.Request) error {
	users, err := s.List(r.Context())
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, users)
	return nil
}

type createUserRequest struct {
	Username    string  `json:"username"`
	DisplayName string  `json:"display_name"`
	Email       *string `json:"email"`
	Role        string  `json:"role"`
	Password    string  `json:"password"`
}

func (s *Service) createUser(w http.ResponseWriter, r *http.Request) error {
	var req createUserRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	if req.Role == "" {
		req.Role = httpx.RoleMember
	}
	u, err := s.Create(r.Context(), CreateInput(req))
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusCreated, u)
	return nil
}

type patchUserRequest struct {
	DisplayName *string                `json:"display_name"`
	Email       httpx.Nullable[string] `json:"email"`
	Role        *string                `json:"role"`
	Disabled    *bool                  `json:"disabled"`
	Password    *string                `json:"password"`
}

func (s *Service) patchUser(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUserID(r)
	if err != nil {
		return err
	}
	var req patchUserRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	actor, session := actorOf(r)
	u, err := s.Update(r.Context(), actor, session, id, UpdateInput(req))
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, u)
	return nil
}

func (s *Service) deleteUser(w http.ResponseWriter, r *http.Request) error {
	if err := httpx.RequireJSONOrNoBody(r); err != nil {
		return err
	}
	id, err := pathUserID(r)
	if err != nil {
		return err
	}
	actor, _ := actorOf(r)
	if err := s.Delete(r.Context(), actor, id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Service) resetMFA(w http.ResponseWriter, r *http.Request) error {
	if err := httpx.RequireJSONOrNoBody(r); err != nil {
		return err
	}
	id, err := pathUserID(r)
	if err != nil {
		return err
	}
	actor, session := actorOf(r)
	if err := s.ResetMFA(r.Context(), actor, session, id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
