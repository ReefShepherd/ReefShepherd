// Package tanks owns tanks and their endpoints.
package tanks

import (
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/settings"
)

// Deps are the dependencies the package's handlers need.
type Deps struct {
	DB       *db.DB
	Settings *settings.Store
}

// Routes registers the package's HTTP routes on mux.
func Routes(mux *http.ServeMux, deps Deps) {}
