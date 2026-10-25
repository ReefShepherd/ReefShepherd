package tanks

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
	"github.com/reefshepherd/reefshepherd/internal/units"
)

func TestCreateGetListAndUpdate(t *testing.T) {
	e := newEnv(t)

	// A bare create fills in the defaults and enables nothing.
	a := e.create(map[string]any{"name": "  Display  "})
	if a.Name != "Display" || a.TankType != "reef" || a.Volume != nil || a.StartedOn != nil || a.Note != "" || a.ArchivedAt != nil || a.SortOrder != 0 || a.VolumeUnit != "l" {
		t.Errorf("defaults = %+v", a)
	}
	if a.CreatedAt == "" || a.UpdatedAt != a.CreatedAt || !strings.HasSuffix(a.CreatedAt, "Z") {
		t.Errorf("timestamps = %q %q", a.CreatedAt, a.UpdatedAt)
	}
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM tank_parameters`); n != 0 {
		t.Errorf("a create without presets enabled %d parameters", n)
	}

	b := e.create(map[string]any{"name": "Frag tank", "tank_type": "frag", "volume": 80.5, "started_on": "2025-03-04", "note": "Hang-on frag rack"})
	if b.ID == a.ID || b.SortOrder != 1 || b.TankType != "frag" || b.Volume == nil || *b.Volume != 80.5 || b.StartedOn == nil || *b.StartedOn != "2025-03-04" || b.Note != "Hang-on frag rack" {
		t.Errorf("created = %+v", b)
	}

	rec := e.do(asMember, http.MethodGet, path(b.ID, ""), nil)
	testutil.ExpectStatus(e.t, rec, http.StatusOK)
	if got := testutil.Decode[Tank](t, rec); got.ID != b.ID || got.Name != b.Name || *got.Volume != *b.Volume || got.CreatedAt != b.CreatedAt {
		t.Errorf("get = %+v, want %+v", got, b)
	}

	rec = e.do(asMember, http.MethodGet, "/api/v1/tanks", nil)
	testutil.ExpectStatus(e.t, rec, http.StatusOK)
	list := testutil.Decode[[]Tank](t, rec)
	if len(list) != 2 || list[0].ID != a.ID || list[1].ID != b.ID {
		t.Errorf("list = %+v", list)
	}

	// A patch changes only what it names, and a member may edit.
	rec = e.do(asMember, http.MethodPatch, path(b.ID, ""), map[string]any{"name": "Frag rack", "sort_order": -1})
	testutil.ExpectStatus(e.t, rec, http.StatusOK)
	got := testutil.Decode[Tank](t, rec)
	if got.Name != "Frag rack" || got.SortOrder != -1 || got.TankType != "frag" || *got.Volume != 80.5 || *got.StartedOn != "2025-03-04" || got.Note != "Hang-on frag rack" {
		t.Errorf("patched = %+v", got)
	}
	list = testutil.Decode[[]Tank](t, e.do(asMember, http.MethodGet, "/api/v1/tanks", nil))
	if list[0].ID != b.ID || list[1].ID != a.ID {
		t.Errorf("sort_order did not reorder the list: %+v", list)
	}

	// Null clears the volume and the start date; an empty patch changes nothing.
	rec = e.admin(http.MethodPatch, path(b.ID, ""), map[string]any{"volume": nil, "started_on": nil, "note": "", "tank_type": "quarantine"})
	testutil.ExpectStatus(e.t, rec, http.StatusOK)
	got = testutil.Decode[Tank](t, rec)
	if got.Volume != nil || got.StartedOn != nil || got.Note != "" || got.TankType != "quarantine" {
		t.Errorf("cleared = %+v", got)
	}
	rec = e.admin(http.MethodPatch, path(b.ID, ""), map[string]any{})
	testutil.ExpectStatus(e.t, rec, http.StatusOK)
	if again := testutil.Decode[Tank](t, rec); again.Name != got.Name || again.TankType != got.TankType || again.SortOrder != got.SortOrder {
		t.Errorf("empty patch = %+v, want %+v", again, got)
	}
}

func TestRequireFindsTanksArchivedOrNot(t *testing.T) {
	e := newEnv(t)
	tank := e.create(map[string]any{"name": "Display"})
	archived := e.create(map[string]any{"name": "Old"})
	testutil.ExpectStatus(e.t, e.admin(http.MethodPost, path(archived.ID, "/archive"), nil), http.StatusOK)
	for _, id := range []int64{tank.ID, archived.ID} {
		if err := Require(t.Context(), e.db.R, id); err != nil {
			t.Errorf("Require(%d) = %v, want nil", id, err)
		}
	}
	if err := Require(t.Context(), e.db.R, 9999); !errors.Is(err, ErrNotFound) {
		t.Errorf("Require(9999) = %v, want ErrNotFound", err)
	}
}

func TestPresetParametersStartTheirScheduleWhenTheTankIsCreated(t *testing.T) {
	e := newEnv(t)
	svc := NewService(Deps{DB: e.db, Settings: e.settings})
	svc.now = func() time.Time { return time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC) }
	tank, err := svc.Create(t.Context(), CreateInput{Name: "Reef", Presets: []string{"ph", "alkalinity"}})
	if err != nil {
		t.Fatal(err)
	}
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM tank_parameters WHERE tank_id = ? AND schedule_since = '2026-10-24T12:00:00Z' AND updated_at = '2026-10-24T12:00:00Z'`, tank.ID); n != 2 {
		t.Errorf("%d preset rows start at the creation time, want 2", n)
	}
}

