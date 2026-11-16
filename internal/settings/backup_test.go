package settings

import (
	"errors"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

func TestBackupSettingsDefaultsAndRoundTrip(t *testing.T) {
	s, d := newStore(t)
	ctx := t.Context()

	if v, err := s.GetBackupEnabled(ctx); err != nil || !v {
		t.Errorf("backup enabled = %v, %v; want true", v, err)
	}
	if v, err := s.GetBackupTime(ctx); err != nil || v != "03:30" {
		t.Errorf("backup time = %q, %v; want 03:30", v, err)
	}
	if v, err := s.GetBackupKeep(ctx); err != nil || v != 14 {
		t.Errorf("backup keep = %d, %v; want 14", v, err)
	}
	if _, ok := storedValue(t, d, "backup_keep"); ok {
		t.Error("reading the defaults stored a value")
	}

	for _, err := range []error{s.SetBackupEnabled(ctx, false), s.SetBackupTime(ctx, "23:59"), s.SetBackupKeep(ctx, 365)} {
		if err != nil {
			t.Fatal(err)
		}
	}
	for key, want := range map[string]string{"backup_enabled": "false", "backup_time": "23:59", "backup_keep": "365"} {
		if got, ok := storedValue(t, d, key); !ok || got != want {
			t.Errorf("stored %s = %q (found %v), want %q", key, got, ok, want)
		}
	}
	if v, _ := s.GetBackupEnabled(ctx); v {
		t.Error("backup enabled still true")
	}
	if v, _ := s.GetBackupTime(ctx); v != "23:59" {
		t.Errorf("backup time = %q", v)
	}
	if v, _ := s.GetBackupKeep(ctx); v != 365 {
		t.Errorf("backup keep = %d", v)
	}
	if err := s.SetBackupKeep(ctx, 1); err != nil {
		t.Errorf("keep 1 rejected: %v", err)
	}
}

func TestBackupSettersRejectInvalidValues(t *testing.T) {
	s, d := newStore(t)
	ctx := t.Context()
	cases := []struct {
		name        string
		err         error
		field, code string
	}{
		{"hour without leading zero", s.SetBackupTime(ctx, "3:30"), "backup_time", "invalid"},
		{"hour out of range", s.SetBackupTime(ctx, "24:00"), "backup_time", "invalid"},
		{"minute out of range", s.SetBackupTime(ctx, "03:60"), "backup_time", "invalid"},
		{"time with seconds", s.SetBackupTime(ctx, "03:30:00"), "backup_time", "invalid"},
		{"empty time", s.SetBackupTime(ctx, ""), "backup_time", "invalid"},
		{"keep zero", s.SetBackupKeep(ctx, 0), "backup_keep", "out_of_range"},
		{"keep negative", s.SetBackupKeep(ctx, -1), "backup_keep", "out_of_range"},
		{"keep too many", s.SetBackupKeep(ctx, 366), "backup_keep", "out_of_range"},
	}
	for _, c := range cases {
		if !errors.Is(c.err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", c.name, c.err)
		}
		if got := fieldCodes(c.err); len(got) != 1 || got[c.field] != c.code {
			t.Errorf("%s: field errors = %v, want %s: %s", c.name, got, c.field, c.code)
		}
	}
	if n := testutil.Count(t, d, `SELECT COUNT(*) FROM settings`); n != 0 {
		t.Errorf("rejected values left %d rows behind", n)
	}
}

func TestCorruptStoredBackupValuesAreReported(t *testing.T) {
	s, d := newStore(t)
	ctx := t.Context()
	if _, err := d.W.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ('backup_enabled', 'maybe'), ('backup_keep', 'many')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetBackupEnabled(ctx); err == nil {
		t.Error("a non-boolean backup_enabled was read without error")
	}
	if _, err := s.GetBackupKeep(ctx); err == nil {
		t.Error("a non-numeric backup_keep was read without error")
	}
	for _, v := range []string{"0", "366"} {
		if _, err := d.W.ExecContext(ctx, `UPDATE settings SET value = ? WHERE key = 'backup_keep'`, v); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetBackupKeep(ctx); err == nil {
			t.Errorf("an out-of-range stored backup_keep %s was read without error", v)
		}
	}
}

func TestBackupSettingsInValuesAndChanges(t *testing.T) {
	s, d := newStore(t)
	ctx := t.Context()
	v, err := s.GetValues(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !v.BackupEnabled || v.BackupTime != DefaultBackupTime || v.BackupKeep != DefaultBackupKeep {
		t.Errorf("default values = %+v", v)
	}

	off, badTime, badKeep := false, "7pm", 0
	err = Changes{BackupEnabled: &off, BackupTime: &badTime, BackupKeep: &badKeep}.Validate()
	if got := fieldCodes(err); len(got) != 2 || got["backup_time"] != "invalid" || got["backup_keep"] != "out_of_range" {
		t.Errorf("codes = %v", got)
	}
	if err := s.Apply(ctx, Changes{BackupEnabled: &off, BackupTime: &badTime}); err == nil {
		t.Fatal("an invalid change was applied")
	}
	if _, ok := storedValue(t, d, "backup_enabled"); ok {
		t.Error("the valid backup field was written next to an invalid one")
	}

	goodTime, keep := "01:15", 30
	if err := s.Apply(ctx, Changes{BackupEnabled: &off, BackupTime: &goodTime, BackupKeep: &keep}); err != nil {
		t.Fatal(err)
	}
	v, _ = s.GetValues(ctx)
	if v.BackupEnabled || v.BackupTime != "01:15" || v.BackupKeep != 30 {
		t.Errorf("values = %+v", v)
	}
}
