package params

import (
	"encoding/json"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
)

// The list shows a default schedule for a parameter without settings and enabling stores one, in two queries that must agree.
func TestUnconfiguredScheduleShowsWhatEnablingStores(t *testing.T) {
	e := newEnv(t)
	enabled, untouched := e.addTank("Enabled"), e.addTank("Untouched")
	var keys []*string
	rows, err := e.db.R.QueryContext(t.Context(), `SELECT preset_key FROM parameter_definitions WHERE preset_key IS NOT NULL`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, &key)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	stamp := "2026-10-24T12:00:00Z"
	if _, err := db.Q(e.db.W).EnablePresetParameters(t.Context(), dbgen.EnablePresetParametersParams{TankID: enabled, ScheduleSince: stamp, UpdatedAt: stamp, PresetKeys: keys}); err != nil {
		t.Fatal(err)
	}

	stored, shown := e.tankParams(enabled), e.tankParams(untouched)
	if len(stored.Parameters) != 10 {
		t.Fatalf("%d parameters enabled, want the 10 presets", len(stored.Parameters))
	}
	for _, p := range stored.Parameters {
		if p.PresetKey == nil {
			continue
		}
		if got := shown.find(t, p.ParameterID); !got.Schedule.Equal(p.Schedule) {
			shownJSON, _ := json.Marshal(got.Schedule)
			storedJSON, _ := json.Marshal(p.Schedule)
			t.Errorf("%s: unconfigured shows %s, enabling stores %s", p.Name, shownJSON, storedJSON)
		}
	}
}
