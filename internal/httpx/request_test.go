package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIncludeArchived(t *testing.T) {
	cases := map[string]struct {
		want bool
		bad  bool
	}{
		"/":                          {},
		"/?include_archived=":        {},
		"/?include_archived=true":    {want: true},
		"/?include_archived=1":       {want: true},
		"/?include_archived=false":   {},
		"/?include_archived=0":       {},
		"/?include_archived=perhaps": {bad: true},
	}
	for target, tc := range cases {
		got, err := IncludeArchived(httptest.NewRequest(http.MethodGet, target, nil))
		var apiErr *APIError
		switch {
		case tc.bad && (!errors.As(err, &apiErr) || apiErr.Status != http.StatusUnprocessableEntity || apiErr.Fields["include_archived"] == ""):
			t.Errorf("IncludeArchived(%q) error = %v, want a 422 on include_archived", target, err)
		case !tc.bad && (err != nil || got != tc.want):
			t.Errorf("IncludeArchived(%q) = %v, %v; want %v", target, got, err, tc.want)
		}
	}
}
