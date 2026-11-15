package settings

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/units"
)

func TestDisplayFamiliesExcludeFixed(t *testing.T) {
	got := DisplayFamilies()
	if len(got) != 5 {
		t.Fatalf("families = %v", got)
	}
	for _, f := range got {
		if f == units.Fixed {
			t.Error("the fixed family has no display unit")
		}
	}
}

func TestValuesFillsInDefaults(t *testing.T) {
	s, _ := newStore(t)
	v, err := s.Values(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if v.InstanceName != DefaultInstanceName || v.Timezone != DefaultTimezone || v.ReminderDefaultTime != DefaultReminderTime || v.AuthMode != config.AuthModePassword {
		t.Errorf("values = %+v", v)
	}
	if v.Units[units.Temperature] != units.Celsius || v.Units[units.Volume] != units.Liter || len(v.Units) != 5 {
		t.Errorf("units = %v", v.Units)
	}
}

// fieldCodes returns the field problems err carries, keyed by API field name.
func fieldCodes(err error) map[string]string {
	var f httpx.FieldErrors
	f.AddFrom(err)
	return httpx.Validation(f).Fields
}

func TestValidateNamesEveryBadFieldWithItsCode(t *testing.T) {
	blank, tz, hhmm := " ", "Mars/Base", "7:5"
	err := Changes{
		InstanceName: &blank, Timezone: &tz, ReminderDefaultTime: &hhmm,
		Units: map[units.Family]units.Unit{units.Temperature: "kelvin", units.Fixed: "x", units.Volume: units.USGallon},
	}.Validate()
	want := map[string]string{
		"instance_name": "required", "timezone": "invalid", "reminder_default_time": "invalid",
		"units.temperature": "invalid", "units.fixed": "invalid",
	}
	got := fieldCodes(err)
	if len(got) != len(want) {
		t.Fatalf("codes = %v, want %v", got, want)
	}
	for field, code := range want {
		if got[field] != code {
			t.Errorf("codes = %v, want %v", got, want)
		}
	}
	if err := (Changes{}).Validate(); err != nil {
		t.Errorf("an empty change set is valid, got %v", err)
	}
}

func TestApplyWritesNothingWhenAnyFieldIsInvalid(t *testing.T) {
	s, d := newStore(t)
	name, tz := "Fine", "Mars/Base"
	err := s.Apply(t.Context(), Changes{InstanceName: &name, Timezone: &tz})
	if got := fieldCodes(err); len(got) != 1 || got["timezone"] != "invalid" {
		t.Fatalf("err=%v, want only timezone: invalid", err)
	}
	if _, ok := storedValue(t, d, "instance_name"); ok {
		t.Error("the valid field was written next to an invalid one")
	}

	tz = "Europe/Berlin"
	hhmm := "06:45"
	if err := s.Apply(t.Context(), Changes{InstanceName: &name, Timezone: &tz, ReminderDefaultTime: &hhmm, Units: map[units.Family]units.Unit{units.Phosphate: units.PPB}}); err != nil {
		t.Fatal(err)
	}
	v, _ := s.Values(t.Context())
	if v.InstanceName != "Fine" || v.Timezone != tz || v.ReminderDefaultTime != hhmm || v.Units[units.Phosphate] != units.PPB {
		t.Errorf("values = %+v", v)
	}
}

func TestSwitchAuthMode(t *testing.T) {
	s, d := newStore(t)
	ctx := t.Context()
	exec := func(q string) {
		t.Helper()
		if _, err := d.W.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}

	// Nobody to act as, nobody who can sign in.
	refused := func(name string, err error) {
		t.Helper()
		if got := fieldCodes(err); !errors.Is(err, ErrInvalid) || len(got) != 1 || got["auth_mode"] != "invalid" {
			t.Errorf("%s: %v, want an ErrInvalid for auth_mode with code invalid", name, err)
		}
	}
	refused("disable with no admin", s.SwitchAuthMode(ctx, config.AuthModeDisabled))
	refused("enable with no admin", s.SwitchAuthMode(ctx, config.AuthModePassword))
	refused("unknown mode", s.SwitchAuthMode(ctx, "oidc"))

	exec(`INSERT INTO users (id, username, role, disabled) VALUES (1, 'off', 'admin', 1)`)
	exec(`INSERT INTO users (id, username, role) VALUES (2, 'member', 'member')`)
	exec(`INSERT INTO users (id, username, role, password_hash) VALUES (3, 'first', 'admin', NULL)`)
	exec(`INSERT INTO users (id, username, role, password_hash) VALUES (4, 'second', 'admin', 'x')`)

	if err := s.SwitchAuthMode(ctx, config.AuthModeDisabled); err != nil {
		t.Fatal(err)
	}
	if id, _ := s.GetAuthDisabledUserID(ctx); id != 3 {
		t.Errorf("acting user = %d, want the first enabled admin (3)", id)
	}
	if mode, _ := s.GetAuthMode(ctx); mode != config.AuthModeDisabled {
		t.Errorf("mode = %q", mode)
	}

	// Enabling needs an enabled admin with a password; admin 4 qualifies.
	if err := s.SwitchAuthMode(ctx, config.AuthModePassword); err != nil {
		t.Fatal(err)
	}
	if id, _ := s.GetAuthDisabledUserID(ctx); id != 0 {
		t.Errorf("acting user %d was not cleared", id)
	}
	exec(`UPDATE users SET password_hash = NULL WHERE id = 4`)
	exec(`UPDATE users SET password_hash = 'x', disabled = 1 WHERE id = 3`)
	refused("only a disabled admin has a password", s.SwitchAuthMode(ctx, config.AuthModePassword))
}

func TestWithTxCommitsAndRollsBackTogether(t *testing.T) {
	s, d := newStore(t)
	// A store that ignored the transaction would wait for the writer connection the transaction holds, so bound the wait.
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	rollback := errors.New("rollback")
	err := d.InTx(ctx, func(tx *sql.Tx) error {
		inTx := s.WithTx(tx)
		if err := inTx.SetInstanceName(ctx, "Inside"); err != nil {
			return err
		}
		// Reads inside the transaction see the uncommitted write.
		if got, _ := inTx.GetInstanceName(ctx); got != "Inside" {
			t.Errorf("read inside tx = %q", got)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if got, _ := s.GetInstanceName(t.Context()); got != DefaultInstanceName {
		t.Errorf("rolled back write is visible: %q", got)
	}

	err = d.InTx(ctx, func(tx *sql.Tx) error {
		return s.WithTx(tx).SetInstanceName(ctx, "Committed")
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetInstanceName(t.Context()); got != "Committed" {
		t.Errorf("committed write = %q", got)
	}
}
