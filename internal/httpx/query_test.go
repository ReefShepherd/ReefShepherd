package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func assertInvalidQuery(t *testing.T, err error, field string) {
	t.Helper()
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnprocessableEntity || apiErr.Fields[field] != string(CodeInvalid) {
		t.Errorf("error = %v, want a 422 with code invalid on %s", err, field)
	}
}

func TestQueryID(t *testing.T) {
	cases := []struct {
		target  string
		want    int64
		wantSet bool
		bad     bool
	}{
		{target: "/"},
		{target: "/?tank_id="},
		{target: "/?tank_id=7", want: 7, wantSet: true},
		{target: "/?tank_id=9223372036854775807", want: 9223372036854775807, wantSet: true},
		{target: "/?tank_id=0", bad: true},
		{target: "/?tank_id=-3", bad: true},
		{target: "/?tank_id=abc", bad: true},
		{target: "/?tank_id=1.5", bad: true},
		{target: "/?tank_id=99999999999999999999", bad: true},
		{target: "/?other=5"},
	}
	for _, tc := range cases {
		id, set, err := QueryID(httptest.NewRequest(http.MethodGet, tc.target, nil), "tank_id")
		if tc.bad {
			assertInvalidQuery(t, err, "tank_id")
			if id != 0 || set {
				t.Errorf("QueryID(%q) = %d, %v on error; want zero values", tc.target, id, set)
			}
			continue
		}
		if err != nil || id != tc.want || set != tc.wantSet {
			t.Errorf("QueryID(%q) = %d, %v, %v; want %d, %v", tc.target, id, set, err, tc.want, tc.wantSet)
		}
	}
}

func TestQueryBool(t *testing.T) {
	cases := []struct {
		target  string
		want    bool
		wantSet bool
		bad     bool
	}{
		{target: "/"},
		{target: "/?unread="},
		{target: "/?unread=true", want: true, wantSet: true},
		{target: "/?unread=1", want: true, wantSet: true},
		{target: "/?unread=false", wantSet: true},
		{target: "/?unread=0", wantSet: true},
		{target: "/?unread=perhaps", bad: true},
		{target: "/?unread=yes", bad: true},
	}
	for _, tc := range cases {
		v, set, err := QueryBool(httptest.NewRequest(http.MethodGet, tc.target, nil), "unread")
		if tc.bad {
			assertInvalidQuery(t, err, "unread")
			if v || set {
				t.Errorf("QueryBool(%q) = %v, %v on error; want zero values", tc.target, v, set)
			}
			continue
		}
		if err != nil || v != tc.want || set != tc.wantSet {
			t.Errorf("QueryBool(%q) = %v, %v, %v; want %v, %v", tc.target, v, set, err, tc.want, tc.wantSet)
		}
	}
}
