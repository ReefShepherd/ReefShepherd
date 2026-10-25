package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPathID(t *testing.T) {
	cases := map[string]struct {
		id int64
		ok bool
	}{
		"7":                   {7, true},
		"1":                   {1, true},
		"9223372036854775807": {9223372036854775807, true},
		"0":                   {},
		"-3":                  {},
		"abc":                 {},
		"1.5":                 {},
		"9223372036854775808": {},
		"%20":                 {},
	}
	for raw, want := range cases {
		var gotID int64
		var gotOK bool
		mux := http.NewServeMux()
		mux.HandleFunc("GET /things/{id}", func(_ http.ResponseWriter, r *http.Request) {
			gotID, gotOK = PathID(r, "id")
		})
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/things/"+raw, nil))
		if gotID != want.id || gotOK != want.ok {
			t.Errorf("PathID(%q) = %d, %v; want %d, %v", raw, gotID, gotOK, want.id, want.ok)
		}
	}
}
