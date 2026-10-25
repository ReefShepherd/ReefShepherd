package app

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/config"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func presetCount(t *testing.T, dbPath string) int {
	t.Helper()
	conn, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM parameter_definitions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestOpenDatabaseCreatesSchemaAndClearsTmp(t *testing.T) {
	dataDir := t.TempDir()
	tmp := filepath.Join(dataDir, tmpDirName)
	if err := os.MkdirAll(filepath.Join(tmp, "upload"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "upload", "half.jpg"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(dataDir, "keep.txt")
	if err := os.WriteFile(keep, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(&syncBuffer{}, nil))
	d, err := openDatabase(t.Context(), dataDir, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	if n := presetCount(t, filepath.Join(dataDir, dbFileName)); n != 10 {
		t.Errorf("%d presets seeded, want 10", n)
	}
	entries, err := os.ReadDir(tmp)
	if err != nil || len(entries) != 0 {
		t.Errorf("tmp dir = %v, %v; want it present and empty", entries, err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("a file outside tmp was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, backupsDirName)); err == nil {
		t.Error("a fresh database wrote a backup")
	}
}

func TestOpenDatabaseKeepsDataAcrossRestart(t *testing.T) {
	dataDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(&syncBuffer{}, nil))

	d, err := openDatabase(t.Context(), dataDir, logger)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.W.ExecContext(t.Context(), `INSERT INTO tanks (name) VALUES ('survivor')`); err != nil {
		t.Fatal(err)
	}
	_ = d.Close()

	d, err = openDatabase(t.Context(), dataDir, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var n int
	if err := d.R.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM tanks WHERE name = 'survivor'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("tank count after restart = %d, %v; want 1", n, err)
	}
	if n := presetCount(t, filepath.Join(dataDir, dbFileName)); n != 10 {
		t.Errorf("%d presets after restart, want 10", n)
	}
	if _, err := os.Stat(filepath.Join(dataDir, backupsDirName)); err == nil {
		t.Error("a restart with nothing pending wrote a backup")
	}
}

func TestOpenDatabaseFailsOnUnusableDataDir(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(&syncBuffer{}, nil))
	if _, err := openDatabase(t.Context(), file, logger); err == nil {
		t.Fatal("expected an error when the data dir is a file")
	}
}

func TestRunPreparesDatabaseBeforeServing(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	cfg := config.Config{Listen: "127.0.0.1:0", DataDir: dataDir}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg, logger) }()

	deadline := time.After(10 * time.Second)
	for !strings.Contains(logs.String(), "msg=starting") {
		select {
		case err := <-done:
			t.Fatalf("Run returned before serving: %v\n%s", err, logs.String())
		case <-deadline:
			t.Fatalf("Run never started serving\n%s", logs.String())
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run = %v after shutdown", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after the context was cancelled")
	}

	if !strings.Contains(logs.String(), "database ready") {
		t.Errorf("no database ready log:\n%s", logs.String())
	}
	// SQLite removes the write-ahead log when the last connection closes cleanly.
	if _, err := os.Stat(filepath.Join(dataDir, dbFileName+"-wal")); err == nil {
		t.Error("database still open after Run returned")
	}
	if n := presetCount(t, filepath.Join(dataDir, dbFileName)); n != 10 {
		t.Errorf("%d presets seeded, want 10", n)
	}
}
