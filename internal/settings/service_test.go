package settings

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/config"
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

func TestValidateNamesEveryBadField(t *testing.T) {
	blank, tz, hhmm := " ", "Mars/Base", "7:5"
	got := Changes{
		InstanceName: &blank, Timezone: &tz, ReminderDefaultTime: &hhmm,
		Units: map[units.Family]units.Unit{units.Temperature: "kelvin", units.Fixed: "x", units.Volume: units.USGallon},
	}.Validate()
	for _, field := range []string{"instance_name", "timezone", "reminder_default_time", "units.temperature", "units.fixed"} {
		if got[field] == "" {
			t.Errorf("no message for %s: %v", field, got)
		}
	}
	if _, bad := got["units.volume"]; bad || len(got) != 5 {
		t.Errorf("messages = %v", got)
	}
	if len(Changes{}.Validate()) != 0 {
		t.Error("an empty change set is valid")
	}
}

func TestApplyWritesNothingWhenAnyFieldIsInvalid(t *testing.T) {
	s, d := newStore(t)
	name, tz := "Fine", "Mars/Base"
	rejected, err := s.Apply(t.Context(), Changes{InstanceName: &name, Timezone: &tz})
	if err != nil || rejected["timezone"] == "" {
		t.Fatalf("rejected=%v err=%v", rejected, err)
	}
	if _, ok := storedValue(t, d, "instance_name"); ok {
		t.Error("the valid field was written next to an invalid one")
	}

	tz = "Europe/Berlin"
	hhmm := "06:45"
	rejected, err = s.Apply(t.Context(), Changes{InstanceName: &name, Timezone: &tz, ReminderDefaultTime: &hhmm, Units: map[units.Family]units.Unit{units.Phosphate: units.PPB}})
	if err != nil || rejected == nil || len(rejected) != 0 {
		t.Fatalf("rejected=%v err=%v; want a non-nil empty map", rejected, err)
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
	if err := s.SwitchAuthMode(ctx, config.AuthModeDisabled); !errors.Is(err, ErrInvalid) {
		t.Errorf("disable with no admin: %v", err)
	}
	if err := s.SwitchAuthMode(ctx, config.AuthModePassword); !errors.Is(err, ErrInvalid) {
		t.Errorf("enable with no admin: %v", err)
	}
	if err := s.SwitchAuthMode(ctx, "oidc"); !errors.Is(err, ErrInvalid) {
		t.Errorf("unknown mode: %v", err)
	}

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
	if err := s.SwitchAuthMode(ctx, config.AuthModePassword); !errors.Is(err, ErrInvalid) {
		t.Errorf("only a disabled admin has a password: %v", err)
	}
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
