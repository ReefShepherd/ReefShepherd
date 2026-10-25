package httpx

import (
	"log/slog"
	"net/http"
)

// HandlerFunc is a handler that reports failure by returning an error.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// Handle adapts h so that a returned error takes the same path to the client as every other failure, logged through log when unexpected.
func Handle(log *slog.Logger, h HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			Fail(log, w, r, err)
		}
	})
}
