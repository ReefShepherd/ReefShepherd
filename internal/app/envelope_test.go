package app

import (
	"bytes"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

// bodyKeys decodes a JSON object response into its raw members.
func bodyKeys(t *testing.T, rec *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	return testutil.Decode[map[string]json.RawMessage](t, rec)
}

// expectKeys fails unless the response is an object with exactly the wanted keys.
func expectKeys(t *testing.T, name string, rec *httptest.ResponseRecorder, want ...string) map[string]json.RawMessage {
	t.Helper()
	body := bodyKeys(t, rec)
	got := slices.Sorted(maps.Keys(body))
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("%s keys = %v, want %v (body %s)", name, got, want, rec.Body)
	}
	return body
}

// expectItems fails unless the response has an items array holding n entries.
func expectItems(t *testing.T, name string, body map[string]json.RawMessage, n int) {
	t.Helper()
	raw := bytes.TrimSpace(body["items"])
	var items []json.RawMessage
	if len(raw) == 0 || raw[0] != '[' || json.Unmarshal(raw, &items) != nil || len(items) != n {
		t.Errorf("%s items = %s, want an array of %d", name, raw, n)
	}
}

// TestCollectionsAreItemsEnvelopes pins the list shape of every collection endpoint and the keys of the objects that carry metadata beside a list.
func TestCollectionsAreItemsEnvelopes(t *testing.T) {
	a, admin := adminAndApp(t)

	// Empty lists are arrays, never null.
	expectItems(t, "empty tanks", expectKeys(t, "empty tanks", admin.do(http.MethodGet, "/api/v1/tanks", nil), "items"), 0)

	tank := mustCreateTank(t, admin)
	id := strconv.FormatInt(tank.ID, 10)
	testutil.Exec(t, a.db, `INSERT INTO tank_parameters (tank_id, parameter_id, enabled, sort_order)
		SELECT ?, id, 1, sort_order FROM parameter_definitions WHERE preset_key = 'ph'`, tank.ID)
	ph := strconv.Itoa(testutil.Count(t, a.db, `SELECT id FROM parameter_definitions WHERE preset_key = 'ph'`))
	tankPath := "/api/v1/tanks/" + id

	expectItems(t, "empty results", expectKeys(t, "empty results", admin.do(http.MethodGet, tankPath+"/results", nil), "items", "next_cursor"), 0)

	created := admin.do(http.MethodPost, tankPath+"/results", map[string]any{"results": []map[string]any{{"parameter_id": json.Number(ph), "value": 8.1}}})
	testutil.ExpectStatus(t, created, http.StatusCreated)
	expectItems(t, "POST results", expectKeys(t, "POST results", created, "items"), 1)

	page := expectKeys(t, "GET results", admin.do(http.MethodGet, tankPath+"/results", nil), "items", "next_cursor")
	expectItems(t, "GET results", page, 1)
	if string(page["next_cursor"]) != "null" {
		t.Errorf("last page next_cursor = %s, want null", page["next_cursor"])
	}

	for _, c := range []struct {
		name, path string
		n          int
	}{
		{"GET tanks", "/api/v1/tanks", 1},
		{"GET users", "/api/v1/users", 1},
		{"GET sessions", "/api/v1/me/sessions", 1},
		{"GET units", "/api/v1/units", 6},
		{"GET parameters", "/api/v1/parameters", 10},
	} {
		expectItems(t, c.name, expectKeys(t, c.name, admin.do(http.MethodGet, c.path, nil), "items"), c.n)
	}

	// Objects with metadata beside a list keep their own keys.
	expectKeys(t, "tank parameters", admin.do(http.MethodGet, tankPath+"/parameters", nil), "tank_id", "timezone", "parameters")
	for _, c := range []struct{ name, path, key string }{
		{"series", tankPath + "/parameters/" + ph + "/series", "points"},
		{"dashboard", tankPath + "/dashboard", "cards"},
		{"auth config", "/api/v1/auth/config", "methods"},
	} {
		body := bodyKeys(t, admin.do(http.MethodGet, c.path, nil))
		if _, ok := body[c.key]; !ok {
			t.Errorf("%s has no %q key (keys %v)", c.name, c.key, slices.Sorted(maps.Keys(body)))
		}
		if _, ok := body["items"]; ok {
			t.Errorf("%s was re-keyed to items", c.name)
		}
	}
}