func TestCreateEnablesPresetsWithOneInsert(t *testing.T) {
	e := newEnv(t)
	other := e.create(map[string]any{"name": "Other", "presets": []string{"ph"}})
	tank := e.create(map[string]any{"name": "Reef", "presets": []string{"alkalinity", "ph", "ammonia", "ph"}})

	type row struct {
		key                string
		enabled            int
		low, high          *float64
		kind               string
		interval           *int
		weeks              int
		dueTime            string
		sortOrder          int
		weekdays, monthDay *int
		anchorDate         *string
	}
	rows, err := e.db.R.QueryContext(t.Context(), `
		SELECT d.preset_key, tp.enabled, tp.ideal_low, tp.ideal_high, tp.schedule_kind, tp.interval_days,
		       tp.interval_weeks, tp.due_time, tp.sort_order, tp.weekdays, tp.month_day, tp.anchor_date
		FROM tank_parameters tp JOIN parameter_definitions d ON d.id = tp.parameter_id
		WHERE tp.tank_id = ? ORDER BY d.sort_order`, tank.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.key, &r.enabled, &r.low, &r.high, &r.kind, &r.interval, &r.weeks, &r.dueTime, &r.sortOrder, &r.weekdays, &r.monthDay, &r.anchorDate); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].key != "ph" || got[1].key != "alkalinity" || got[2].key != "ammonia" {
		t.Fatalf("enabled = %+v, want ph, alkalinity, ammonia once each", got)
	}
	ph, alk, amm := got[0], got[1], got[2]
	if ph.enabled != 1 || ph.low == nil || *ph.low != 7.9 || ph.high == nil || *ph.high != 8.4 || ph.kind != "after_last" || ph.interval == nil || *ph.interval != 7 || ph.sortOrder != 30 {
		t.Errorf("ph = %+v", ph)
	}
	if alk.low == nil || *alk.low != 7.0 || *alk.high != 11.0 || alk.sortOrder != 40 {
		t.Errorf("alkalinity = %+v", alk)
	}
	if amm.low != nil || amm.high == nil || *amm.high != 0.10 || amm.interval == nil || *amm.interval != 30 {
		t.Errorf("ammonia has an open lower bound and a 30-day interval: %+v", amm)
	}
	for _, r := range got {
		if r.weeks != 1 || r.dueTime != "09:00" || r.weekdays != nil || r.monthDay != nil || r.anchorDate != nil {
			t.Errorf("%s: unused schedule columns were set: %+v", r.key, r)
		}
	}
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM tank_parameters WHERE tank_id = ?`, other.ID); n != 1 {
		t.Errorf("the other tank has %d parameters, want 1", n)
	}

	// An empty list enables nothing.
	empty := e.create(map[string]any{"name": "Bare", "presets": []string{}})
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM tank_parameters WHERE tank_id = ?`, empty.ID); n != 0 {
		t.Errorf("empty presets enabled %d parameters", n)
	}
}

