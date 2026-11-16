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

	if v, err := d.Version(t.Context()); err != nil || v != 2 {
		t.Errorf("version = %d, %v; want 2", v, err)
	}
	if _, err := os.Stat(backups); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a fresh database wrote a backup (stat err = %v)", err)
	}

	wantTables := []string{
		"dashboard_cards", "maintenance_completions", "maintenance_presets", "maintenance_tasks",
		"notification_channels", "notification_deliveries", "notification_reads", "notification_subscriptions",
		"notifications", "parameter_definitions", "sessions", "settings", "tank_parameters",
		"tanks", "test_reminders", "test_results", "user_preferences", "user_recovery_codes", "user_totp", "users",
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

		"maintenance_tasks_tank":          "tank_id",
		"maintenance_completions_task":    "task_id,done_at",
		"notifications_created":           "created_at",
		"notification_deliveries_pending": "status,next_attempt_at",
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
		decimals                 int
	}
	f := func(v float64) *float64 { return &v }
	want := []preset{
		{"temperature", "Temperature", "temperature", "", f(24.5), f(27.5), 7, 1},
		{"salinity", "Salinity", "salinity", "", f(34.0), f(35.5), 7, 3},
		{"ph", "pH", "fixed", "", f(7.9), f(8.4), 7, 1},
		{"alkalinity", "Alkalinity", "alkalinity", "", f(7.0), f(11.0), 7, 1},
		{"calcium", "Calcium", "fixed", "ppm", f(380), f(450), 7, 0},
		{"magnesium", "Magnesium", "fixed", "ppm", f(1250), f(1400), 14, 0},
		{"nitrate", "Nitrate", "fixed", "ppm", f(5), f(50), 7, 1},
		{"phosphate", "Phosphate", "phosphate", "", f(0.06), f(0.30), 7, 2},
		{"ammonia", "Ammonia", "fixed", "ppm", nil, f(0.10), 30, 2},
		{"nitrite", "Nitrite", "fixed", "ppm", nil, f(0.1), 30, 1},
	}

	rows, err := d.R.QueryContext(t.Context(),
		`SELECT preset_key, name, unit_family, unit_label, default_low, default_high, default_test_interval_days, decimals
		FROM parameter_definitions ORDER BY sort_order`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []preset
	for rows.Next() {
		var p preset
		if err := rows.Scan(&p.key, &p.name, &p.family, &p.label, &p.low, &p.high, &p.interval, &p.decimals); err != nil {
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
		if g.key != w.key || g.name != w.name || g.family != w.family || g.label != w.label || g.interval != w.interval || g.decimals != w.decimals ||
			!equalFloatPtr(g.low, w.low) || !equalFloatPtr(g.high, w.high) {
			t.Errorf("preset %d = %+v, want %+v", i, g, w)
		}
	}
}

func TestMigrateSeedsMaintenancePresets(t *testing.T) {
	d := migrated(t)

	type preset struct {
		key, name, kind string
		days            int
		tankless        bool
	}
	want := []preset{
		{"water_change", "Water change", "after_last", 7, false},
		{"skimmer_cup", "Empty skimmer cup", "after_last", 3, false},
		{"filter_media", "Change filter socks or fleece", "after_last", 3, false},
		{"glass", "Clean glass", "after_last", 3, false},
		{"ato_reservoir", "Refill ATO reservoir", "after_last", 7, false},
		{"carbon_gfo", "Replace carbon and GFO", "after_last", 30, false},
		{"probe_calibration", "Calibrate probes", "after_last", 30, false},
		{"overflow", "Clean overflow and plumbing", "after_last", 30, false},
		{"pumps", "Clean return pump and powerheads", "after_last", 90, false},
		{"rodi_cartridges", "Replace RO/DI cartridges", "after_last", 180, true},
	}

	rows, err := d.R.QueryContext(t.Context(),
		`SELECT preset_key, name, schedule_kind, interval_days, tankless, description, sort_order, archived_at
		FROM maintenance_presets ORDER BY sort_order`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []preset
	for rows.Next() {
		var p preset
		var tankless, sortOrder int
		var description string
		var archivedAt *string
		if err := rows.Scan(&p.key, &p.name, &p.kind, &p.days, &tankless, &description, &sortOrder, &archivedAt); err != nil {
			t.Fatal(err)
		}
		p.tankless = tankless == 1
		if description != "" || archivedAt != nil || sortOrder != (len(got)+1)*10 {
			t.Errorf("preset %s: description %q, archived_at %v, sort_order %d", p.key, description, archivedAt, sortOrder)
		}
		got = append(got, p)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("presets = %+v, want %+v", got, want)
	}
}

func TestMigrationTablesAreStrict(t *testing.T) {
	d := migrated(t)
	for _, table := range []string{
		"maintenance_presets", "maintenance_tasks", "maintenance_completions", "notification_channels",
		"notification_subscriptions", "notifications", "notification_reads", "notification_deliveries", "test_reminders",
	} {
		var strict int
		err := d.R.QueryRowContext(t.Context(), `SELECT strict FROM pragma_table_list WHERE schema = 'main' AND name = ?`, table).Scan(&strict)
		if err != nil || strict != 1 {
			t.Errorf("table %s: strict = %d, %v; want 1", table, strict, err)
		}
	}
	if _, err := d.W.ExecContext(t.Context(), `INSERT INTO notification_channels (name, kind, config_json, enabled) VALUES ('c', 'ntfy', '{}', 'yes')`); err == nil {
		t.Error("text in an integer column of a new table was accepted")
	}
}

func TestTestRemindersAreUniquePerDueTimeAndGoWithTheTank(t *testing.T) {
	d := migrated(t)
	exec(t, d.W, `INSERT INTO tanks (id, name) VALUES (1, 'tank')`)
	add := func(parameter int, due string) error {
		_, err := d.W.ExecContext(t.Context(), `INSERT INTO test_reminders (tank_id, parameter_id, due_at) VALUES (1, ?, ?)`, parameter, due)
		return err
	}
	if err := add(1, "2026-11-15T09:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if err := add(1, "2026-11-15T09:00:00Z"); err == nil {
		t.Error("the same parameter and due time was stored twice")
	}
	if err := add(1, "2026-11-16T09:00:00Z"); err != nil {
		t.Errorf("a new due time of the same parameter was refused: %v", err)
	}
	if err := add(2, "2026-11-15T09:00:00Z"); err != nil {
		t.Errorf("the same due time of another parameter was refused: %v", err)
	}
	exec(t, d.W, `DELETE FROM tanks WHERE id = 1`)
	if n := count(t, d.R, `SELECT COUNT(*) FROM test_reminders`); n != 0 {
		t.Errorf("reminders left after the tank was deleted = %d", n)
	}
}

func TestNotificationOccurrenceIsUnique(t *testing.T) {
	d := migrated(t)
	insert := func(kind string, id int, occurrence string) error {
		_, err := d.W.ExecContext(t.Context(),
			`INSERT INTO notifications (event_type, title, source_kind, source_id, occurrence) VALUES ('maintenance_due', 't', ?, ?, ?)`,
			kind, id, occurrence)
		return err
	}
	if err := insert("maintenance_task", 1, "2026-11-15T09:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if err := insert("maintenance_task", 1, "2026-11-15T09:00:00Z"); err == nil {
		t.Error("a second notification with the same source_kind, source_id and occurrence was accepted")
	}
	// Source id 0 is how a notification without a row source is keyed, and it must dedupe too.
	if err := insert("backup", 0, "2026-11-15T03:30:00Z"); err != nil {
		t.Fatal(err)
	}
	if err := insert("backup", 0, "2026-11-15T03:30:00Z"); err == nil {
		t.Error("a second notification for the same backup instant was accepted")
	}
	others := map[string]func() error{
		"another occurrence": func() error { return insert("maintenance_task", 1, "2026-11-16T09:00:00Z") },
		"another source id":  func() error { return insert("maintenance_task", 2, "2026-11-15T09:00:00Z") },
		"another source":     func() error { return insert("test_result", 1, "2026-11-15T09:00:00Z") },
	}
	for name, run := range others {
		if err := run(); err != nil {
			t.Errorf("%s was rejected: %v", name, err)
		}
	}
	if _, err := d.W.ExecContext(t.Context(), `INSERT INTO notifications (event_type, title, source_kind, source_id) VALUES ('e', 't', 'backup', 0)`); err == nil {
		t.Error("a notification without an occurrence was accepted")
	}
}

func TestMigrateFromM1KeepsDataAndWritesPremigrateBackup(t *testing.T) {
	ctx := t.Context()
	backups := filepath.Join(t.TempDir(), "backups")
	d := openTemp(t)

	if err := d.migrate(ctx, firstOnly(t), backups, time.Now()); err != nil {
		t.Fatal(err)
	}
	if v, err := d.Version(ctx); err != nil || v != 1 {
		t.Fatalf("version before upgrade = %d, %v; want 1", v, err)
	}
	exec(t, d.W, `INSERT INTO users (id, username) VALUES (1, 'owner')`)
	exec(t, d.W, `INSERT INTO tanks (id, name, volume_liters) VALUES (1, 'Display', 300)`)
	exec(t, d.W, `INSERT INTO tank_parameters (tank_id, parameter_id, ideal_low, ideal_high) VALUES (1, 1, 24.5, 27.5)`)
	exec(t, d.W, `INSERT INTO test_results (id, tank_id, parameter_id, measured_at, value, created_by) VALUES
		(1, 1, 1, '2026-10-24T10:00:00Z', 25.5, 1), (2, 1, 1, '2026-10-25T10:00:00Z', 26.0, 1)`)
	snapshot := func(conn dbgen.DBTX) string {
		t.Helper()
		var out string
		err := conn.QueryRowContext(ctx, `SELECT
			(SELECT group_concat(id || ':' || name || ':' || volume_liters, ',') FROM tanks) || '|' ||
			(SELECT group_concat(tank_id || ':' || parameter_id || ':' || ideal_low || ':' || ideal_high, ',') FROM tank_parameters) || '|' ||
			(SELECT group_concat(id || ':' || measured_at || ':' || value || ':' || created_by, ',') FROM (SELECT * FROM test_results ORDER BY id)) || '|' ||
			(SELECT COUNT(*) FROM parameter_definitions)`).Scan(&out)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	before := snapshot(d.R)

	if err := d.Migrate(ctx, backups); err != nil {
		t.Fatal(err)
	}

	if v, err := d.Version(ctx); err != nil || v != 2 {
		t.Errorf("version after upgrade = %d, %v; want 2", v, err)
	}
	if after := snapshot(d.R); after != before {
		t.Errorf("M1 rows changed by the upgrade:\n got %s\nwant %s", after, before)
	}
	if n := count(t, d.R, `SELECT COUNT(*) FROM maintenance_presets`); n != 10 {
		t.Errorf("%d maintenance presets after the upgrade, want 10", n)
	}

	files, err := filepath.Glob(filepath.Join(backups, "reefshepherd-premigrate-00002-*.db"))
	if err != nil || len(files) != 1 {
		t.Fatalf("premigrate backups = %v, %v; want exactly one", files, err)
	}
	copyDB, err := Open(files[0])
	if err != nil {
		t.Fatal(err)
	}
	defer copyDB.Close()
	if v, err := copyDB.Version(ctx); err != nil || v != 1 {
		t.Errorf("backup version = %d, %v; want 1", v, err)
	}
	if got := snapshot(copyDB.R); got != before {
		t.Errorf("backup rows differ from the M1 database:\n got %s\nwant %s", got, before)
	}
	if n := count(t, copyDB.R, `SELECT COUNT(*) FROM sqlite_master WHERE name = 'maintenance_tasks'`); n != 0 {
		t.Error("backup already holds the new tables")
	}
}

func TestInReadTx(t *testing.T) {
	d := migrated(t)
	ctx := t.Context()
	exec(t, d.W, `INSERT INTO tanks (name) VALUES ('first')`)
	tanks := func(tx *sql.Tx) int { return count(t, tx, `SELECT COUNT(*) FROM tanks`) }

	t.Run("reads one snapshot and rejects writes", func(t *testing.T) {
		err := d.InReadTx(ctx, func(tx *sql.Tx) error {
			if n := tanks(tx); n != 1 {
				t.Errorf("tanks = %d, want 1", n)
			}
			exec(t, d.W, `INSERT INTO tanks (name) VALUES ('concurrent')`)
			if n := tanks(tx); n != 1 {
				t.Errorf("tanks after a concurrent commit = %d, want the snapshot's 1", n)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO tanks (name) VALUES ('from tx')`); err == nil {
				t.Error("a write inside a read transaction was accepted")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if n := count(t, d.R, `SELECT COUNT(*) FROM tanks`); n != 2 {
			t.Errorf("tanks after the transaction = %d, want 2", n)
		}
	})

	t.Run("returns the error from fn", func(t *testing.T) {
		boom := errors.New("boom")
		if err := d.InReadTx(ctx, func(*sql.Tx) error { return boom }); !errors.Is(err, boom) {
			t.Errorf("err = %v, want boom", err)
		}
	})

	t.Run("a panic releases the connection", func(t *testing.T) {
		// More panics than pooled reader connections exhaust the pool if each one leaks its connection.
		short, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		for range readerConns + 1 {
			func() {
				defer func() { _ = recover() }()
				_ = d.InReadTx(short, func(*sql.Tx) error { panic("handler bug") })
			}()
		}
		if err := d.InReadTx(short, func(tx *sql.Tx) error { return tx.QueryRowContext(short, `SELECT 1`).Err() }); err != nil {
			t.Fatalf("reader pool stayed exhausted after panics: %v", err)
		}
	})

	t.Run("a canceled context fails to begin", func(t *testing.T) {
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		called := false
		if err := d.InReadTx(canceled, func(*sql.Tx) error { called = true; return nil }); err == nil || called {
			t.Errorf("err = %v, called = %v; want an error and no call", err, called)
		}
	})
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
	if n := count(t, d.R, `SELECT COUNT(*) FROM goose_db_version WHERE version_id > 0`); n != 2 {
		t.Errorf("%d versions recorded, want 2", n)
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
		return Q(tx).SetSetting(ctx, dbgen.SetSettingParams{Key: "k", Value: "in tx", UpdatedAt: "2026-01-01T00:00:00Z"})
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

	if err := q.SetSetting(ctx, dbgen.SetSettingParams{Key: "timezone", Value: "UTC", UpdatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	exec(t, d.W, `UPDATE settings SET updated_at = '2000-01-01T00:00:00Z'`)
	if err := q.SetSetting(ctx, dbgen.SetSettingParams{Key: "timezone", Value: "Europe/Berlin", UpdatedAt: "2026-01-02T00:00:00Z"}); err != nil {
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
	if updatedAt != "2026-01-02T00:00:00Z" {
		t.Errorf("updated_at = %q, want the bound timestamp", updatedAt)
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
	if v, err := c.Version(t.Context()); err != nil || v != 2 {
		t.Errorf("copy version = %d, %v; want 2", v, err)
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
	exec(t, d.W, `INSERT INTO maintenance_tasks (id, tank_id, name) VALUES (1, 1, 'task')`)
	exec(t, d.W, `INSERT INTO maintenance_completions (task_id, done_at) VALUES (1, 'x')`)
	exec(t, d.W, `INSERT INTO notification_channels (id, name, kind, config_json) VALUES (1, 'channel', 'ntfy', '{}')`)
	exec(t, d.W, `INSERT INTO notifications (id, event_type, title, source_kind, source_id, occurrence) VALUES (1, 'maintenance_due', 'title', 'maintenance_task', 1, 'x')`)
	exec(t, d.W, `INSERT INTO notification_reads (notification_id, user_id) VALUES (1, 1)`)
	exec(t, d.W, `INSERT INTO notification_deliveries (notification_id, channel_id) VALUES (1, 1)`)
	exec(t, d.W, `INSERT INTO test_reminders (tank_id, parameter_id, due_at) VALUES (1, 1, 'x')`)

	want := map[string]string{
		"settings":              "updated_at",
		"users":                 "created_at,updated_at",
		"user_totp":             "created_at",
		"user_recovery_codes":   "created_at",
		"sessions":              "created_at,last_seen_at",
		"tanks":                 "created_at,updated_at",
		"user_preferences":      "updated_at",
		"parameter_definitions": "created_at,updated_at",
		"tank_parameters":       "schedule_since,updated_at",
		"test_results":          "created_at,updated_at",
		"dashboard_cards":       "created_at",

		"maintenance_presets":     "created_at,updated_at",
		"maintenance_tasks":       "schedule_since,created_at,updated_at",
		"maintenance_completions": "created_at",
		"notification_channels":   "created_at,updated_at",
		"notifications":           "created_at",
		"notification_reads":      "read_at",
		"notification_deliveries": "created_at",
		"test_reminders":          "created_at",
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
