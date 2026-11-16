package httpx

import (
	"net/http"
	"strconv"
)

// QueryID reads the optional query parameter name as a positive integer id; set is false when it is absent or empty.
func QueryID(r *http.Request, name string) (id int64, set bool, err error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, false, nil
	}
	id, err = strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 1 {
		return 0, false, rejection("invalid query", name)
	}
	return id, true, nil
}

// QueryBool reads the optional query parameter name as a boolean; set is false when it is absent or empty.
func QueryBool(r *http.Request, name string) (value, set bool, err error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return false, false, nil
	}
	value, err = strconv.ParseBool(raw)
	if err != nil {
		return false, false, rejection("invalid query", name)
	}
	return value, true, nil
}
