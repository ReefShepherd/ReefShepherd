// Package params owns water parameters, test results and their endpoints.
package params

import "net/http"

// Deps are the dependencies the package's handlers need.
type Deps struct{}

// Routes registers the package's HTTP routes on mux.
func Routes(mux *http.ServeMux, deps Deps) {}
