package httpx

import (
	"log/slog"
	"net/http"
)

// HandlerFunc is a handler that reports failure by returning an error.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// Handle adapts h so that a returned error, first passed through mapErr to turn a package's sentinels into API errors, reaches the client like every other failure and is logged when unexpected.
func Handle(log *slog.Logger, h HandlerFunc, mapErr ...func(error) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := h(w, r)
		if err == nil {
			return
		}
		for _, m := range mapErr {
			err = m(err)
		}
		Fail(log, w, r, err)
	})
}
