package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/app"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

// restoreFixture is a data directory holding a database with one tank and a separate backup file holding another.
type restoreFixture struct {
	t      *testing.T
	dir    string
	dbPath string
	backup string
	listen string
}

func newRestoreFixture(t *testing.T) *restoreFixture {
	t.Helper()
	f := &restoreFixture{t: t, dir: t.TempDir()}
	f.dbPath = filepath.Join(f.dir, app.DBFileName)

	current := testutil.OpenDBAt(t, f.dbPath)
	testutil.Exec(t, current, `INSERT INTO tanks (name) VALUES ('current tank')`)
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}

	source := testutil.OpenDBAt(t, filepath.Join(t.TempDir(), "source.db"))
	testutil.Exec(t, source, `INSERT INTO tanks (name) VALUES ('backup tank')`)
	f.backup = filepath.Join(t.TempDir(), "backup.db")
	if err := source.BackupTo(t.Context(), f.backup); err != nil {
		t.Fatal(err)
	}

	// A port nothing listens on, so the server check passes unless a test binds it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f.listen = ln.Addr().String()
	_ = ln.Close()
	return f
}

func (f *restoreFixture) restore(file string, extra ...string) (code int, stdout, stderr string) {
	f.t.Helper()
	var out, errBuf bytes.Buffer
	args := append([]string{"restore", file, "--data-dir", f.dir, "--listen", f.listen}, extra...)
	code = run(context.Background(), args, noEnv, &out, &errBuf)
	return code, out.String(), errBuf.String()
}

