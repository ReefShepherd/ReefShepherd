package settings

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/units"
)

func newStore(t *testing.T) (*Store, *db.DB) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "reef.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(t.Context(), filepath.Join(t.TempDir(), "backups")); err != nil {
		t.Fatal(err)
	}
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
	if n := rowCount(t, d); n != 0 {
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

func rowCount(t *testing.T, d *db.DB) int {
	t.Helper()
	var n int
	if err := d.R.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM settings`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
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

func TestSettersRejectInvalidValues(t *testing.T) {
	s, d := newStore(t)
	ctx := t.Context()

	cases := map[string]error{
		"blank instance name":       s.SetInstanceName(ctx, "   "),
		"unknown timezone":          s.SetTimezone(ctx, "Mars/Olympus"),
		"empty timezone":            s.SetTimezone(ctx, ""),
		"Local timezone":            s.SetTimezone(ctx, "Local"),
		"unknown auth mode":         s.SetAuthMode(ctx, "ldap"),
		"zero user id":              s.SetAuthDisabledUserID(ctx, 0),
		"negative user id":          s.SetAuthDisabledUserID(ctx, -3),
		"hour without leading zero": s.SetReminderDefaultTime(ctx, "9:00"),
		"hour out of range":         s.SetReminderDefaultTime(ctx, "24:00"),
		"minute out of range":       s.SetReminderDefaultTime(ctx, "09:60"),
		"time with seconds":         s.SetReminderDefaultTime(ctx, "09:00:00"),
		"fahrenheit for salinity":   s.SetUnit(ctx, units.Salinity, units.Fahrenheit),
		"unknown unit":              s.SetUnit(ctx, units.Volume, units.Unit("gallon")),
		"fixed family":              s.SetUnit(ctx, units.Fixed, units.None),
		"unknown family":            s.SetUnit(ctx, units.Family("pressure"), units.PPM),
	}
	for name, err := range cases {
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	if _, err := s.GetUnit(ctx, units.Fixed); !errors.Is(err, ErrInvalid) {
		t.Errorf("GetUnit(fixed) err = %v, want ErrInvalid", err)
	}
	if n := rowCount(t, d); n != 0 {
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
