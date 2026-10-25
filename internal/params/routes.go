// Package params owns water parameters, test results and their endpoints.
package params

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/settings"
)

// Deps are the dependencies the package's handlers need.
type Deps struct {
	DB       *db.DB
	Settings *settings.Store
	Logger   *slog.Logger
	// OnResultCreated, when set, is called once per stored result after the transaction that wrote it has committed.
	OnResultCreated func(ctx context.Context, r Result)
}

// Routes registers the package's HTTP routes on mux; the caller wraps mux in the sign-in check.
func Routes(mux *http.ServeMux, deps Deps) {
	NewService(deps).register(mux)
}

func (s *Service) register(mux *http.ServeMux) {
	handle := func(h httpx.HandlerFunc) http.Handler { return httpx.Handle(s.Logger, h) }
	mux.Handle("GET /api/v1/parameters", handle(s.listParameters))
	mux.Handle("POST /api/v1/parameters", handle(s.createParameter))
	mux.Handle("PATCH /api/v1/parameters/{id}", handle(s.patchParameter))
	mux.Handle("POST /api/v1/parameters/{id}/archive", handle(s.archiveParameter))
	mux.Handle("POST /api/v1/parameters/{id}/unarchive", handle(s.unarchiveParameter))
	mux.Handle("DELETE /api/v1/parameters/{id}", handle(s.deleteParameter))

	mux.Handle("GET /api/v1/tanks/{id}/parameters", handle(s.listTankParameters))
	mux.Handle("PUT /api/v1/tanks/{id}/parameters/{pid}", handle(s.putTankParameter))
	mux.Handle("GET /api/v1/tanks/{id}/parameters/{pid}/series", handle(s.getSeries))

	mux.Handle("POST /api/v1/tanks/{id}/results", handle(s.createResults))
	mux.Handle("GET /api/v1/tanks/{id}/results", handle(s.listResults))
	mux.Handle("PATCH /api/v1/results/{id}", handle(s.patchResult))
	mux.Handle("DELETE /api/v1/results/{id}", handle(s.deleteResult))
}
