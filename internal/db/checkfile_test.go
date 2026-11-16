package db

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// backupOf writes a backup of a migrated database holding the given number of extra tanks, so the file spans several pages.
func backupOf(t *testing.T, tanks int) string {
	t.Helper()
	d := migrated(t)
	for i := range tanks {
		exec(t, d.W, `INSERT INTO tanks (name, volume_liters, note) VALUES (?, 100, ?)`, fmt.Sprintf("tank %d", i), strings.Repeat("x", 200))
	}
	path := filepath.Join(t.TempDir(), "backup.db")
	if err := d.BackupTo(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	return path
}

func execOnFile(t *testing.T, path, query string) {
	t.Helper()
	c, err := sql.Open("sqlite", fileURI(path))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.ExecContext(t.Context(), query); err != nil {
		t.Fatal(err)
	}
}

func TestCheckFileAcceptsABackupAndReportsItsVersion(t *testing.T) {
	path := backupOf(t, 3)
	want, err := migrated(t).Version(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if v, err := CheckFile(t.Context(), path); err != nil || v != want {
		t.Errorf("CheckFile = %d, %v; want %d", v, err, want)
	}
}

func TestCheckFileAcceptsAnOlderSchema(t *testing.T) {
	path := backupOf(t, 0)
	execOnFile(t, path, `DELETE FROM goose_db_version WHERE version_id = 2`)
	if v, err := CheckFile(t.Context(), path); err != nil || v != 1 {
		t.Errorf("CheckFile = %d, %v; want 1", v, err)
	}
}

func TestCheckFileRefusesANewerSchema(t *testing.T) {
	path := backupOf(t, 0)
	execOnFile(t, path, `INSERT INTO goose_db_version (version_id, is_applied) VALUES (99, 1)`)
	if _, err := CheckFile(t.Context(), path); !errors.Is(err, ErrSchemaTooNew) {
		t.Errorf("err = %v, want ErrSchemaTooNew", err)
	}
}

func TestCheckFileRefusesFilesThatAreNotABackup(t *testing.T) {
	dir := t.TempDir()
	garbage := filepath.Join(dir, "garbage.db")
	if err := os.WriteFile(garbage, []byte(strings.Repeat("not a database ", 500)), 0o600); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(dir, "foreign.db")
	execOnFile(t, foreign, `CREATE TABLE t (x)`)

	for name, path := range map[string]string{"garbage": garbage, "foreign sqlite": foreign, "missing": filepath.Join(dir, "none.db"), "directory": dir} {
		if _, err := CheckFile(t.Context(), path); err == nil {
			t.Errorf("%s: CheckFile accepted it", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "none.db")); err == nil {
		t.Error("checking a missing file created it")
	}
}

func TestCheckFileRunsTheIntegrityCheck(t *testing.T) {
	path := backupOf(t, 400)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const pageSize = 4096
	last := (len(raw)/pageSize - 1) * pageSize
	// A page with an impossible type byte still lets the schema be read, but fails the structural check.
	raw[last] = 0x7f
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = CheckFile(t.Context(), path)
	if err == nil || !strings.Contains(err.Error(), "integrity check failed") {
		t.Errorf("err = %v, want an integrity check failure", err)
	}
}

func TestCheckpointFoldsTheLogIntoTheDatabaseFile(t *testing.T) {
	d := migrated(t)
	var path string
	if err := d.W.QueryRowContext(t.Context(), `SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&path); err != nil {
		t.Fatal(err)
	}
	exec(t, d.W, `INSERT INTO users (id, username) VALUES (1, 'owner')`)
	if info, err := os.Stat(path + "-wal"); err != nil || info.Size() == 0 {
		t.Fatalf("expected a non-empty log before the checkpoint: %v", err)
	}
	if err := Checkpoint(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path + "-wal"); err != nil || info.Size() != 0 {
		t.Errorf("log after checkpoint: %v, %v; want an empty file", info, err)
	}
	if err := Checkpoint(t.Context(), filepath.Join(t.TempDir(), "missing-dir", "x.db")); err == nil {
		t.Error("checkpointing an impossible path succeeded")
	}
}
