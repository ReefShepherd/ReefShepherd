package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
)

func openTemp(t *testing.T) *DB {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "reef.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func migrated(t *testing.T) *DB {
	t.Helper()
	d := openTemp(t)
	if err := d.Migrate(t.Context(), filepath.Join(t.TempDir(), "backups")); err != nil {
		t.Fatal(err)
	}
	return d
}

func count(t *testing.T, conn dbgen.DBTX, query string, args ...any) int {
	t.Helper()
	var n int
	if err := conn.QueryRowContext(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func exec(t *testing.T, conn dbgen.DBTX, query string, args ...any) {
	t.Helper()
	if _, err := conn.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func TestMigrateCreatesSchemaAndSeeds(t *testing.T) {
	backups := filepath.Join(t.TempDir(), "backups")
	d := openTemp(t)
	if err := d.Migrate(t.Context(), backups); err != nil {
		t.Fatal(err)
	}

	if v, err := d.Version(t.Context()); err != nil || v != 1 {
		t.Errorf("version = %d, %v; want 1", v, err)
	}
	if _, err := os.Stat(backups); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a fresh database wrote a backup (stat err = %v)", err)
	}

	wantTables := []string{
		"dashboard_cards", "parameter_definitions", "sessions", "settings", "tank_parameters",
		"tanks", "test_results", "user_preferences", "user_recovery_codes", "user_totp", "users",
	}
	rows, err := d.R.QueryContext(t.Context(),
		`SELECT name, strict FROM pragma_table_list WHERE schema = 'main' AND type = 'table' AND name NOT LIKE '%sqlite_%' AND name NOT LIKE 'goose_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var gotTables []string
	for rows.Next() {
		var name string
		var strict int
		if err := rows.Scan(&name, &strict); err != nil {
			t.Fatal(err)
		}
		gotTables = append(gotTables, name)
		if strict != 1 {
			t.Errorf("table %s is not STRICT", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(gotTables, wantTables) {
		t.Errorf("tables = %v, want %v", gotTables, wantTables)
	}

	wantIndexes := map[string]string{
		"user_recovery_codes_user": "user_id",
		"sessions_user":            "user_id",
		"sessions_expires":         "expires_at",
		"test_results_series":      "tank_id,parameter_id,measured_at",
		"dashboard_cards_owner":    "user_id,tank_id,position",
	}
	for idx, wantColumns := range wantIndexes {
		var gotColumns string
		err := d.R.QueryRowContext(t.Context(),
			`SELECT COALESCE(group_concat(name, ','), '') FROM (SELECT name FROM pragma_index_info(?) ORDER BY seqno)`, idx).Scan(&gotColumns)
		if err != nil {
			t.Fatal(err)
		}
		if gotColumns != wantColumns {
			t.Errorf("index %s columns = %q, want %q", idx, gotColumns, wantColumns)
		}
	}
	if n := count(t, d.R, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name NOT LIKE 'sqlite_%'`); n != len(wantIndexes) {
		t.Errorf("%d named indexes, want %d", n, len(wantIndexes))
	}
	if n := count(t, d.R, `SELECT COUNT(*) FROM settings`); n != 0 {
		t.Errorf("settings has %d seeded rows, want 0", n)
	}
}

func TestMigrateSeedsParameterPresets(t *testing.T) {
	d := migrated(t)

	type preset struct {
		key, name, family, label string
		low, high                *float64
		interval                 int
	}
	f := func(v float64) *float64 { return &v }
	want := []preset{
		{"temperature", "Temperature", "temperature", "", f(25.0), f(26.5), 7},
		{"salinity", "Salinity", "salinity", "", f(34.0), f(35.5), 7},
		{"ph", "pH", "fixed", "", f(7.9), f(8.4), 7},
		{"alkalinity", "Alkalinity", "alkalinity", "", f(7.0), f(10.0), 7},
		{"calcium", "Calcium", "fixed", "ppm", f(400), f(450), 7},
		{"magnesium", "Magnesium", "fixed", "ppm", f(1250), f(1400), 14},
		{"nitrate", "Nitrate", "fixed", "ppm", f(2), f(15), 7},
		{"phosphate", "Phosphate", "phosphate", "", f(0.02), f(0.10), 7},
		{"ammonia", "Ammonia", "fixed", "ppm", nil, f(0.05), 30},
		{"nitrite", "Nitrite", "fixed", "ppm", nil, f(0.1), 30},
	}

	rows, err := d.R.QueryContext(t.Context(),
		`SELECT preset_key, name, unit_family, unit_label, default_low, default_high, default_test_interval_days
		FROM parameter_definitions ORDER BY sort_order`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []preset
	for rows.Next() {
		var p preset
		if err := rows.Scan(&p.key, &p.name, &p.family, &p.label, &p.low, &p.high, &p.interval); err != nil {
			t.Fatal(err)
		}
		got = append(got, p)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d presets, want %d", len(got), len(want))
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.key != w.key || g.name != w.name || g.family != w.family || g.label != w.label || g.interval != w.interval ||
			!equalFloatPtr(g.low, w.low) || !equalFloatPtr(g.high, w.high) {
			t.Errorf("preset %d = %+v, want %+v", i, g, w)
		}
	}
}

func equalFloatPtr(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func TestMigrateTwiceIsNoOp(t *testing.T) {
	backups := filepath.Join(t.TempDir(), "backups")
	d := openTemp(t)
	for range 2 {
		if err := d.Migrate(t.Context(), backups); err != nil {
			t.Fatal(err)
		}
		exec(t, d.W, `INSERT INTO tanks (name) VALUES ('kept')`)
	}
	if n := count(t, d.R, `SELECT COUNT(*) FROM goose_db_version WHERE version_id > 0`); n != 1 {
		t.Errorf("%d versions recorded, want 1", n)
	}
	if n := count(t, d.R, `SELECT COUNT(*) FROM parameter_definitions`); n != 10 {
		t.Errorf("%d presets after the second migrate, want 10", n)
	}
	if _, err := os.Stat(backups); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a no-op migrate wrote a backup (stat err = %v)", err)
	}
}

// upgradeFS is the real first migration plus a second one, so tests can exercise the upgrade path.
func upgradeFS(t *testing.T, second string) fs.FS {
	t.Helper()
	first, err := fs.ReadFile(migrationFiles, "migrations/00001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	return fstest.MapFS{
		"00001_initial.sql": {Data: first},
		"00002_next.sql":    {Data: []byte(second)},
	}
}

const addTankColor = "-- +goose Up\nALTER TABLE tanks ADD COLUMN color TEXT NOT NULL DEFAULT 'blue';\n"

func firstOnly(t *testing.T) fs.FS {
	t.Helper()
	fsys := upgradeFS(t, "")
	delete(fsys.(fstest.MapFS), "00002_next.sql")
	return fsys
}

func TestMigrateUpgradeWritesPremigrateBackup(t *testing.T) {
	ctx := t.Context()
	backups := filepath.Join(t.TempDir(), "backups")
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	d := openTemp(t)

	if err := d.migrate(ctx, firstOnly(t), backups, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backups); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("fresh database wrote a backup (stat err = %v)", err)
	}
	exec(t, d.W, `INSERT INTO tanks (name) VALUES ('before upgrade')`)

	if err := d.migrate(ctx, upgradeFS(t, addTankColor), backups, now); err != nil {
		t.Fatal(err)
	}

	backup := filepath.Join(backups, "reefshepherd-premigrate-00002-20261101T120000Z.db")
	copyDB, err := Open(backup)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer copyDB.Close()
	if n := count(t, copyDB.R, `SELECT COUNT(*) FROM tanks WHERE name = 'before upgrade'`); n != 1 {
		t.Errorf("backup holds %d tanks named 'before upgrade', want 1", n)
	}
	if n := count(t, copyDB.R, `SELECT COUNT(*) FROM pragma_table_info('tanks') WHERE name = 'color'`); n != 0 {
		t.Error("backup was taken after the migration ran")
	}
	if n := count(t, d.R, `SELECT COUNT(*) FROM pragma_table_info('tanks') WHERE name = 'color'`); n != 1 {
		t.Error("live database was not upgraded")
	}
	leftovers, _ := filepath.Glob(filepath.Join(backups, "*.tmp"))
	if len(leftovers) != 0 {
		t.Errorf("temporary backup files left: %v", leftovers)
	}
}

func TestMigrateFailureKeepsBackupAndOldSchema(t *testing.T) {
	ctx := t.Context()
	backups := filepath.Join(t.TempDir(), "backups")
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	d := openTemp(t)
	if err := d.migrate(ctx, firstOnly(t), backups, now); err != nil {
		t.Fatal(err)
	}

	broken := "-- +goose Up\nALTER TABLE tanks ADD COLUMN color TEXT NOT NULL DEFAULT 'blue';\nTHIS IS NOT SQL;\n"
	if err := d.migrate(ctx, upgradeFS(t, broken), backups, now); err == nil {
		t.Fatal("broken migration succeeded")
	}

	if v, err := d.Version(ctx); err != nil || v != 1 {
		t.Errorf("version after failure = %d, %v; want 1", v, err)
	}
	if n := count(t, d.R, `SELECT COUNT(*) FROM pragma_table_info('tanks') WHERE name = 'color'`); n != 0 {
		t.Error("half of the failed migration was kept")
	}
	if _, err := os.Stat(filepath.Join(backups, "reefshepherd-premigrate-00002-20261101T120000Z.db")); err != nil {
		t.Errorf("backup missing after failed migration: %v", err)
	}
}

func TestMigrateRefusesDatabaseNewerThanBuild(t *testing.T) {
	ctx := t.Context()
	backups := filepath.Join(t.TempDir(), "backups")
	d := openTemp(t)
	if err := d.migrate(ctx, upgradeFS(t, addTankColor), backups, time.Now()); err != nil {
		t.Fatal(err)
	}
	err := d.migrate(ctx, firstOnly(t), backups, time.Now())
	if err == nil || !strings.Contains(err.Error(), "newer than this build") {
		t.Errorf("err = %v, want a newer-than-build error", err)
	}
}

func TestPragmasOnEveryConnection(t *testing.T) {
	d := openTemp(t)
	check := func(name string, pool *sql.DB, conns int, queryOnly int) {
		t.Helper()
		var held []*sql.Conn
		defer func() {
			for _, c := range held {
				_ = c.Close()
			}
		}()
		for i := range conns {
			c, err := pool.Conn(t.Context())
			if err != nil {
				t.Fatalf("%s conn %d: %v", name, i, err)
			}
			held = append(held, c)
			for pragma, want := range map[string]any{
				"foreign_keys": int64(1), "busy_timeout": int64(5000), "synchronous": int64(1),
				"journal_mode": "wal", "query_only": int64(queryOnly),
			} {
				var got any
				if err := c.QueryRowContext(t.Context(), "PRAGMA "+pragma).Scan(&got); err != nil {
					t.Fatal(err)
				}
				if got != want {
					t.Errorf("%s conn %d: %s = %v, want %v", name, i, pragma, got, want)
				}
			}
		}
		// One more connection than the pool allows must block.
		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()
		if extra, err := pool.Conn(ctx); err == nil {
			_ = extra.Close()
			t.Errorf("%s: a connection beyond the pool size was granted", name)
		}
	}
	check("writer", d.W, 1, 0)
	check("reader", d.R, 4, 1)
}

func TestReaderPoolRejectsWrites(t *testing.T) {
	d := migrated(t)
	if _, err := d.R.ExecContext(t.Context(), `INSERT INTO tanks (name) VALUES ('x')`); err == nil {
		t.Fatal("reader pool accepted a write")
	}
}

func TestWriterTransactionsTakeTheWriteLockAtBegin(t *testing.T) {
	d := migrated(t)
	other, err := sql.Open("sqlite", dsnWithBusyTimeout(t, d, 100))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()

	err = d.InTx(t.Context(), func(*sql.Tx) error {
		// The transaction has not written anything yet, so only BEGIN IMMEDIATE blocks this write.
		_, err := other.ExecContext(t.Context(), `INSERT INTO tanks (name) VALUES ('blocked')`)
		if err == nil {
			t.Error("a second connection wrote while the writer transaction was open")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// dsnWithBusyTimeout opens a plain second connection to the same file with a short busy timeout.
func dsnWithBusyTimeout(t *testing.T, d *DB, millis int) string {
	t.Helper()
	var file string
	if err := d.R.QueryRowContext(t.Context(), `SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&file); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("file:%s?_pragma=busy_timeout(%d)", (&url.URL{Path: file}).EscapedPath(), millis)
}

func TestInTx(t *testing.T) {
	d := migrated(t)
	insert := func(tx *sql.Tx, name string) error {
		_, err := tx.ExecContext(t.Context(), `INSERT INTO tanks (name) VALUES (?)`, name)
		return err
	}
	tanks := func() int { return count(t, d.R, `SELECT COUNT(*) FROM tanks`) }

	if err := d.InTx(t.Context(), func(tx *sql.Tx) error { return insert(tx, "committed") }); err != nil {
		t.Fatal(err)
	}
	if tanks() != 1 {
		t.Fatalf("tanks after commit = %d, want 1", tanks())
	}

	boom := errors.New("boom")
	err := d.InTx(t.Context(), func(tx *sql.Tx) error {
		if err := insert(tx, "rolled back"); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom", err)
	}
	if tanks() != 1 {
		t.Errorf("tanks after rollback = %d, want 1", tanks())
	}
}

func TestInTxPanicReleasesTheWriter(t *testing.T) {
	d := migrated(t)
	func() {
		defer func() { _ = recover() }()
		_ = d.InTx(t.Context(), func(tx *sql.Tx) error {
			_, _ = tx.ExecContext(t.Context(), `INSERT INTO tanks (name) VALUES ('lost')`)
			panic("handler bug")
		})
	}()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if _, err := d.W.ExecContext(ctx, `INSERT INTO tanks (name) VALUES ('after panic')`); err != nil {
		t.Fatalf("writer stayed locked after a panic: %v", err)
	}
	if n := count(t, d.R, `SELECT COUNT(*) FROM tanks`); n != 1 {
		t.Errorf("tanks = %d, want only the one written after the panic", n)
	}
}

func TestQBindsToPoolAndTransaction(t *testing.T) {
	d := migrated(t)
	ctx := t.Context()
	err := d.InTx(ctx, func(tx *sql.Tx) error {
		return Q(tx).SetSetting(ctx, dbgen.SetSettingParams{Key: "k", Value: "in tx"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if v, err := Q(d.R).GetSetting(ctx, "k"); err != nil || v != "in tx" {
		t.Errorf("GetSetting = %q, %v", v, err)
	}
}

func TestSettingQueries(t *testing.T) {
	d := migrated(t)
	ctx := t.Context()
	q := Q(d.W)

	if _, err := q.GetSetting(ctx, "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetSetting(missing) err = %v, want sql.ErrNoRows", err)
	}

	if err := q.SetSetting(ctx, dbgen.SetSettingParams{Key: "timezone", Value: "UTC"}); err != nil {
		t.Fatal(err)
	}
	exec(t, d.W, `UPDATE settings SET updated_at = '2000-01-01T00:00:00Z'`)
	if err := q.SetSetting(ctx, dbgen.SetSettingParams{Key: "timezone", Value: "Europe/Berlin"}); err != nil {
		t.Fatal(err)
	}
	v, err := q.GetSetting(ctx, "timezone")
	if err != nil || v != "Europe/Berlin" {
		t.Errorf("GetSetting = %q, %v; want the updated value", v, err)
	}
	var updatedAt string
	if err := d.W.QueryRowContext(ctx, `SELECT updated_at FROM settings WHERE key = 'timezone'`).Scan(&updatedAt); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(updatedAt, time.Now().UTC().Format("2006-01-02")) || !strings.HasSuffix(updatedAt, "Z") {
		t.Errorf("updated_at = %q, want today's UTC timestamp", updatedAt)
	}

	if n, err := q.DeleteSetting(ctx, "timezone"); err != nil || n != 1 {
		t.Errorf("DeleteSetting = %d, %v; want 1", n, err)
	}
	if n, err := q.DeleteSetting(ctx, "timezone"); err != nil || n != 0 {
		t.Errorf("second DeleteSetting = %d, %v; want 0", n, err)
	}
}

func TestSchemaConstraints(t *testing.T) {
	d := migrated(t)
	ctx := t.Context()
	exec(t, d.W, `INSERT INTO users (id, username) VALUES (1, 'Jayce')`)
	exec(t, d.W, `INSERT INTO tanks (id, name) VALUES (1, 'Display')`)
	exec(t, d.W, `INSERT INTO tank_parameters (tank_id, parameter_id) VALUES (1, 1)`)
	exec(t, d.W, `INSERT INTO test_results (id, tank_id, parameter_id, measured_at, value, created_by)
		VALUES (1, 1, 1, '2026-10-24T10:00:00Z', 25.5, 1)`)
	exec(t, d.W, `INSERT INTO dashboard_cards (user_id, tank_id, card_type, position) VALUES (1, 1, 'tests_due', 0)`)
	exec(t, d.W, `INSERT INTO sessions (token_hash, user_id, stage, auth_method, expires_at)
		VALUES (x'00', 1, 'active', 'password', '2026-11-01T00:00:00Z')`)

	rejected := map[string]string{
		"foreign key to a missing user":      `INSERT INTO sessions (token_hash, user_id, stage, auth_method, expires_at) VALUES (x'01', 99, 'active', 'password', 'x')`,
		"user name differing only by case":   `INSERT INTO users (username) VALUES ('JAYCE')`,
		"disabled flag outside 0 and 1":      `INSERT INTO users (username, disabled) VALUES ('flag', 2)`,
		"text in an integer column (STRICT)": `INSERT INTO tanks (name, sort_order) VALUES ('t', 'first')`,
		"zero tank volume":                   `INSERT INTO tanks (name, volume_liters) VALUES ('t', 0)`,
		"weekday mask above 127":             `UPDATE tank_parameters SET weekdays = 128`,
		"interval of zero days":              `UPDATE tank_parameters SET interval_days = 0`,
		"interval of zero weeks":             `UPDATE tank_parameters SET interval_weeks = 0`,
		"month day zero":                     `UPDATE tank_parameters SET month_day = 0`,
		"month day above 31":                 `UPDATE tank_parameters SET month_day = 32`,
		"enabled flag outside 0 and 1":       `UPDATE tank_parameters SET enabled = 2`,
		"decimals above 4":                   `UPDATE parameter_definitions SET decimals = 5`,
		"duplicate preset key":               `INSERT INTO parameter_definitions (preset_key, name) VALUES ('ph', 'Another pH')`,
		"deleting a parameter with results":  `DELETE FROM parameter_definitions WHERE id = 1`,
	}
	for name, stmt := range rejected {
		if _, err := d.W.ExecContext(ctx, stmt); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	exec(t, d.W, `DELETE FROM users WHERE id = 1`)
	if n := count(t, d.W, `SELECT COUNT(*) FROM sessions`) + count(t, d.W, `SELECT COUNT(*) FROM dashboard_cards`); n != 0 {
		t.Errorf("%d sessions and cards survived their user", n)
	}
	var createdBy *int64
	if err := d.W.QueryRowContext(ctx, `SELECT created_by FROM test_results WHERE id = 1`).Scan(&createdBy); err != nil || createdBy != nil {
		t.Errorf("created_by = %v, %v; want NULL after the user is deleted", createdBy, err)
	}

	exec(t, d.W, `DELETE FROM tanks WHERE id = 1`)
	if n := count(t, d.W, `SELECT COUNT(*) FROM test_results`) + count(t, d.W, `SELECT COUNT(*) FROM tank_parameters`); n != 0 {
		t.Errorf("%d results and tank parameters survived their tank", n)
	}
}

func TestOpenHandlesAwkwardPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my reef #1 100%", "reef's.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	exec(t, d.W, `CREATE TABLE t (x INTEGER)`)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("database file not at the requested path: %v", err)
	}
}

func TestBackupToWritesVerifiedCopy(t *testing.T) {
	d := migrated(t)
	exec(t, d.W, `INSERT INTO tanks (name) VALUES ('backed up')`)
	dir := t.TempDir()
	dest := filepath.Join(dir, "it's a backup.db")
	// A leftover from a crashed backup must not block the next one.
	if err := os.WriteFile(dest+".tmp", []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := d.BackupTo(t.Context(), dest); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(dest + ".tmp"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("temporary file left behind (stat err = %v)", err)
	}
	c, err := Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if n := count(t, c.R, `SELECT COUNT(*) FROM tanks WHERE name = 'backed up'`); n != 1 {
		t.Errorf("copy holds %d matching tanks, want 1", n)
	}
	if v, err := c.Version(t.Context()); err != nil || v != 1 {
		t.Errorf("copy version = %d, %v; want 1", v, err)
	}

	if err := d.BackupTo(t.Context(), dest); err != nil {
		t.Errorf("overwriting an existing backup: %v", err)
	}
}

func TestBackupToLeavesNothingOnFailure(t *testing.T) {
	d := migrated(t)
	dest := filepath.Join(t.TempDir(), "missing-dir", "backup.db")
	if err := d.BackupTo(t.Context(), dest); err == nil {
		t.Fatal("backup into a missing directory succeeded")
	}
	if _, err := os.Stat(dest); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a file exists at the destination (stat err = %v)", err)
	}
}

func TestCheckCopyRejectsDamagedFile(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "bad.db")
	if err := os.WriteFile(bad, []byte(strings.Repeat("not a database ", 500)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkCopy(t.Context(), bad); err == nil {
		t.Fatal("a file that is not a database passed the check")
	}
}

func TestForeignKeyActions(t *testing.T) {
	d := migrated(t)
	exec(t, d.W, `INSERT INTO users (id, username) VALUES (1, 'owner'), (2, 'other')`)
	exec(t, d.W, `INSERT INTO tanks (id, name) VALUES (1, 'preferred'), (2, 'cards')`)
	exec(t, d.W, `INSERT INTO parameter_definitions (id, name) VALUES (100, 'custom')`)
	exec(t, d.W, `INSERT INTO user_preferences (user_id, default_tank_id) VALUES (1, 1), (2, 1)`)
	exec(t, d.W, `INSERT INTO user_totp (user_id, secret) VALUES (1, 'SECRET')`)
	exec(t, d.W, `INSERT INTO user_recovery_codes (user_id, code_hash) VALUES (1, 'hash')`)
	exec(t, d.W, `INSERT INTO dashboard_cards (user_id, tank_id, card_type, parameter_id, position) VALUES (2, 2, 'parameter', 100, 0)`)

	exec(t, d.W, `DELETE FROM tanks WHERE id = 1`)
	if n := count(t, d.W, `SELECT COUNT(*) FROM user_preferences WHERE default_tank_id IS NULL`); n != 2 {
		t.Errorf("%d preference rows have a NULL default tank after the tank was deleted, want 2", n)
	}

	exec(t, d.W, `DELETE FROM parameter_definitions WHERE id = 100`)
	if n := count(t, d.W, `SELECT COUNT(*) FROM dashboard_cards`); n != 0 {
		t.Errorf("%d dashboard cards survived their parameter", n)
	}

	exec(t, d.W, `DELETE FROM users WHERE id = 1`)
	for _, table := range []string{"user_totp", "user_recovery_codes"} {
		if n := count(t, d.W, `SELECT COUNT(*) FROM `+table+` WHERE user_id = 1`); n != 0 {
			t.Errorf("%d %s rows survived their user", n, table)
		}
	}
	if n := count(t, d.W, `SELECT COUNT(*) FROM user_preferences`); n != 1 {
		t.Errorf("%d preference rows after deleting one user, want the other user's 1", n)
	}
}

func TestTimestampDefaultsUseUTCFormat(t *testing.T) {
	d := migrated(t)
	ctx := t.Context()
	exec(t, d.W, `INSERT INTO settings (key, value) VALUES ('k', 'v')`)
	exec(t, d.W, `INSERT INTO users (id, username) VALUES (1, 'owner')`)
	exec(t, d.W, `INSERT INTO user_totp (user_id, secret) VALUES (1, 'SECRET')`)
	exec(t, d.W, `INSERT INTO user_recovery_codes (user_id, code_hash) VALUES (1, 'hash')`)
	exec(t, d.W, `INSERT INTO sessions (token_hash, user_id, stage, auth_method, expires_at) VALUES (x'00', 1, 'active', 'password', 'x')`)
	exec(t, d.W, `INSERT INTO tanks (id, name) VALUES (1, 'tank')`)
	exec(t, d.W, `INSERT INTO user_preferences (user_id) VALUES (1)`)
	exec(t, d.W, `INSERT INTO tank_parameters (tank_id, parameter_id) VALUES (1, 1)`)
	exec(t, d.W, `INSERT INTO test_results (tank_id, parameter_id, measured_at, value) VALUES (1, 1, 'x', 1)`)
	exec(t, d.W, `INSERT INTO dashboard_cards (user_id, tank_id, card_type, position) VALUES (1, 1, 'tests_due', 0)`)

	want := map[string]string{
		"settings":              "updated_at",
		"users":                 "created_at,updated_at",
		"user_totp":             "created_at",
		"user_recovery_codes":   "created_at",
		"sessions":              "created_at,last_seen_at",
		"tanks":                 "created_at,updated_at",
		"user_preferences":      "updated_at",
		"parameter_definitions": "created_at,updated_at",
		"tank_parameters":       "updated_at",
		"test_results":          "created_at,updated_at",
		"dashboard_cards":       "created_at",
	}
	shape := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)
	for table, columns := range want {
		var withDefault string
		err := d.R.QueryRowContext(ctx,
			`SELECT COALESCE(group_concat(name, ','), '') FROM (SELECT name FROM pragma_table_info(?) WHERE dflt_value LIKE '%strftime%' ORDER BY cid)`, table).Scan(&withDefault)
		if err != nil {
			t.Fatal(err)
		}
		if withDefault != columns {
			t.Errorf("%s columns defaulting to a timestamp = %q, want %q", table, withDefault, columns)
		}
		for _, column := range strings.Split(columns, ",") {
			var got string
			if err := d.R.QueryRowContext(ctx, fmt.Sprintf(`SELECT %s FROM %s ORDER BY rowid DESC LIMIT 1`, column, table)).Scan(&got); err != nil {
				t.Fatal(err)
			}
			parsed, err := time.Parse(time.RFC3339, got)
			if !shape.MatchString(got) || err != nil || time.Since(parsed).Abs() > time.Minute {
				t.Errorf("%s.%s default = %q, want a current UTC timestamp like 2026-01-02T03:04:05Z", table, column, got)
			}
		}
	}
}

func TestSyncPath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{file, dir} {
		if err := syncPath(p); err != nil {
			t.Errorf("syncPath(%s): %v", p, err)
		}
	}
	if err := syncPath(filepath.Join(dir, "missing")); err == nil {
		t.Error("syncPath of a missing path succeeded")
	}
}
