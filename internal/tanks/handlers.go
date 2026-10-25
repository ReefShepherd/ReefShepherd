package tanks

import (
	"context"
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

func pathTankID(r *http.Request) (int64, error) {
	id, ok := httpx.PathID(r, "id")
	if !ok {
		return 0, ErrNotFound
	}
	return id, nil
}

func (s *Service) listTanks(w http.ResponseWriter, r *http.Request) error {
	include, err := httpx.IncludeArchived(r)
	if err != nil {
		return err
	}
	tanks, err := s.List(r.Context(), include)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, tanks)
	return nil
}

type createTankRequest struct {
	Name      string   `json:"name"`
	TankType  string   `json:"tank_type"`
	Volume    *float64 `json:"volume"`
	StartedOn *string  `json:"started_on"`
	Note      string   `json:"note"`
	Presets   []string `json:"presets"`
}

func (s *Service) createTank(w http.ResponseWriter, r *http.Request) error {
	var req createTankRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	tank, err := s.Create(r.Context(), CreateInput(req))
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusCreated, tank)
	return nil
}

func (s *Service) getTank(w http.ResponseWriter, r *http.Request) error {
	id, err := pathTankID(r)
	if err != nil {
		return err
	}
	tank, err := s.Get(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, tank)
	return nil
}

// patchTankRequest keeps every field nullable so an explicit null can be told from an absent field.
type patchTankRequest struct {
	Name      httpx.Nullable[string]  `json:"name"`
	TankType  httpx.Nullable[string]  `json:"tank_type"`
	Volume    httpx.Nullable[float64] `json:"volume"`
	StartedOn httpx.Nullable[string]  `json:"started_on"`
	Note      httpx.Nullable[string]  `json:"note"`
	SortOrder httpx.Nullable[int64]   `json:"sort_order"`
}

// input turns the request into an update, reporting a null for a field that cannot be cleared.
func (req patchTankRequest) input() (UpdateInput, error) {
	fields := map[string]string{}
	required := func(field string, n httpx.Nullable[string]) *string {
		if n.Set && n.Value == nil {
			fields[field] = "must not be null"
		}
		return n.Value
	}
	in := UpdateInput{Name: required("name", req.Name), TankType: required("tank_type", req.TankType), Note: required("note", req.Note), Volume: req.Volume, StartedOn: req.StartedOn}
	if req.SortOrder.Set && req.SortOrder.Value == nil {
		fields["sort_order"] = "must not be null"
	}
	in.SortOrder = req.SortOrder.Value
	if len(fields) > 0 {
		return UpdateInput{}, httpx.Validation("invalid tank", fields)
	}
	return in, nil
}

func (s *Service) patchTank(w http.ResponseWriter, r *http.Request) error {
	id, err := pathTankID(r)
	if err != nil {
		return err
	}
	var req patchTankRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	in, err := req.input()
	if err != nil {
		return err
	}
	tank, err := s.Update(r.Context(), id, in)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, tank)
	return nil
}

func (s *Service) archiveTank(w http.ResponseWriter, r *http.Request) error {
	return s.setArchivedHandler(w, r, s.Archive)
}

func (s *Service) unarchiveTank(w http.ResponseWriter, r *http.Request) error {
	return s.setArchivedHandler(w, r, s.Unarchive)
}

func (s *Service) setArchivedHandler(w http.ResponseWriter, r *http.Request, change func(ctx context.Context, id int64) (Tank, error)) error {
	if err := httpx.RequireJSONOrNoBody(r); err != nil {
		return err
	}
	id, err := pathTankID(r)
	if err != nil {
		return err
	}
	tank, err := change(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, tank)
	return nil
}

func (s *Service) deleteTank(w http.ResponseWriter, r *http.Request) error {
	if err := httpx.RequireJSONOrNoBody(r); err != nil {
		return err
	}
	id, err := pathTankID(r)
	if err != nil {
		return err
	}
	if err := s.Delete(r.Context(), id, r.URL.Query().Get("confirm")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
