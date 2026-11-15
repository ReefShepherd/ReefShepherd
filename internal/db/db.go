// Package db opens the SQLite database, applies the embedded migrations and writes consistent backups.
package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver

	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

const (
	busyTimeoutMillis = 5000
	readerConns       = 4
	// backupTimeLayout is the UTC timestamp in backup file names.
	backupTimeLayout = "20060102T150405Z"
)

// DB is the two connection pools on one database file.
type DB struct {
	// W is the writer pool: a single connection, so writes never contend with each other.
	W *sql.DB
	// R is the reader pool: read-only connections that never block on the writer under WAL.
	R *sql.DB
}

// Open opens the writer and reader pools on the database file at path, creating the file if it is missing.
func Open(path string) (*DB, error) {
	w, err := openPool(dsn(path, false), 1)
	if err != nil {
		return nil, fmt.Errorf("open writer: %w", err)
	}
	// The writer is opened first because its first connection switches the file to WAL.
	r, err := openPool(dsn(path, true), readerConns)
	if err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("open reader: %w", err)
	}
	return &DB{W: w, R: r}, nil
}

func openPool(dsn string, conns int) (*sql.DB, error) {
	pool, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	pool.SetMaxOpenConns(conns)
	pool.SetMaxIdleConns(conns)
	pool.SetConnMaxLifetime(0)
	if err := pool.Ping(); err != nil {
		_ = pool.Close()
		return nil, err
	}
	return pool, nil
}

// dsn builds the connection string; the pragmas apply to every connection the pool opens.
func dsn(path string, readOnly bool) string {
	pragmas := []string{
		// First, so the journal_mode switch below also waits for a lock instead of failing.
		fmt.Sprintf("busy_timeout(%d)", busyTimeoutMillis),
		"journal_mode(WAL)",
		"synchronous(NORMAL)",
		"foreign_keys(1)",
	}
	q := url.Values{}
	if readOnly {
		pragmas = append(pragmas, "query_only(1)")
	} else {
		// Take the write lock when a transaction begins, so a read-then-write transaction cannot fail with SQLITE_BUSY midway.
		q.Set("_txlock", "immediate")
	}
	for _, p := range pragmas {
		q.Add("_pragma", p)
	}
	return fileURI(path) + "?" + q.Encode()
}

// fileURI is the SQLite URI of a file path, with characters such as ? and # escaped.
func fileURI(path string) string {
	return "file:" + (&url.URL{Path: path}).EscapedPath()
}

// Close closes both pools.
func (d *DB) Close() error {
	return errors.Join(d.R.Close(), d.W.Close())
}

// Q returns the generated queries bound to a pool or transaction.
func Q(conn dbgen.DBTX) *dbgen.Queries {
	return dbgen.New(conn)
}

// InTx runs fn in a transaction on the writer pool, committing when fn returns nil and rolling back otherwise.
func (d *DB) InTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := d.W.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	// Also covers a panic in fn, which would otherwise keep the only writer connection locked.
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	committed = true
	return nil
}

// Migrate applies the pending migrations, first writing a premigrate backup into backupDir when the database already has data.
func (d *DB) Migrate(ctx context.Context, backupDir string) error {
	fsys, err := embeddedMigrations()
	if err != nil {
		return err
	}
	return d.migrate(ctx, fsys, backupDir, time.Now())
}

func embeddedMigrations() (fs.FS, error) {
	fsys, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return nil, fmt.Errorf("open embedded migrations: %w", err)
	}
	return fsys, nil
}

func (d *DB) provider(fsys fs.FS) (*goose.Provider, error) {
	p, err := goose.NewProvider(goose.DialectSQLite3, d.W, fsys)
	if err != nil {
		return nil, fmt.Errorf("load migrations: %w", err)
	}
	return p, nil
}

func (d *DB) migrate(ctx context.Context, fsys fs.FS, backupDir string, now time.Time) error {
	p, err := d.provider(fsys)
	if err != nil {
		return err
	}
	current, target, err := p.GetVersions(ctx)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if current > target {
		return fmt.Errorf("database schema version %d is newer than this build supports (%d)", current, target)
	}
	if current > 0 && current < target {
		if err := os.MkdirAll(backupDir, 0o750); err != nil {
			return fmt.Errorf("create backup dir: %w", err)
		}
		name := fmt.Sprintf("reefshepherd-premigrate-%05d-%s.db", target, now.UTC().Format(backupTimeLayout))
		if err := d.BackupTo(ctx, filepath.Join(backupDir, name)); err != nil {
			return fmt.Errorf("premigrate backup: %w", err)
		}
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// Version returns the schema version of the database, 0 when no migration has run.
func (d *DB) Version(ctx context.Context) (int64, error) {
	fsys, err := embeddedMigrations()
	if err != nil {
		return 0, err
	}
	p, err := d.provider(fsys)
	if err != nil {
		return 0, err
	}
	v, err := p.GetDBVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return v, nil
}

// BackupTo writes a consistent copy of the database to dest, renamed into place only after an integrity check and an fsync so dest is never half written.
func (d *DB) BackupTo(ctx context.Context, dest string) error {
	tmp := dest + ".tmp"
	if err := os.Remove(tmp); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove stale backup: %w", err)
	}
	// VACUUM INTO is not parsed by sqlc, and binding the name keeps quotes in the path harmless.
	if _, err := d.W.ExecContext(ctx, "VACUUM INTO ?", tmp); err != nil {
		return fmt.Errorf("vacuum into: %w", err)
	}
	if err := checkCopy(ctx, tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	// VACUUM INTO does not fsync its output, so make it durable before it takes the final name.
	if err := syncPath(tmp); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("sync backup: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename backup: %w", err)
	}
	if err := syncPath(filepath.Dir(dest)); err != nil {
		return fmt.Errorf("sync backup directory: %w", err)
	}
	return nil
}

// syncPath flushes a file or directory to stable storage.
func syncPath(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// checkCopy opens the copy on its own connection and runs SQLite's quick integrity check.
func checkCopy(ctx context.Context, path string) error {
	c, err := sql.Open("sqlite", fileURI(path)+"?mode=ro")
	if err != nil {
		return fmt.Errorf("open backup copy: %w", err)
	}
	defer c.Close()
	var result string
	if err := c.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&result); err != nil {
		return fmt.Errorf("check backup copy: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("check backup copy: %s", result)
	}
	return nil
}
