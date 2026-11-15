package httpx

import "net/http"

// Page is a list response that also carries the cursor for the following page, which is null on the last one.
type Page[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

// List writes a collection as {"items": [...]}; a nil slice is sent as an empty array.
func List[T any](w http.ResponseWriter, status int, items []T) {
	WriteJSON(w, status, struct {
		Items []T `json:"items"`
	}{Items: nonNil(items)})
}

// ListPage writes one page of a collection as {"items": [...], "next_cursor": ...}.
func ListPage[T any](w http.ResponseWriter, status int, page Page[T]) {
	page.Items = nonNil(page.Items)
	WriteJSON(w, status, page)
}

func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}
