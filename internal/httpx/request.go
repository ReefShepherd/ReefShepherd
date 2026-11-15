package httpx

import (
	"net/http"
	"strconv"
)

// RequireUser returns the signed-in user, or a 401 error when the request is anonymous.
func RequireUser(r *http.Request) (User, error) {
	u, ok := CurrentUser(r.Context())
	if !ok {
		return User{}, Unauthorized("sign in required")
	}
	return u, nil
}

// IncludeArchived reads the optional include_archived query flag, false when it is absent.
func IncludeArchived(r *http.Request) (bool, error) {
	raw := r.URL.Query().Get("include_archived")
	if raw == "" {
		return false, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, Validation("invalid query", map[string]string{"include_archived": "must be true or false"})
	}
	return v, nil
}
