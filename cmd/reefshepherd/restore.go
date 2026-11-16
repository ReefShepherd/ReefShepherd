package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/app"
	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/db"
)

const (
	// restoreBackupsDir and restoreTmpDir are the folders the server uses inside the data directory.
	restoreBackupsDir = "backups"
	restoreTmpDir     = "tmp"
	dialTimeout       = 2 * time.Second
)

func restoreUsage() string {
	return `Usage: reefshepherd restore <file> [flags]

Replaces the database with a backup file. Stop the server first.
The current database is kept in the backups folder as reefshepherd-prerestore-<time>.db.
Flags such as --data-dir and --config follow the file name.
`
}

func runRestore(ctx context.Context, args []string, env config.LookupFunc, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, restoreUsage())
		return 2
	}
	switch args[0] {
	case "help", "-h", "--help", "-help":
		fmt.Fprint(stdout, restoreUsage())
		return 0
	}
	if args[0][0] == '-' {
		fmt.Fprintf(stderr, "reefshepherd restore: missing backup file\n\n%s", restoreUsage())
		return 2
	}
	file, rest := args[0], args[1:]
	cfg, code, ok := loadConfig(rest, env, stderr)
	if !ok {
		return code
	}
	if err := restoreBackup(ctx, cfg, file, time.Now(), stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "reefshepherd restore: %v\n", err)
		return 1
	}
	return 0
}

// restoreBackup puts the backup file in place of the database, keeping the old database in the backups folder.
func restoreBackup(ctx context.Context, cfg config.Config, file string, now time.Time, stdout, stderr io.Writer) error {
	if addr, answering, err := serverAnswering(ctx, cfg); err != nil {
		return err
	} else if answering {
		return fmt.Errorf("a server is answering on %s; stop it first, for example with systemctl stop reefshepherd", addr)
	}
	version, err := db.CheckFile(ctx, file)
	if errors.Is(err, db.ErrSchemaTooNew) {
		return fmt.Errorf("%s was made by a newer version of ReefShepherd: %w", file, err)
	}
	if err != nil {
		return fmt.Errorf("%s is not a usable backup: %w", file, err)
	}

	dbPath := filepath.Join(cfg.DataDir, app.DBFileName)
	if same, err := sameFile(file, dbPath); err != nil {
		return err
	} else if same {
		return errors.New("the backup file is the current database")
	}
	tmpDir := filepath.Join(cfg.DataDir, restoreTmpDir)
	backupsDir := filepath.Join(cfg.DataDir, restoreBackupsDir)
	for _, dir := range []string{tmpDir, backupsDir} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}

	// The copy is made first, so a full disk is found before anything is moved.
	staged, err := stageCopy(file, tmpDir)
	if err != nil {
		return err
	}
	defer os.Remove(staged)

	aside, err := moveCurrentAside(ctx, dbPath, backupsDir, now, stderr)
	if err != nil {
		return err
	}
	if err := os.Rename(staged, dbPath); err != nil { //nolint:gosec // The paths come from the operator running the command.
		if aside != "" {
			_ = os.Rename(aside, dbPath)
		}
		return fmt.Errorf("put the backup in place: %w", err)
	}
	if err := syncDir(cfg.DataDir); err != nil {
		return fmt.Errorf("sync data directory: %w", err)
	}

	fmt.Fprintf(stdout, "Restored %s (schema version %d) to %s.\n", file, version, dbPath)
	if aside != "" {
		fmt.Fprintf(stdout, "The previous database is at %s.\n", aside)
	} else {
		fmt.Fprintln(stdout, "There was no previous database.")
	}
	fmt.Fprintln(stdout, "Start the server again; it applies any newer migrations on start.")
	return nil
}

// serverAnswering reports whether anything accepts connections on the configured listen address.
func serverAnswering(ctx context.Context, cfg config.Config) (addr string, answering bool, err error) {
	healthURL, err := cfg.HealthURL()
	if err != nil {
		return "", false, err
	}
	u, err := url.Parse(healthURL)
	if err != nil {
		return "", false, err
	}
	dialer := net.Dialer{Timeout: dialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return u.Host, false, nil
	}
	_ = conn.Close()
	return u.Host, true, nil
}

func sameFile(a, b string) (bool, error) {
	ia, err := os.Stat(a) //nolint:gosec // The paths come from the operator running the command.
	if err != nil {
		return false, err
	}
	ib, err := os.Stat(b)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return os.SameFile(ia, ib), nil
}

// stageCopy copies src into a new file in dir, flushed to disk, and returns its path.
func stageCopy(src, dir string) (string, error) {
	in, err := os.Open(src) //nolint:gosec // The paths come from the operator running the command.
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.CreateTemp(dir, "restore-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temporary file: %w", err)
	}
	_, copyErr := io.Copy(out, in)
	if copyErr == nil {
		copyErr = out.Sync()
	}
	closeErr := out.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		_ = os.Remove(out.Name())
		return "", fmt.Errorf("copy the backup: %w", err)
	}
	return out.Name(), nil
}

// moveCurrentAside moves the database at dbPath, with its write-ahead log files, into backupsDir as a prerestore backup and returns the new path, or "" when there was no database.
func moveCurrentAside(ctx context.Context, dbPath, backupsDir string, now time.Time, stderr io.Writer) (string, error) {
	suffixes := []string{"", "-wal", "-shm"}
	if _, err := os.Stat(dbPath); errors.Is(err, fs.ErrNotExist) {
		// A log file left without its database would be replayed into the restored one.
		for _, suffix := range suffixes[1:] {
			if err := removeIfExists(dbPath + suffix); err != nil {
				return "", err
			}
		}
		return "", nil
	} else if err != nil {
		return "", err
	}
	aside := filepath.Join(backupsDir, "reefshepherd-prerestore-"+now.UTC().Format(db.BackupTimeLayout)+".db")
	if _, err := os.Lstat(aside); err == nil {
		return "", fmt.Errorf("%s already exists; try again in a second", aside)
	}
	// The log files move with the database so they stay paired with it, then the checkpoint makes the moved file complete on its own.
	for _, suffix := range suffixes {
		if err := os.Rename(dbPath+suffix, aside+suffix); err != nil && (suffix == "" || !errors.Is(err, fs.ErrNotExist)) {
			return "", fmt.Errorf("move the current database aside: %w", err)
		}
	}
	if err := db.Checkpoint(ctx, aside); err != nil {
		fmt.Fprintf(stderr, "reefshepherd restore: could not checkpoint the previous database, so its log files stay beside it: %v\n", err)
	}
	return aside, nil
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

func syncDir(path string) error {
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
