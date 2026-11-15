package app

import (
	"maps"
	"net/http"
	"strconv"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

// expectCodes fails unless the error body names exactly the given fields with the given codes.
func expectCodes(t *testing.T, b testutil.ErrorBody, want map[string]string) {
	t.Helper()
	if !maps.Equal(b.Error.Fields, want) {
		t.Errorf("fields = %v, want %v", b.Error.Fields, want)
	}
}

// TestFieldKeysMatchWhatTheBrowserReads pins the field names, including the dotted shapes, through the whole stack.
func TestFieldKeysMatchWhatTheBrowserReads(t *testing.T) {
	a, admin := adminAndApp(t)
	tank := mustCreateTank(t, admin)
	id := strconv.FormatInt(tank.ID, 10)
	testutil.Exec(t, a.db, `INSERT INTO tank_parameters (tank_id, parameter_id, enabled, sort_order)
		SELECT ?, id, 1, sort_order FROM parameter_definitions WHERE preset_key = 'ph'`, tank.ID)
	ph := testutil.Count(t, a.db, `SELECT id FROM parameter_definitions WHERE preset_key = 'ph'`)
	phPath := "/api/v1/tanks/" + id + "/parameters/" + strconv.Itoa(ph)

	cases := []struct {
		name         string
		method, path string
		body         any
		status       int
		code         string
		want         map[string]string
	}{
		{"settings units", http.MethodPatch, "/api/v1/settings", map[string]any{"units": map[string]string{"temperature": "kelvin"}}, 422, "validation",
			map[string]string{"units.temperature": "invalid"}},
		{"dashboard cards", http.MethodPut, "/api/v1/tanks/" + id + "/dashboard", map[string]any{"cards": []map[string]any{{"card_type": "tests_due", "size": "huge"}, {"card_type": "tests_due"}, {"card_type": "tests_due"}}}, 422, "validation",
			map[string]string{"cards.0.size": "invalid", "cards.1": "invalid", "cards.2": "invalid"}},
		{"schedule", http.MethodPut, phPath, map[string]any{"schedule": map[string]any{"kind": "every_n_days", "interval_days": 3}}, 422, "validation",
			map[string]string{"schedule": "required"}},
		{"schedule kind", http.MethodPut, phPath, map[string]any{"schedule": map[string]any{"interval_days": 3}}, 422, "validation",
			map[string]string{"schedule.kind": "required"}},
		{"results", http.MethodPost, "/api/v1/tanks/" + id + "/results", map[string]any{"results": []map[string]any{{"parameter_id": ph}}}, 422, "validation",
			map[string]string{"results.0.value": "required"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			expectCodes(t, testutil.ExpectError(t, admin.do(c.method, c.path, c.body), c.status, c.code), c.want)
		})
	}
}