func TestPresetWithoutIntervalHasNoSchedule(t *testing.T) {
	e := newEnv(t)
	testutil.Exec(e.t, e.db, `UPDATE parameter_definitions SET default_test_interval_days = NULL WHERE preset_key = 'ph'`)
	testutil.Exec(e.t, e.db, `UPDATE parameter_definitions SET default_test_interval_days = 0 WHERE preset_key = 'calcium'`)
	tank := e.create(map[string]any{"name": "Reef", "presets": []string{"ph", "calcium"}})
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM tank_parameters WHERE tank_id = ? AND schedule_kind = 'none' AND interval_days IS NULL`, tank.ID); n != 2 {
		t.Errorf("%d of 2 parameters without a usable interval have no schedule", n)
	}
}

func TestUnknownPresetRejectsTheWholeCreate(t *testing.T) {
	e := newEnv(t)
	testutil.Exec(e.t, e.db, `UPDATE parameter_definitions SET archived_at = '2026-01-01T00:00:00Z' WHERE preset_key = 'nitrite'`)
	testutil.Exec(e.t, e.db, `INSERT INTO parameter_definitions (name) VALUES ('My custom')`)

	rec := e.admin(http.MethodPost, "/api/v1/tanks", map[string]any{"name": "Reef", "presets": []string{"ph", "bogus", "nitrite", "BOGUS2"}})
	fields := testutil.ExpectError(e.t, rec, http.StatusUnprocessableEntity, "validation").Error.Fields
	if msg := fields["presets"]; !strings.Contains(msg, "bogus") || !strings.Contains(msg, "nitrite") || !strings.Contains(msg, "BOGUS2") || strings.Contains(msg, "ph") {
		t.Errorf("presets message = %q", msg)
	}
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM tanks`); n != 0 {
		t.Errorf("a rejected create left %d tanks", n)
	}
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM tank_parameters`); n != 0 {
		t.Errorf("a rejected create left %d tank parameters", n)
	}
}

func TestFailedPresetInsertRollsBackTheCreate(t *testing.T) {
	e := newEnv(t)
	testutil.Exec(e.t, e.db, `CREATE TRIGGER fail_preset_insert BEFORE INSERT ON tank_parameters BEGIN SELECT RAISE(ABORT, 'preset insert failed'); END`)

	rec := e.admin(http.MethodPost, "/api/v1/tanks", map[string]any{"name": "Reef", "presets": []string{"ph"}})
	testutil.ExpectError(e.t, rec, http.StatusInternalServerError, "internal")
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM tanks`); n != 0 {
		t.Errorf("a create whose presets failed left %d tanks", n)
	}

	// Without presets nothing touches tank_parameters, so the same trigger does not stop the create.
	e.create(map[string]any{"name": "Reef"})
}

func TestArchivedParametersAreNotEnabledByPreset(t *testing.T) {
	e := newEnv(t)
	testutil.Exec(e.t, e.db, `UPDATE parameter_definitions SET archived_at = '2026-01-01T00:00:00Z' WHERE preset_key = 'ph'`)
	ph := "ph"
	rows, err := db.Q(e.db.W).EnablePresetParameters(t.Context(), dbgen.EnablePresetParametersParams{
		TankID: e.create(map[string]any{"name": "Reef"}).ID, UpdatedAt: "2026-01-02T00:00:00Z", PresetKeys: []*string{&ph},
	})
	if err != nil || rows != 0 {
		t.Errorf("enabling an archived preset = %d rows, %v; want 0", rows, err)
	}
}

