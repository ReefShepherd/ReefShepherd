// Package users owns user accounts and per-user preferences.
package users

import "net/http"

// Deps are the dependencies the package's handlers need.
type Deps struct{}

// Routes registers the package's HTTP routes on mux.
func Routes(mux *http.ServeMux, deps Deps) {}