func tankNames(t *testing.T, path string) []string {
	t.Helper()
	c, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	rows, err := c.QueryContext(t.Context(), `SELECT name FROM tanks ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	return names
}

func fileHash(t *testing.T, path string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

func (f *restoreFixture) prerestoreFiles() []string {
	f.t.Helper()
	files, err := filepath.Glob(filepath.Join(f.dir, "backups", "reefshepherd-prerestore-*.db"))
	if err != nil {
		f.t.Fatal(err)
	}
	return files
}

func TestRestoreReplacesTheDatabaseAndKeepsTheOldOne(t *testing.T) {
	f := newRestoreFixture(t)
	wantHash := fileHash(t, f.backup)

	code, out, errOut := f.restore(f.backup)
	if code != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}

	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(f.dbPath + suffix); err == nil {
			t.Errorf("stale %s file beside the restored database", suffix)
		}
	}
	if got := tankNames(t, f.dbPath); len(got) != 1 || got[0] != "backup tank" {
		t.Errorf("restored tanks = %v, want [backup tank]", got)
	}
	// The restored database opens as a normal one and keeps its rows.
	d, err := db.Open(f.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if n := testutil.Count(t, d, `SELECT COUNT(*) FROM tanks WHERE name = 'backup tank'`); n != 1 {
		t.Errorf("tanks named 'backup tank' = %d, want 1", n)
	}
	aside := f.prerestoreFiles()
	if len(aside) != 1 {
		t.Fatalf("prerestore files = %v, want exactly one", aside)
	}
	if got := tankNames(t, aside[0]); len(got) != 1 || got[0] != "current tank" {
		t.Errorf("moved-aside tanks = %v, want [current tank]", got)
	}
	if !strings.Contains(out, aside[0]) || !strings.Contains(out, f.dbPath) {
		t.Errorf("output does not say where things went: %q", out)
	}
	if fileHash(t, f.backup) != wantHash {
		t.Error("the backup file was changed")
	}
	if leftovers, _ := filepath.Glob(filepath.Join(f.dir, "tmp", "*")); len(leftovers) != 0 {
		t.Errorf("temporary files left: %v", leftovers)
	}
}

func TestRestoreRestoredFileMatchesTheBackupByteForByte(t *testing.T) {
	f := newRestoreFixture(t)
	want := fileHash(t, f.backup)
	if code, out, errOut := f.restore(f.backup); code != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
	if got := fileHash(t, f.dbPath); got != want {
		t.Error("the restored database differs from the backup file")
	}
}

func TestRestoreRefusesWhileAServerAnswers(t *testing.T) {
	f := newRestoreFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	f.listen = strings.TrimPrefix(srv.URL, "http://")
	before := fileHash(t, f.dbPath)

	code, _, errOut := f.restore(f.backup)
	if code != 1 || !strings.Contains(errOut, "answering on "+f.listen) {
		t.Errorf("code=%d stderr=%q, want a refusal naming the address", code, errOut)
	}
	if fileHash(t, f.dbPath) != before {
		t.Error("the database was changed although a server was answering")
	}
	if len(f.prerestoreFiles()) != 0 {
		t.Error("the database was moved aside although a server was answering")
	}
}

func TestRestoreRefusesACorruptOrUnusableFile(t *testing.T) {
	f := newRestoreFixture(t)
	before := fileHash(t, f.dbPath)

	garbage := filepath.Join(t.TempDir(), "garbage.db")
	if err := os.WriteFile(garbage, []byte(strings.Repeat("not a database ", 500)), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing.db")
	for name, file := range map[string]string{"garbage": garbage, "missing": missing} {
		code, _, errOut := f.restore(file)
		if code != 1 || !strings.Contains(errOut, "not a usable backup") {
			t.Errorf("%s: code=%d stderr=%q", name, code, errOut)
		}
	}
	if fileHash(t, f.dbPath) != before || len(f.prerestoreFiles()) != 0 {
		t.Error("a refused restore touched the current database")
	}
}

func TestRestoreRefusesAStructurallyDamagedFile(t *testing.T) {
	f := newRestoreFixture(t)
	source := testutil.OpenDBAt(t, filepath.Join(t.TempDir(), "big.db"))
	for range 400 {
		testutil.Exec(t, source, `INSERT INTO tanks (name, note) VALUES ('t', ?)`, strings.Repeat("x", 200))
	}
	damaged := filepath.Join(t.TempDir(), "damaged.db")
	if err := source.BackupTo(t.Context(), damaged); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(damaged)
	if err != nil {
		t.Fatal(err)
	}
	raw[(len(raw)/4096-1)*4096] = 0x7f
	if err := os.WriteFile(damaged, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	before := fileHash(t, f.dbPath)

	code, _, errOut := f.restore(damaged)
	if code != 1 || !strings.Contains(errOut, "integrity check failed") {
		t.Errorf("code=%d stderr=%q, want an integrity check failure", code, errOut)
	}
	if fileHash(t, f.dbPath) != before {
		t.Error("a damaged backup replaced the database")
	}
}

func TestRestoreRefusesANewerSchema(t *testing.T) {
	f := newRestoreFixture(t)
	c, err := sql.Open("sqlite", "file:"+f.backup)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ExecContext(t.Context(), `INSERT INTO goose_db_version (version_id, is_applied) VALUES (99, 1)`); err != nil {
		t.Fatal(err)
	}
	_ = c.Close()
	before := fileHash(t, f.dbPath)

	code, _, errOut := f.restore(f.backup)
	if code != 1 || !strings.Contains(errOut, "newer version") {
		t.Errorf("code=%d stderr=%q, want a refusal for a newer schema", code, errOut)
	}
	if fileHash(t, f.dbPath) != before || len(f.prerestoreFiles()) != 0 {
		t.Error("a refused restore touched the current database")
	}
}

func TestRestoreRefusesTheCurrentDatabaseAsTheSource(t *testing.T) {
	f := newRestoreFixture(t)
	code, _, errOut := f.restore(f.dbPath)
	if code != 1 || !strings.Contains(errOut, "current database") {
		t.Errorf("code=%d stderr=%q", code, errOut)
	}
}

func TestRestoreIntoAnEmptyDataDirectory(t *testing.T) {
	f := newRestoreFixture(t)
	empty := t.TempDir()
	f.dir = filepath.Join(empty, "fresh")
	f.dbPath = filepath.Join(f.dir, app.DBFileName)

	code, out, errOut := f.restore(f.backup)
	if code != 0 || !strings.Contains(out, "no previous database") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
	if got := tankNames(t, f.dbPath); len(got) != 1 || got[0] != "backup tank" {
		t.Errorf("restored tanks = %v", got)
	}
}

func TestRestoreMovesAnUnreadableCurrentDatabaseAside(t *testing.T) {
	f := newRestoreFixture(t)
	damaged := []byte(strings.Repeat("damaged ", 600))
	if err := os.WriteFile(f.dbPath, damaged, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.dbPath+"-wal", []byte("stale log"), 0o600); err != nil {
		t.Fatal(err)
	}

	code, out, errOut := f.restore(f.backup)
	if code != 0 || !strings.Contains(errOut, "could not checkpoint") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
	aside := f.prerestoreFiles()
	if len(aside) != 1 {
		t.Fatalf("prerestore files = %v", aside)
	}
	if b, err := os.ReadFile(aside[0]); err != nil || !bytes.Equal(b, damaged) {
		t.Errorf("the damaged database was not kept: %v", err)
	}
	if _, err := os.Stat(f.dbPath + "-wal"); err == nil {
		t.Error("the old log file was left beside the restored database")
	}
	if got := tankNames(t, f.dbPath); len(got) != 1 || got[0] != "backup tank" {
		t.Errorf("restored tanks = %v", got)
	}
}

func TestRestoreDropsLogFilesLeftWithoutADatabase(t *testing.T) {
	f := newRestoreFixture(t)
	if err := os.Remove(f.dbPath); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.WriteFile(f.dbPath+suffix, []byte("stale"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if code, out, errOut := f.restore(f.backup); code != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(f.dbPath + suffix); err == nil {
			t.Errorf("stale %s file left beside the restored database", suffix)
		}
	}
}
