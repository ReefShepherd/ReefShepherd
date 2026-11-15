package httpx

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandle(t *testing.T) {
	cases := []struct {
		name      string
		h         HandlerFunc
		status    int
		body      string
		wantInLog string
	}{
		{"success is left as written", func(w http.ResponseWriter, _ *http.Request) error {
			w.WriteHeader(http.StatusCreated)
			return nil
		}, http.StatusCreated, "", ""},
		{"api errors are sent as they are", func(http.ResponseWriter, *http.Request) error {
			return NotFound("no such thing")
		}, http.StatusNotFound, `"code":"not_found"`, ""},
		{"wrapped api errors are still found", func(http.ResponseWriter, *http.Request) error {
			return errors.Join(errors.New("context"), Conflict("clash"))
		}, http.StatusConflict, `"code":"conflict"`, ""},
		{"other errors become a logged 500", func(http.ResponseWriter, *http.Request) error {
			return errors.New("disk on fire")
		}, http.StatusInternalServerError, `"message":"internal error"`, "disk on fire"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var logs strings.Builder
			h := Handle(slog.New(slog.NewTextHandler(&logs, nil)), c.h)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/thing", nil))
			if rec.Code != c.status || !strings.Contains(rec.Body.String(), c.body) {
				t.Errorf("status=%d body=%q, want %d containing %q", rec.Code, rec.Body, c.status, c.body)
			}
			if strings.Contains(rec.Body.String(), "disk") {
				t.Errorf("internal detail leaked: %s", rec.Body)
			}
			if c.wantInLog == "" && logs.Len() != 0 {
				t.Errorf("unexpected log: %s", logs.String())
			}
			if !strings.Contains(logs.String(), c.wantInLog) {
				t.Errorf("log = %q, want it to contain %q", logs.String(), c.wantInLog)
			}
		})
	}
}
