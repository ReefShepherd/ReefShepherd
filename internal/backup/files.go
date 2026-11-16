package backup

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
)

// The kinds of backup file, told apart by their names.
const (
	KindScheduled  = "scheduled"
	KindPremigrate = "premigrate"
	KindPrerestore = "prerestore"
)

const (
	// premigrateKeep and prerestoreKeep are how many files of those kinds survive a prune.
	premigrateKeep = 5
	prerestoreKeep = 5
	// staleTempAge is how old a leftover temporary file must be before a prune removes it.
	staleTempAge = time.Hour
	tempSuffix   = ".tmp"
)

// namePattern is the only shape of file name that is listed, served or deleted, which also rules out any path trick.
var namePattern = regexp.MustCompile(`^reefshepherd-((premigrate-[0-9]{5}|prerestore)-)?[0-9]{8}T[0-9]{6}Z\.db$`)

// Backup is one backup file as the API shows it.
type Backup struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	SizeBytes int64  `json:"size_bytes"`
	// CreatedAt is the time in the file name, RFC 3339 UTC.
	CreatedAt string `json:"created_at"`

	at time.Time
}

// parseName reports the kind and creation time a backup file name carries, and false for any other name.
func parseName(name string) (kind string, at time.Time, ok bool) {
	if !namePattern.MatchString(name) {
		return "", time.Time{}, false
	}
	switch {
	case strings.HasPrefix(name, "reefshepherd-premigrate-"):
		kind = KindPremigrate
	case strings.HasPrefix(name, "reefshepherd-prerestore-"):
		kind = KindPrerestore
	default:
		kind = KindScheduled
	}
	stamp := strings.TrimSuffix(name, ".db")
	at, err := time.Parse(db.BackupTimeLayout, stamp[len(stamp)-len(db.BackupTimeLayout):])
	if err != nil {
		return "", time.Time{}, false
	}
	return kind, at, true
}

// fileName is the name of a scheduled or manual backup made at at.
func fileName(at time.Time) string {
	return "reefshepherd-" + at.UTC().Format(db.BackupTimeLayout) + ".db"
}

// list returns the backup files in dir, newest first; a missing folder holds none.
func list(dir string) ([]Backup, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read backups folder: %w", err)
	}
	var out []Backup
	for _, e := range entries {
		kind, at, ok := parseName(e.Name())
		if !ok || !e.Type().IsRegular() {
			continue
		}
		info, err := e.Info()
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("stat backup %s: %w", e.Name(), err)
		}
		out = append(out, Backup{Name: e.Name(), Kind: kind, SizeBytes: info.Size(), CreatedAt: db.FormatTime(at), at: at})
	}
	slices.SortFunc(out, func(a, b Backup) int {
		if c := b.at.Compare(a.at); c != 0 {
			return c
		}
		return strings.Compare(b.Name, a.Name)
	})
	return out, nil
}

// prune removes the backups beyond the keep counts and the temporary files left by an interrupted backup; files with any other name are never touched.
func prune(dir string, keepScheduled int, now time.Time) error {
	backups, err := list(dir)
	if err != nil {
		return err
	}
	keep := map[string]int{KindScheduled: keepScheduled, KindPremigrate: premigrateKeep, KindPrerestore: prerestoreKeep}
	seen := map[string]int{}
	var errs []error
	for _, b := range backups {
		seen[b.Kind]++
		if seen[b.Kind] > keep[b.Kind] {
			errs = append(errs, removeFile(filepath.Join(dir, b.Name)))
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return errors.Join(append(errs, fmt.Errorf("read backups folder: %w", err))...)
	}
	for _, e := range entries {
		base, isTemp := strings.CutSuffix(e.Name(), tempSuffix)
		if _, _, ok := parseName(base); !isTemp || !ok || !e.Type().IsRegular() {
			continue
		}
		info, err := e.Info()
		if err != nil || now.Sub(info.ModTime()) <= staleTempAge {
			continue
		}
		errs = append(errs, removeFile(filepath.Join(dir, e.Name())))
	}
	return errors.Join(errs...)
}

// removeFile deletes path, treating a file that is already gone as success.
func removeFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", filepath.Base(path), err)
	}
	return nil
}
