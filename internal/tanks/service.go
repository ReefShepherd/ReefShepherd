package tanks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/schedule"
	"github.com/reefshepherd/reefshepherd/internal/units"
)

const (
	defaultType  = "reef"
	maxNameRunes = 100
	maxNoteRunes = 10000
	// maxVolumeLiters is far beyond any aquarium and only exists to catch typos such as an extra digit group.
	maxVolumeLiters = 1_000_000
)

// tankTypes are the allowed values of a tank's type.
var tankTypes = []string{"reef", "fowlr", "frag", "quarantine", "other"}

// ErrNotFound is the error for a tank that does not exist.
var ErrNotFound = httpx.NotFound("no such tank")

// Tank is a tank as the API shows it, with the volume in the instance's display unit.
type Tank struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	TankType   string   `json:"tank_type"`
	Volume     *float64 `json:"volume"`
	VolumeUnit string   `json:"volume_unit"`
	StartedOn  *string  `json:"started_on"`
	Note       string   `json:"note"`
	SortOrder  int64    `json:"sort_order"`
	ArchivedAt *string  `json:"archived_at"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

// Service is the tank logic behind the HTTP handlers; its errors are *httpx.APIError when the client caused them.
type Service struct {
	Deps
	now func() time.Time
}

// NewService returns the service for deps.
func NewService(deps Deps) *Service {
	return &Service{Deps: deps, now: time.Now}
}

// volumeUnit returns the display unit tank volumes are shown and entered in.
func (s *Service) volumeUnit(ctx context.Context) (units.Unit, error) {
	u, err := s.Settings.GetUnit(ctx, units.Volume)
	if err != nil {
		return "", fmt.Errorf("read volume unit: %w", err)
	}
	return u, nil
}

// view converts a stored tank to its API form.
func view(row dbgen.Tank, unit units.Unit) Tank {
	t := Tank{
		ID: row.ID, Name: row.Name, TankType: row.TankType, VolumeUnit: string(unit), StartedOn: row.StartedOn,
		Note: row.Note, SortOrder: row.SortOrder, ArchivedAt: row.ArchivedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if row.VolumeLiters != nil {
		v := units.Round(units.FromCanonical(units.Volume, unit, *row.VolumeLiters), units.Precision(units.Volume, unit))
		t.Volume = &v
	}
	return t
}

// List returns the tanks in display order, archived ones last and only when asked for.
func (s *Service) List(ctx context.Context, includeArchived bool) ([]Tank, error) {
	unit, err := s.volumeUnit(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.Q(s.DB.R).ListTanks(ctx, db.Flag(includeArchived))
	if err != nil {
		return nil, fmt.Errorf("list tanks: %w", err)
	}
	out := make([]Tank, len(rows))
	for i, row := range rows {
		out[i] = view(row, unit)
	}
	return out, nil
}

// Require returns a not-found error when no tank has the id, archived or not.
func Require(ctx context.Context, conn dbgen.DBTX, id int64) error {
	n, err := db.Q(conn).TankExists(ctx, id)
	if err != nil {
		return fmt.Errorf("check tank: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Get returns one tank, archived or not.
func (s *Service) Get(ctx context.Context, id int64) (Tank, error) {
	unit, err := s.volumeUnit(ctx)
	if err != nil {
		return Tank{}, err
	}
	row, err := db.Q(s.DB.R).GetTank(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Tank{}, ErrNotFound
	}
	if err != nil {
		return Tank{}, fmt.Errorf("read tank: %w", err)
	}
	return view(row, unit), nil
}

// CreateInput is a new tank, with the volume in the display unit and the presets to switch on for it.
type CreateInput struct {
	Name      string
	TankType  string
	Volume    *float64
	StartedOn *string
	Note      string
	// Presets are the keys of the parameter presets to enable, copying each one's default range and test interval.
	Presets []string
}

// Create adds a tank and enables the chosen parameter presets on it in one transaction.
func (s *Service) Create(ctx context.Context, in CreateInput) (Tank, error) {
	unit, err := s.volumeUnit(ctx)
	if err != nil {
		return Tank{}, err
	}
	fields := map[string]string{}
	name, err := cleanName(in.Name)
	if err != nil {
		fields["name"] = err.Error()
	}
	tankType := strings.TrimSpace(in.TankType)
	if tankType == "" {
		tankType = defaultType
	}
	if !slices.Contains(tankTypes, tankType) {
		fields["tank_type"] = "must be one of " + strings.Join(tankTypes, ", ")
	}
	liters, err := cleanVolume(unit, in.Volume)
	if err != nil {
		fields["volume"] = err.Error()
	}
	startedOn, err := cleanStartedOn(in.StartedOn)
	if err != nil {
		fields["started_on"] = err.Error()
	}
	note, err := cleanNote(in.Note)
	if err != nil {
		fields["note"] = err.Error()
	}

	var created dbgen.Tank
	err = s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		keys, err := s.checkPresets(ctx, q, in.Presets, fields)
		if err != nil {
			return err
		}
		if len(fields) > 0 {
			return httpx.Validation("invalid tank", fields)
		}
		now := db.FormatTime(s.now())
		created, err = q.CreateTank(ctx, dbgen.CreateTankParams{
			Name: name, TankType: tankType, VolumeLiters: liters, StartedOn: startedOn, Note: note, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("create tank: %w", err)
		}
		if len(keys) == 0 {
			return nil
		}
		if _, err := q.EnablePresetParameters(ctx, dbgen.EnablePresetParametersParams{TankID: created.ID, ScheduleSince: now, UpdatedAt: now, PresetKeys: keys}); err != nil {
			return fmt.Errorf("enable presets: %w", err)
		}
		return nil
	})
	if err != nil {
		return Tank{}, err
	}
	return view(created, unit), nil
}

// checkPresets reports unknown or retired preset keys in fields and returns the valid keys for the insert, where repeats are harmless.
func (s *Service) checkPresets(ctx context.Context, q *dbgen.Queries, requested []string, fields map[string]string) ([]*string, error) {
	if len(requested) == 0 {
		return nil, nil
	}
	known, err := q.ListActivePresetKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("list presets: %w", err)
	}
	var keys []*string
	var unknown []string
	for _, key := range requested {
		if slices.Contains(known, key) {
			keys = append(keys, &key)
		} else {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		fields["presets"] = "unknown preset: " + strings.Join(unknown, ", ")
	}
	return keys, nil
}

// UpdateInput changes the fields that are set; a nil pointer or unset Nullable leaves the field alone.
type UpdateInput struct {
	Name      *string
	TankType  *string
	Volume    httpx.Nullable[float64]
	StartedOn httpx.Nullable[string]
	Note      *string
	SortOrder *int64
}

// Update changes a tank, archived or not.
func (s *Service) Update(ctx context.Context, id int64, in UpdateInput) (Tank, error) {
	unit, err := s.volumeUnit(ctx)
	if err != nil {
		return Tank{}, err
	}
	var updated dbgen.Tank
	err = s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		cur, err := q.GetTank(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read tank: %w", err)
		}
		fields := map[string]string{}
		p := dbgen.UpdateTankParams{
			ID: id, Name: cur.Name, TankType: cur.TankType, VolumeLiters: cur.VolumeLiters,
			StartedOn: cur.StartedOn, Note: cur.Note, SortOrder: cur.SortOrder, UpdatedAt: db.FormatTime(s.now()),
		}
		if in.Name != nil {
			if p.Name, err = cleanName(*in.Name); err != nil {
				fields["name"] = err.Error()
			}
		}
		if in.TankType != nil {
			p.TankType = strings.TrimSpace(*in.TankType)
			if !slices.Contains(tankTypes, p.TankType) {
				fields["tank_type"] = "must be one of " + strings.Join(tankTypes, ", ")
			}
		}
		if in.Volume.Set {
			if p.VolumeLiters, err = cleanVolume(unit, in.Volume.Value); err != nil {
				fields["volume"] = err.Error()
			}
		}
		if in.StartedOn.Set {
			if p.StartedOn, err = cleanStartedOn(in.StartedOn.Value); err != nil {
				fields["started_on"] = err.Error()
			}
		}
		if in.Note != nil {
			if p.Note, err = cleanNote(*in.Note); err != nil {
				fields["note"] = err.Error()
			}
		}
		if in.SortOrder != nil {
			p.SortOrder = *in.SortOrder
		}
		if len(fields) > 0 {
			return httpx.Validation("invalid tank", fields)
		}
		if updated, err = q.UpdateTank(ctx, p); err != nil {
			return fmt.Errorf("update tank: %w", err)
		}
		return nil
	})
	if err != nil {
		return Tank{}, err
	}
	return view(updated, unit), nil
}

// Archive hides a tank from the default list; archiving an archived tank changes nothing but the edit time.
func (s *Service) Archive(ctx context.Context, id int64) (Tank, error) {
	return s.setArchived(ctx, id, true)
}

// Unarchive brings a tank back into the default list.
func (s *Service) Unarchive(ctx context.Context, id int64) (Tank, error) {
	return s.setArchived(ctx, id, false)
}

func (s *Service) setArchived(ctx context.Context, id int64, archived bool) (Tank, error) {
	unit, err := s.volumeUnit(ctx)
	if err != nil {
		return Tank{}, err
	}
	now := db.FormatTime(s.now())
	var row dbgen.Tank
	err = s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		if archived {
			row, err = q.ArchiveTank(ctx, dbgen.ArchiveTankParams{ArchivedAt: &now, UpdatedAt: now, ID: id})
		} else {
			row, err = q.UnarchiveTank(ctx, dbgen.UnarchiveTankParams{UpdatedAt: now, ID: id})
		}
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		return Tank{}, ErrNotFound
	}
	if err != nil {
		return Tank{}, fmt.Errorf("set tank archived: %w", err)
	}
	return view(row, unit), nil
}

// Delete removes a tank and everything that belongs to it, but only when confirm is exactly the tank's name.
func (s *Service) Delete(ctx context.Context, id int64, confirm string) error {
	return s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		cur, err := q.GetTank(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read tank: %w", err)
		}
		if confirm != cur.Name {
			return httpx.Validation("confirmation does not match", map[string]string{"confirm": "must be the tank's name"})
		}
		if _, err := q.DeleteTank(ctx, id); err != nil {
			return fmt.Errorf("delete tank: %w", err)
		}
		return nil
	})
}

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("is required")
	}
	if utf8.RuneCountInString(name) > maxNameRunes {
		return "", fmt.Errorf("must be at most %d characters", maxNameRunes)
	}
	return name, nil
}

func cleanNote(note string) (string, error) {
	if utf8.RuneCountInString(note) > maxNoteRunes {
		return "", fmt.Errorf("must be at most %d characters", maxNoteRunes)
	}
	return note, nil
}

// cleanVolume converts a volume entered in the display unit to canonical liters, nil meaning no volume.
func cleanVolume(unit units.Unit, v *float64) (*float64, error) {
	if v == nil {
		return nil, nil
	}
	liters, err := units.ToCanonical(units.Volume, unit, *v)
	if err != nil || !(liters > 0) || liters > maxVolumeLiters {
		return nil, errors.New("must be a positive, realistic tank volume")
	}
	return &liters, nil
}

// cleanStartedOn accepts a YYYY-MM-DD calendar date, treating blank as no date.
func cleanStartedOn(v *string) (*string, error) {
	if v == nil {
		return nil, nil
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return nil, nil
	}
	d, err := schedule.ParseDate(s)
	if err != nil || d.String() != s {
		return nil, errors.New("must be a date as YYYY-MM-DD")
	}
	return &s, nil
}
