package httpx

import "net/http"

// IncludeArchived reads the optional include_archived query flag, false when it is absent.
func IncludeArchived(r *http.Request) (bool, error) {
	v, _, err := QueryBool(r, "include_archived")
	return v, err
}
