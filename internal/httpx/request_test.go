package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireUser(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	var apiErr *APIError
	if _, err := RequireUser(r); !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized {
		t.Fatalf("anonymous RequireUser error = %v, want a 401", err)
	}
	signedIn := r.WithContext(WithUser(r.Context(), User{ID: 7, Username: "jayce"}))
	if u, err := RequireUser(signedIn); err != nil || u.ID != 7 {
		t.Errorf("signed-in RequireUser = %+v, %v; want user 7", u, err)
	}
}

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
