package httpx

import (
	"net/http"
	"strconv"
)

// PathID reads the path value name as a positive integer id, reporting false when it is anything else.
func PathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id < 1 {
		return 0, false
	}
	return id, true
}
