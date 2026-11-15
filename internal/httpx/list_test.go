package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListWritesAnItemsEnvelope(t *testing.T) {
	for _, c := range []struct {
		name  string
		items []int
		want  string
	}{
		{"nil", nil, `{"items":[]}`},
		{"empty", []int{}, `{"items":[]}`},
		{"values", []int{1, 2}, `{"items":[1,2]}`},
	} {
		rec := httptest.NewRecorder()
		List(rec, http.StatusCreated, c.items)
		if rec.Code != http.StatusCreated || rec.Body.String() != c.want+"\n" {
			t.Errorf("%s: %d %q, want %s", c.name, rec.Code, rec.Body, c.want)
		}
	}
}

func TestListPageCarriesTheCursor(t *testing.T) {
	next := "abc"
	for _, c := range []struct {
		name string
		page Page[string]
		want string
	}{
		{"last page", Page[string]{}, `{"items":[],"next_cursor":null}`},
		{"more", Page[string]{Items: []string{"a"}, NextCursor: &next}, `{"items":["a"],"next_cursor":"abc"}`},
	} {
		rec := httptest.NewRecorder()
		ListPage(rec, http.StatusOK, c.page)
		if rec.Body.String() != c.want+"\n" {
			t.Errorf("%s: %q, want %s", c.name, rec.Body, c.want)
		}
	}
}