func TestVolumeRoundTripsThroughLiters(t *testing.T) {
	e := newEnv(t)
	if err := e.settings.SetUnit(t.Context(), units.Volume, "us_gal"); err != nil {
		t.Fatal(err)
	}
	tank := e.create(map[string]any{"name": "Display", "volume": 100})
	if tank.Volume == nil || *tank.Volume != 100 || tank.VolumeUnit != "us_gal" {
		t.Fatalf("created = %+v", tank)
	}
	var liters float64
	if err := e.db.R.QueryRowContext(t.Context(), `SELECT volume_liters FROM tanks WHERE id = ?`, tank.ID).Scan(&liters); err != nil {
		t.Fatal(err)
	}
	if !near(liters, 378.5411784, 1e-9) {
		t.Errorf("stored %v liters, want 378.5411784", liters)
	}

	// Changing the display unit changes the output, never the stored row.
	for unit, want := range map[string]float64{"l": 378.5, "uk_gal": 83.3} {
		if err := e.settings.SetUnit(t.Context(), units.Volume, unitOf(unit)); err != nil {
			t.Fatal(err)
		}
		got := testutil.Decode[Tank](t, e.do(asMember, http.MethodGet, path(tank.ID, ""), nil))
		if got.Volume == nil || *got.Volume != want || got.VolumeUnit != unit {
			t.Errorf("%s: volume = %v %q, want %v", unit, got.Volume, got.VolumeUnit, want)
		}
	}
	var after float64
	if err := e.db.R.QueryRowContext(t.Context(), `SELECT volume_liters FROM tanks WHERE id = ?`, tank.ID).Scan(&after); err != nil || after != liters {
		t.Errorf("stored liters changed from %v to %v (%v)", liters, after, err)
	}

	// A patch is read in the current display unit too.
	if err := e.settings.SetUnit(t.Context(), units.Volume, "uk_gal"); err != nil {
		t.Fatal(err)
	}
	rec := e.admin(http.MethodPatch, path(tank.ID, ""), map[string]any{"volume": 50})
	testutil.ExpectStatus(e.t, rec, http.StatusOK)
	if err := e.db.R.QueryRowContext(t.Context(), `SELECT volume_liters FROM tanks WHERE id = ?`, tank.ID).Scan(&after); err != nil || !near(after, 227.3045, 1e-9) {
		t.Errorf("50 UK gal stored as %v liters (%v)", after, err)
	}
}

func TestVolumeIsValidatedInTheDisplayUnit(t *testing.T) {
	e := newEnv(t)
	for _, v := range []float64{0, -5, 1e9} {
		fields := testutil.ExpectError(e.t, e.admin(http.MethodPost, "/api/v1/tanks", map[string]any{"name": "x", "volume": v}), http.StatusUnprocessableEntity, "validation").Error.Fields
		if fields["volume"] == "" {
			t.Errorf("volume %v accepted: %v", v, fields)
		}
	}
	// 1,000,000 liters is the limit, so the same number is fine in liters and too big in gallons.
	testutil.ExpectStatus(e.t, e.admin(http.MethodPost, "/api/v1/tanks", map[string]any{"name": "x", "volume": 999999}), http.StatusCreated)
	if err := e.settings.SetUnit(t.Context(), units.Volume, "us_gal"); err != nil {
		t.Fatal(err)
	}
	testutil.ExpectError(e.t, e.admin(http.MethodPost, "/api/v1/tanks", map[string]any{"name": "x", "volume": 999999}), http.StatusUnprocessableEntity, "validation")
}

func TestValidationReportsEveryField(t *testing.T) {
	e := newEnv(t)
	rec := e.admin(http.MethodPost, "/api/v1/tanks", map[string]any{
		"name": "   ", "tank_type": "pond", "volume": -1, "started_on": "2026-02-30", "note": strings.Repeat("n", 10001),
	})
	fields := testutil.ExpectError(e.t, rec, http.StatusUnprocessableEntity, "validation").Error.Fields
	for _, f := range []string{"name", "tank_type", "volume", "started_on", "note"} {
		if fields[f] == "" {
			t.Errorf("no message for %s: %v", f, fields)
		}
	}
	if !strings.Contains(fields["tank_type"], "reef, fowlr, frag, quarantine, other") {
		t.Errorf("tank_type message = %q", fields["tank_type"])
	}

	testutil.ExpectError(e.t, e.admin(http.MethodPost, "/api/v1/tanks", map[string]any{"name": strings.Repeat("é", 101)}), http.StatusUnprocessableEntity, "validation")
	testutil.ExpectStatus(e.t, e.admin(http.MethodPost, "/api/v1/tanks", map[string]any{"name": strings.Repeat("é", 100)}), http.StatusCreated)
	for _, bad := range []string{"2026-1-2", "tomorrow", "2026-13-01", "02/03/2026"} {
		fields := testutil.ExpectError(e.t, e.admin(http.MethodPost, "/api/v1/tanks", map[string]any{"name": "x", "started_on": bad}), http.StatusUnprocessableEntity, "validation").Error.Fields
		if fields["started_on"] == "" {
			t.Errorf("started_on %q accepted", bad)
		}
	}
	// A blank date means none.
	if got := e.create(map[string]any{"name": "x", "started_on": " "}); got.StartedOn != nil {
		t.Errorf("blank started_on = %v", *got.StartedOn)
	}

	testutil.ExpectError(e.t, e.admin(http.MethodPost, "/api/v1/tanks", map[string]any{"name": "x", "colour": "blue"}), http.StatusUnprocessableEntity, "validation")
	testutil.ExpectError(e.t, e.admin(http.MethodPost, "/api/v1/tanks", map[string]any{"name": "x", "volume": "big"}), http.StatusUnprocessableEntity, "validation")
	testutil.ExpectError(e.t, e.admin(http.MethodPost, "/api/v1/tanks", map[string]any{"name": "x", "presets": "ph"}), http.StatusUnprocessableEntity, "validation")

	r := testutil.DoRaw(e.t, e.mux, http.MethodPost, "/api/v1/tanks", "text/plain", "name=x", testutil.AsUser(1, asAdmin))
	testutil.ExpectError(e.t, r, http.StatusUnsupportedMediaType, "unsupported_media_type")
}

