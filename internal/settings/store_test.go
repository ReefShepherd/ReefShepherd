package settings

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
	"github.com/reefshepherd/reefshepherd/internal/units"
)

func newStore(t *testing.T) (*Store, *db.DB) {
	t.Helper()
	d := testutil.OpenDB(t)
	return NewStore(d), d
}

func storedValue(t *testing.T, d *db.DB, key string) (string, bool) {
	t.Helper()
	var v string
	err := d.R.QueryRowContext(t.Context(), `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	return v, err == nil
}

func TestDefaultsWhenNothingIsStored(t *testing.T) {
	s, d := newStore(t)
	ctx := t.Context()

	if v, err := s.GetSetupCompleted(ctx); err != nil || v {
		t.Errorf("setup completed = %v, %v; want false", v, err)
	}
	if v, err := s.GetInstanceName(ctx); err != nil || v != "ReefShepherd" {
		t.Errorf("instance name = %q, %v", v, err)
	}
	if v, err := s.GetTimezone(ctx); err != nil || v != "UTC" {
		t.Errorf("timezone = %q, %v", v, err)
	}
	if v, err := s.GetAuthMode(ctx); err != nil || v != "password" {
		t.Errorf("auth mode = %q, %v", v, err)
	}
	if v, err := s.GetAuthDisabledUserID(ctx); err != nil || v != 0 {
		t.Errorf("auth disabled user = %d, %v", v, err)
	}
	if v, err := s.GetReminderDefaultTime(ctx); err != nil || v != "09:00" {
		t.Errorf("reminder time = %q, %v", v, err)
	}
	wantUnits := map[units.Family]units.Unit{
		units.Temperature: units.Celsius,
		units.Salinity:    units.SG,
		units.Alkalinity:  units.DKH,
		units.Phosphate:   units.PPM,
		units.Volume:      units.Liter,
	}
	for family, want := range wantUnits {
		if got, err := s.GetUnit(ctx, family); err != nil || got != want {
			t.Errorf("unit for %s = %q, %v; want %q", family, got, err, want)
		}
	}
	if n := testutil.Count(t, d, `SELECT COUNT(*) FROM settings`); n != 0 {
		t.Errorf("reading defaults wrote %d rows", n)
	}
}

func TestSalinityDefaultsToSGAndPPTCanBeChosen(t *testing.T) {
	s, d := newStore(t)
	ctx := t.Context()

	if got, err := s.GetUnit(ctx, units.Salinity); err != nil || got != units.SG {
		t.Errorf("default salinity unit = %q, %v; want %q", got, err, units.SG)
	}
	if _, ok := storedValue(t, d, "unit_salinity"); ok {
		t.Error("reading the default stored a salinity unit")
	}
	if err := s.SetUnit(ctx, units.Salinity, units.PPT); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetUnit(ctx, units.Salinity); err != nil || got != units.PPT {
		t.Errorf("chosen salinity unit = %q, %v; want %q", got, err, units.PPT)
	}
}

func TestSettersRoundTrip(t *testing.T) {
	s, d := newStore(t)
	ctx := t.Context()

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.SetSetupCompleted(ctx, true))
	must(s.SetInstanceName(ctx, "  Garage reef "))
	must(s.SetTimezone(ctx, "Europe/Berlin"))
	must(s.SetAuthMode(ctx, "disabled"))
	must(s.SetAuthDisabledUserID(ctx, 7))
	must(s.SetReminderDefaultTime(ctx, "18:30"))
	must(s.SetUnit(ctx, units.Temperature, units.Fahrenheit))
	must(s.SetUnit(ctx, units.Salinity, units.PPT))
	must(s.SetUnit(ctx, units.Alkalinity, units.MeqPerL))
	must(s.SetUnit(ctx, units.Phosphate, units.PPB))
	must(s.SetUnit(ctx, units.Volume, units.USGallon))

	if v, _ := s.GetSetupCompleted(ctx); !v {
		t.Error("setup completed not stored")
	}
	if v, _ := s.GetInstanceName(ctx); v != "Garage reef" {
		t.Errorf("instance name = %q, want it trimmed", v)
	}
	if v, _ := s.GetTimezone(ctx); v != "Europe/Berlin" {
		t.Errorf("timezone = %q", v)
	}
	if v, _ := s.GetAuthMode(ctx); v != "disabled" {
		t.Errorf("auth mode = %q", v)
	}
	if v, _ := s.GetAuthDisabledUserID(ctx); v != 7 {
		t.Errorf("auth disabled user = %d", v)
	}
	if v, _ := s.GetReminderDefaultTime(ctx); v != "18:30" {
		t.Errorf("reminder time = %q", v)
	}

	// The unit keys are part of the documented settings contract.
	for key, want := range map[string]string{
		"unit_temperature": "f", "unit_salinity": "ppt", "unit_alkalinity": "meq_l",
		"unit_phosphate": "ppb", "unit_volume": "us_gal", "setup_completed": "true",
	} {
		if got, ok := storedValue(t, d, key); !ok || got != want {
			t.Errorf("stored %s = %q (found %v), want %q", key, got, ok, want)
		}
	}
	if got, err := s.GetUnit(ctx, units.Temperature); err != nil || got != units.Fahrenheit {
		t.Errorf("temperature unit = %q, %v", got, err)
	}

	must(s.SetSetupCompleted(ctx, false))
	if v, _ := s.GetSetupCompleted(ctx); v {
		t.Error("setup completed still true after being set to false")
	}

	must(s.ClearAuthDisabledUserID(ctx))
	if v, err := s.GetAuthDisabledUserID(ctx); err != nil || v != 0 {
		t.Errorf("auth disabled user after clear = %d, %v", v, err)
	}
	must(s.ClearAuthDisabledUserID(ctx))
}

func TestCodesAreOnTheAPIsClosedList(t *testing.T) {
	if httpx.Code(codeRequired) != httpx.CodeRequired || httpx.Code(codeInvalid) != httpx.CodeInvalid || httpx.Code(codeOutOfRange) != httpx.CodeOutOfRange {
		t.Errorf("local codes %q, %q and %q differ from the API's", codeRequired, codeInvalid, codeOutOfRange)
	}
}

func TestSettersRejectInvalidValues(t *testing.T) {
	s, d := newStore(t)
	ctx := t.Context()

	cases := []struct {
		name        string
		err         error
		field, code string
	}{
		{"blank instance name", s.SetInstanceName(ctx, "   "), "instance_name", "required"},
		{"unknown timezone", s.SetTimezone(ctx, "Mars/Olympus"), "timezone", "invalid"},
		{"empty timezone", s.SetTimezone(ctx, ""), "timezone", "invalid"},
		{"Local timezone", s.SetTimezone(ctx, "Local"), "timezone", "invalid"},
		{"unknown auth mode", s.SetAuthMode(ctx, "ldap"), "auth_mode", "invalid"},
		{"zero user id", s.SetAuthDisabledUserID(ctx, 0), "auth_disabled_user_id", "invalid"},
		{"negative user id", s.SetAuthDisabledUserID(ctx, -3), "auth_disabled_user_id", "invalid"},
		{"hour without leading zero", s.SetReminderDefaultTime(ctx, "9:00"), "reminder_default_time", "invalid"},
		{"hour out of range", s.SetReminderDefaultTime(ctx, "24:00"), "reminder_default_time", "invalid"},
		{"minute out of range", s.SetReminderDefaultTime(ctx, "09:60"), "reminder_default_time", "invalid"},
		{"time with seconds", s.SetReminderDefaultTime(ctx, "09:00:00"), "reminder_default_time", "invalid"},
		{"fahrenheit for salinity", s.SetUnit(ctx, units.Salinity, units.Fahrenheit), "units.salinity", "invalid"},
		{"unknown unit", s.SetUnit(ctx, units.Volume, units.Unit("gallon")), "units.volume", "invalid"},
		{"fixed family", s.SetUnit(ctx, units.Fixed, units.None), "units.fixed", "invalid"},
		{"unknown family", s.SetUnit(ctx, units.Family("pressure"), units.PPM), "units.pressure", "invalid"},
	}
	for _, c := range cases {
		if !errors.Is(c.err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", c.name, c.err)
		}
		if got := fieldCodes(c.err); len(got) != 1 || got[c.field] != c.code {
			t.Errorf("%s: field errors = %v, want %s: %s", c.name, got, c.field, c.code)
		}
	}
	if _, err := s.GetUnit(ctx, units.Fixed); !errors.Is(err, ErrInvalid) {
		t.Errorf("GetUnit(fixed) err = %v, want ErrInvalid", err)
	}
	if n := testutil.Count(t, d, `SELECT COUNT(*) FROM settings`); n != 0 {
		t.Errorf("rejected values left %d rows behind", n)
	}
}

func TestCorruptStoredValuesAreReported(t *testing.T) {
	s, d := newStore(t)
	ctx := t.Context()
	for _, kv := range [][2]string{{"setup_completed", "maybe"}, {"auth_disabled_user_id", "seven"}, {"auth_mode", "ldap"}, {"unit_temperature", "furlongs"}, {"unit_salinity", "f"}} {
		if _, err := d.W.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?)`, kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.GetSetupCompleted(ctx); err == nil {
		t.Error("a non-boolean setup_completed was read without error")
	}
	if _, err := s.GetAuthDisabledUserID(ctx); err == nil {
		t.Error("a non-numeric auth_disabled_user_id was read without error")
	}
	if _, err := s.GetAuthMode(ctx); err == nil {
		t.Error("an unknown stored auth_mode was read without error")
	}
	for _, family := range []units.Family{units.Temperature, units.Salinity} {
		if _, err := s.GetUnit(ctx, family); err == nil {
			t.Errorf("an invalid stored unit for %s was read without error", family)
		}
	}
	if _, err := d.W.ExecContext(ctx, `UPDATE settings SET value = '' WHERE key = 'setup_completed'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSetupCompleted(ctx); err == nil {
		t.Error("an empty setup_completed was treated as unset")
	}
}

func storedUpdatedAt(t *testing.T, d *db.DB, key string) string {
	t.Helper()
	var at string
	if err := d.R.QueryRowContext(t.Context(), `SELECT updated_at FROM settings WHERE key = ?`, key).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at
}

func TestSettingsRecordTheClockAsTheirUpdateTime(t *testing.T) {
	s, d := newStore(t)
	ctx := t.Context()
	first := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	s.now = func() time.Time { return first }

	if err := s.SetInstanceName(ctx, "One"); err != nil {
		t.Fatal(err)
	}
	if got := storedUpdatedAt(t, d, "instance_name"); got != "2026-03-04T05:06:07Z" {
		t.Errorf("updated_at after insert = %q, want the clock", got)
	}

	// An overwrite moves the time, and so does a write through a transaction's store.
	s.now = func() time.Time { return first.Add(48 * time.Hour) }
	err := d.InTx(ctx, func(tx *sql.Tx) error { return s.WithTx(tx).SetInstanceName(ctx, "Two") })
	if err != nil {
		t.Fatal(err)
	}
	if got := storedUpdatedAt(t, d, "instance_name"); got != "2026-03-06T05:06:07Z" {
		t.Errorf("updated_at after overwrite = %q, want the later clock", got)
	}
}
