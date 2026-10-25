package dashboard

import (
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/tanks"
)

// target returns the signed-in user and the tank named in the path.
func target(r *http.Request) (userID, tankID int64, err error) {
	user, _ := httpx.CurrentUser(r.Context())
	tankID, ok := httpx.PathID(r, "id")
	if !ok {
		return 0, 0, tanks.ErrNotFound
	}
	return user.ID, tankID, nil
}

func (s *Service) getDashboard(w http.ResponseWriter, r *http.Request) error {
	userID, tankID, err := target(r)
	if err != nil {
		return err
	}
	layout, err := s.Get(r.Context(), userID, tankID)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, layout)
	return nil
}

type putRequest struct {
	Cards []CardInput `json:"cards"`
}

func (s *Service) putDashboard(w http.ResponseWriter, r *http.Request) error {
	userID, tankID, err := target(r)
	if err != nil {
		return err
	}
	var req putRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	layout, err := s.Replace(r.Context(), userID, tankID, req.Cards)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, layout)
	return nil
}

func (s *Service) resetDashboard(w http.ResponseWriter, r *http.Request) error {
	if err := httpx.RequireJSONOrNoBody(r); err != nil {
		return err
	}
	userID, tankID, err := target(r)
	if err != nil {
		return err
	}
	layout, err := s.Reset(r.Context(), userID, tankID)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, layout)
	return nil
}