func TestRejectedPatchChangesNothing(t *testing.T) {
	e := newEnv(t)
	tank := e.create(map[string]any{"name": "Keep", "volume": 60, "note": "original"})
	rec := e.admin(http.MethodPatch, path(tank.ID, ""), map[string]any{"name": "Changed", "note": "changed", "tank_type": "pond", "volume": 0, "started_on": "nope"})
	fields := testutil.ExpectError(e.t, rec, http.StatusUnprocessableEntity, "validation").Error.Fields
	for _, f := range []string{"tank_type", "volume", "started_on"} {
		if fields[f] == "" {
			t.Errorf("no message for %s: %v", f, fields)
		}
	}
	if fields["name"] != "" || fields["note"] != "" {
		t.Errorf("valid fields were reported: %v", fields)
	}
	got := testutil.Decode[Tank](t, e.admin(http.MethodGet, path(tank.ID, ""), nil))
	if got.Name != "Keep" || got.Note != "original" || *got.Volume != 60 {
		t.Errorf("a rejected patch was partly applied: %+v", got)
	}
	testutil.ExpectError(e.t, e.admin(http.MethodPatch, path(tank.ID, ""), map[string]any{"name": ""}), http.StatusUnprocessableEntity, "validation")
	testutil.ExpectError(e.t, e.admin(http.MethodPatch, path(tank.ID, ""), map[string]any{"sort_order": "first"}), http.StatusUnprocessableEntity, "validation")
	testutil.ExpectError(e.t, e.admin(http.MethodPatch, path(tank.ID, ""), map[string]any{"mystery": 1}), http.StatusUnprocessableEntity, "validation")
}

func TestPatchRejectsNullForFieldsThatCannotBeCleared(t *testing.T) {
	e := newEnv(t)
	tank := e.create(map[string]any{"name": "Keep", "tank_type": "frag", "note": "original"})
	for _, field := range []string{"name", "tank_type", "note", "sort_order"} {
		rec := e.admin(http.MethodPatch, path(tank.ID, ""), map[string]any{field: nil})
		fields := testutil.ExpectError(e.t, rec, http.StatusUnprocessableEntity, "validation").Error.Fields
		if fields[field] != "must not be null" || len(fields) != 1 {
			t.Errorf("null %s: fields = %v", field, fields)
		}
	}
	rec := e.admin(http.MethodPatch, path(tank.ID, ""), map[string]any{"name": nil, "note": nil, "tank_type": "reef"})
	if fields := testutil.ExpectError(e.t, rec, http.StatusUnprocessableEntity, "validation").Error.Fields; len(fields) != 2 || fields["name"] == "" || fields["note"] == "" {
		t.Errorf("two nulls: fields = %v", fields)
	}
	got := testutil.Decode[Tank](t, e.admin(http.MethodGet, path(tank.ID, ""), nil))
	if got.Name != "Keep" || got.TankType != "frag" || got.Note != "original" {
		t.Errorf("a rejected null patch changed the tank: %+v", got)
	}

	// Volume and started_on stay clearable, and an empty patch still changes nothing.
	testutil.ExpectStatus(e.t, e.admin(http.MethodPatch, path(tank.ID, ""), map[string]any{"volume": nil, "started_on": nil}), http.StatusOK)
	testutil.ExpectStatus(e.t, e.admin(http.MethodPatch, path(tank.ID, ""), map[string]any{}), http.StatusOK)
}

