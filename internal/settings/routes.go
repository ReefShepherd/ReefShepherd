// Package settings owns the instance settings and their endpoints.
package settings

import (
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/db"
)

// Deps are the dependencies the package's handlers need.
type Deps struct {
	DB       *db.DB
	Settings *Store
}

// Routes registers the package's HTTP routes on mux.
func Routes(mux *http.ServeMux, deps Deps) {}
