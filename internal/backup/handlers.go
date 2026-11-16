package backup

import (
	"errors"
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

func apiError(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return httpx.NotFound(ErrNotFound.Error())
	case errors.Is(err, ErrNameTaken):
		return httpx.Conflict(ErrNameTaken.Error())
	}
	return err
}

func (s *Service) listBackups(w http.ResponseWriter, _ *http.Request) error {
	items, err := s.List()
	if err != nil {
		return err
	}
	httpx.List(w, http.StatusOK, items)
	return nil
}

func (s *Service) createBackup(w http.ResponseWriter, r *http.Request) error {
	if err := httpx.RequireJSONOrNoBody(r); err != nil {
		return err
	}
	b, err := s.Create(r.Context())
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusCreated, b)
	return nil
}

func (s *Service) downloadBackup(w http.ResponseWriter, r *http.Request) error {
	f, b, err := s.Open(r.PathValue("name"))
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	h := w.Header()
	h.Set("Content-Type", "application/vnd.sqlite3")
	h.Set("Content-Disposition", `attachment; filename="`+b.Name+`"`)
	h.Set("Cache-Control", "no-store")
	http.ServeContent(w, r, b.Name, info.ModTime(), f)
	return nil
}

func (s *Service) deleteBackup(w http.ResponseWriter, r *http.Request) error {
	if err := httpx.RequireJSONOrNoBody(r); err != nil {
		return err
	}
	if err := s.Delete(r.PathValue("name")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