func TestArchiveAndUnarchive(t *testing.T) {
	e := newEnv(t)
	a := e.create(map[string]any{"name": "A"})
	b := e.create(map[string]any{"name": "B"})

	rec := e.do(asMember, http.MethodPost, path(a.ID, "/archive"), nil)
	testutil.ExpectStatus(e.t, rec, http.StatusOK)
	archived := testutil.Decode[Tank](t, rec)
	if archived.ArchivedAt == nil || !strings.HasSuffix(*archived.ArchivedAt, "Z") {
		t.Fatalf("archived = %+v", archived)
	}

	list := testutil.Decode[[]Tank](t, e.do(asMember, http.MethodGet, "/api/v1/tanks", nil))
	if len(list) != 1 || list[0].ID != b.ID {
		t.Errorf("default list = %+v, want only B", list)
	}
	list = testutil.Decode[[]Tank](t, e.do(asMember, http.MethodGet, "/api/v1/tanks?include_archived=true", nil))
	if len(list) != 2 || list[0].ID != b.ID || list[1].ID != a.ID || list[1].ArchivedAt == nil {
		t.Errorf("list with archived = %+v, want B then archived A", list)
	}
	list = testutil.Decode[[]Tank](t, e.do(asMember, http.MethodGet, "/api/v1/tanks?include_archived=false", nil))
	if len(list) != 1 {
		t.Errorf("include_archived=false listed %d tanks", len(list))
	}
	testutil.ExpectError(e.t, e.do(asMember, http.MethodGet, "/api/v1/tanks?include_archived=maybe", nil), http.StatusUnprocessableEntity, "validation")

	// An archived tank can still be read and edited, and archiving again keeps the first time.
	testutil.Exec(e.t, e.db, `UPDATE tanks SET archived_at = '2020-01-01T00:00:00Z' WHERE id = ?`, a.ID)
	testutil.ExpectStatus(e.t, e.do(asMember, http.MethodGet, path(a.ID, ""), nil), http.StatusOK)
	testutil.ExpectStatus(e.t, e.do(asMember, http.MethodPatch, path(a.ID, ""), map[string]any{"note": "kept"}), http.StatusOK)
	again := testutil.Decode[Tank](t, e.do(asMember, http.MethodPost, path(a.ID, "/archive"), nil))
	if again.ArchivedAt == nil || *again.ArchivedAt != "2020-01-01T00:00:00Z" || again.Note != "kept" {
		t.Errorf("second archive = %+v, first at 2020-01-01", again)
	}

	rec = e.do(asMember, http.MethodPost, path(a.ID, "/unarchive"), nil)
	testutil.ExpectStatus(e.t, rec, http.StatusOK)
	if back := testutil.Decode[Tank](t, rec); back.ArchivedAt != nil {
		t.Errorf("unarchived = %+v", back)
	}
	if list = testutil.Decode[[]Tank](t, e.do(asMember, http.MethodGet, "/api/v1/tanks", nil)); len(list) != 2 {
		t.Errorf("list after unarchive = %+v", list)
	}
	// Unarchiving an active tank is fine.
	testutil.ExpectStatus(e.t, e.do(asMember, http.MethodPost, path(a.ID, "/unarchive"), nil), http.StatusOK)
	testutil.ExpectError(e.t, testutil.DoRaw(e.t, e.mux, http.MethodPost, path(a.ID, "/archive"), "text/plain", "x", testutil.AsUser(1, asAdmin)), http.StatusUnsupportedMediaType, "unsupported_media_type")
}

