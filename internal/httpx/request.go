package httpx

import (
	"net/http"
	"strconv"
)

// IncludeArchived reads the optional include_archived query flag, false when it is absent.
func IncludeArchived(r *http.Request) (bool, error) {
	raw := r.URL.Query().Get("include_archived")
	if raw == "" {
		return false, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, rejection("invalid query", "include_archived")
	}
	return v, nil
}
