package backup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
)

// Errors the service returns for a client mistake.
var (
	ErrNotFound  = errors.New("no such backup")
	ErrNameTaken = errors.New("a backup was already made in this second")
)

// Service creates, lists, serves and prunes backups.
type Service struct {
	Deps
	now func() time.Time
}

// NewService returns the service for deps.
func NewService(deps Deps) *Service {
	return &Service{Deps: deps, now: time.Now}
}

// List returns the backups, newest first.
func (s *Service) List() ([]Backup, error) {
	return list(s.Dir)
}

// Create writes a backup named for the current time, counted with the scheduled ones, and prunes; a failed prune is logged and does not fail the backup.
func (s *Service) Create(ctx context.Context) (Backup, error) {
	return s.create(ctx, s.now())
}

func (s *Service) create(ctx context.Context, at time.Time) (Backup, error) {
	at = at.UTC().Truncate(time.Second)
	if err := os.MkdirAll(s.Dir, 0o750); err != nil {
		return Backup{}, fmt.Errorf("create backups folder: %w", err)
	}
	name := fileName(at)
	dest := filepath.Join(s.Dir, name)
	if _, err := os.Lstat(dest); err == nil {
		return Backup{}, ErrNameTaken
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Backup{}, fmt.Errorf("check backup name: %w", err)
	}
	if err := s.DB.BackupTo(ctx, dest); err != nil {
		return Backup{}, err
	}
	info, err := os.Stat(dest)
	if err != nil {
		return Backup{}, fmt.Errorf("stat backup: %w", err)
	}
	if err := s.prune(ctx, at); err != nil {
		s.Logger.Warn("prune backups failed", "error", err)
	}
	return Backup{Name: name, Kind: KindScheduled, SizeBytes: info.Size(), CreatedAt: db.FormatTime(at), at: at}, nil
}

// prune applies the keep counts, reading the number of scheduled backups to keep from the settings.
func (s *Service) prune(ctx context.Context, now time.Time) error {
	keep, err := s.Settings.GetBackupKeep(ctx)
	if err != nil {
		return err
	}
	return prune(s.Dir, keep, now)
}

// Open opens a backup file for download; any name that is not a backup file name is reported as not found.
func (s *Service) Open(name string) (*os.File, Backup, error) {
	root, err := s.root()
	if err != nil {
		return nil, Backup{}, err
	}
	defer root.Close()
	b, err := resolve(root, name)
	if err != nil {
		return nil, Backup{}, err
	}
	f, err := root.Open(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, Backup{}, ErrNotFound
	}
	if err != nil {
		return nil, Backup{}, fmt.Errorf("open backup: %w", err)
	}
	return f, b, nil
}

// Delete removes a backup file.
func (s *Service) Delete(name string) error {
	root, err := s.root()
	if err != nil {
		return err
	}
	defer root.Close()
	if _, err := resolve(root, name); err != nil {
		return err
	}
	if err := root.Remove(name); errors.Is(err, fs.ErrNotExist) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("delete backup: %w", err)
	}
	return nil
}

// root opens the backups folder as a root that cannot be escaped; a missing folder holds no backup.
func (s *Service) root() (*os.Root, error) {
	root, err := os.OpenRoot(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("open backups folder: %w", err)
	}
	return root, nil
}

// resolve describes the backup file called name, which must match the backup name pattern and be a regular file in the folder.
func resolve(root *os.Root, name string) (Backup, error) {
	kind, at, ok := parseName(name)
	if !ok {
		return Backup{}, ErrNotFound
	}
	info, err := root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && !info.Mode().IsRegular()) {
		return Backup{}, ErrNotFound
	}
	if err != nil {
		return Backup{}, fmt.Errorf("stat backup: %w", err)
	}
	return Backup{Name: name, Kind: kind, SizeBytes: info.Size(), CreatedAt: db.FormatTime(at), at: at}, nil
}