func TestDeleteNeedsTheTankNameAndCascades(t *testing.T) {
	e := newEnv(t)
	name := "Reef & Co. ö 100%"
	tank := e.create(map[string]any{"name": name, "presets": []string{"ph", "alkalinity"}})
	keep := e.create(map[string]any{"name": "Keep", "presets": []string{"ph"}})

	testutil.Exec(e.t, e.db, `INSERT INTO users (id, username, role) VALUES (1, 'owner', 'admin')`)
	for _, id := range []int64{tank.ID, keep.ID} {
		testutil.Exec(e.t, e.db, `INSERT INTO test_results (tank_id, parameter_id, measured_at, value) VALUES (?, (SELECT id FROM parameter_definitions WHERE preset_key = 'ph'), '2026-01-01T00:00:00Z', 8.1)`, id)
		testutil.Exec(e.t, e.db, `INSERT INTO dashboard_cards (user_id, tank_id, card_type, position) VALUES (1, ?, 'tests_due', 0)`, id)
	}
	testutil.Exec(e.t, e.db, `INSERT INTO user_preferences (user_id, default_tank_id) VALUES (1, ?)`, tank.ID)

	del := func(role string, id int64, query string) *httptest.ResponseRecorder {
		return e.do(role, http.MethodDelete, path(id, query), nil)
	}
	for _, q := range []string{"", "?confirm=", "?confirm=reef", "?confirm=" + url.QueryEscape(name+" "), "?confirm=" + url.QueryEscape(strings.ToLower(name)), "?confirm=Keep"} {
		fields := testutil.ExpectError(e.t, del(asAdmin, tank.ID, q), http.StatusUnprocessableEntity, "validation").Error.Fields
		if fields["confirm"] == "" {
			t.Errorf("%q: no confirm message: %v", q, fields)
		}
	}
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM tanks WHERE id = ?`, tank.ID); n != 1 {
		t.Fatal("a delete without the right confirmation removed the tank")
	}

	testutil.ExpectStatus(e.t, del(asAdmin, tank.ID, "?confirm="+url.QueryEscape(name)), http.StatusNoContent)

	for _, table := range []string{"tank_parameters", "test_results", "dashboard_cards"} {
		if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM `+table+` WHERE tank_id = ?`, tank.ID); n != 0 {
			t.Errorf("%s: %d rows survived the delete", table, n)
		}
		if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM `+table+` WHERE tank_id = ?`, keep.ID); n == 0 {
			t.Errorf("%s: the other tank lost its rows", table)
		}
	}
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM user_preferences WHERE user_id = 1 AND default_tank_id IS NULL`); n != 1 {
		t.Error("the default tank was not cleared")
	}
	testutil.ExpectError(e.t, e.admin(http.MethodGet, path(tank.ID, ""), nil), http.StatusNotFound, "not_found")
	testutil.ExpectError(e.t, del(asAdmin, tank.ID, "?confirm="+url.QueryEscape(name)), http.StatusNotFound, "not_found")
	if list := testutil.Decode[[]Tank](t, e.admin(http.MethodGet, "/api/v1/tanks", nil)); len(list) != 1 || list[0].ID != keep.ID {
		t.Errorf("list after delete = %+v", list)
	}
}

func TestOnlyAdminsDelete(t *testing.T) {
	e := newEnv(t)
	tank := e.create(map[string]any{"name": "Reef"})
	testutil.ExpectError(e.t, e.do(asMember, http.MethodDelete, path(tank.ID, "?confirm=Reef"), nil), http.StatusForbidden, "forbidden")
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM tanks`); n != 1 {
		t.Error("a member deleted a tank")
	}
	// A request that carries no user at all never reaches the handler's data.
	testutil.ExpectError(e.t, e.do("", http.MethodDelete, path(tank.ID, "?confirm=Reef"), nil), http.StatusUnauthorized, "unauthorized")
	testutil.ExpectStatus(e.t, e.admin(http.MethodDelete, path(tank.ID, "?confirm=Reef"), nil), http.StatusNoContent)
}

func TestMissingTanksAreNotFound(t *testing.T) {
	e := newEnv(t)
	for _, id := range []string{"999", "0", "-1", "abc", "1.5"} {
		base := "/api/v1/tanks/" + id
		for _, c := range []struct{ method, suffix string }{
			{http.MethodGet, ""}, {http.MethodPatch, ""}, {http.MethodDelete, "?confirm=x"},
			{http.MethodPost, "/archive"}, {http.MethodPost, "/unarchive"},
		} {
			var body any
			if c.method == http.MethodPatch {
				body = map[string]any{"note": "x"}
			}
			rec := e.admin(c.method, base+c.suffix, body)
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s %s%s = %d, want 404; body=%s", c.method, base, c.suffix, rec.Code, rec.Body)
			}
		}
	}
	// The method is checked before the id, so a wrong verb is a 405 from the router.
	if rec := e.admin(http.MethodPut, "/api/v1/tanks/1", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT = %d", rec.Code)
	}
}
