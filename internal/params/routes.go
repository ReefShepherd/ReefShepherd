// Package params owns water parameters, test results and their endpoints.
package params

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/db"
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
	mux.HandleFunc("GET /api/v1/parameters", s.handle(s.listParameters))
	mux.HandleFunc("POST /api/v1/parameters", s.handle(s.createParameter))
	mux.HandleFunc("PATCH /api/v1/parameters/{id}", s.handle(s.patchParameter))
	mux.HandleFunc("POST /api/v1/parameters/{id}/archive", s.handle(s.archiveParameter))
	mux.HandleFunc("POST /api/v1/parameters/{id}/unarchive", s.handle(s.unarchiveParameter))
	mux.HandleFunc("DELETE /api/v1/parameters/{id}", s.handle(s.deleteParameter))

	mux.HandleFunc("GET /api/v1/tanks/{id}/parameters", s.handle(s.listTankParameters))
	mux.HandleFunc("PUT /api/v1/tanks/{id}/parameters/{pid}", s.handle(s.putTankParameter))
	mux.HandleFunc("GET /api/v1/tanks/{id}/parameters/{pid}/series", s.handle(s.getSeries))

	mux.HandleFunc("POST /api/v1/tanks/{id}/results", s.handle(s.createResults))
	mux.HandleFunc("GET /api/v1/tanks/{id}/results", s.handle(s.listResults))
	mux.HandleFunc("PATCH /api/v1/results/{id}", s.handle(s.patchResult))
	mux.HandleFunc("DELETE /api/v1/results/{id}", s.handle(s.deleteResult))
}
