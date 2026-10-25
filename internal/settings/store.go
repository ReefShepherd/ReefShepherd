package settings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // lets timezone names resolve in minimal containers that ship no zoneinfo

	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/units"
)

// Keys of the settings table.
const (
	keySetupCompleted      = "setup_completed"
	keyInstanceName        = "instance_name"
	keyTimezone            = "timezone"
	keyAuthMode            = "auth_mode"
	keyAuthDisabledUserID  = "auth_disabled_user_id"
	keyReminderDefaultTime = "reminder_default_time"
	// keyUnitPrefix plus a unit family gives the key of that family's display unit, such as unit_temperature.
	keyUnitPrefix = "unit_"
)

// Defaults for keys that were never set.
const (
	DefaultInstanceName = "ReefShepherd"
	DefaultTimezone     = "UTC"
	DefaultAuthMode     = config.AuthModePassword
	// DefaultReminderTime is the local time of day (HH:MM) when new reminders become due.
	DefaultReminderTime = "09:00"
)

const reminderTimeLayout = "15:04"

// ErrInvalid is wrapped by every setter that rejects its value.
var ErrInvalid = errors.New("invalid setting value")

// Store reads and writes the instance settings; a key that was never set reads as its default.
type Store struct {
	db *db.DB
}

// NewStore returns a store on the given database.
func NewStore(d *db.DB) *Store {
	return &Store{db: d}
}

// GetSetupCompleted reports whether first-run setup has finished.
func (s *Store) GetSetupCompleted(ctx context.Context) (bool, error) {
	return getParsed(ctx, s, keySetupCompleted, false, strconv.ParseBool)
}

// SetSetupCompleted records whether first-run setup has finished.
func (s *Store) SetSetupCompleted(ctx context.Context, done bool) error {
	return s.set(ctx, keySetupCompleted, strconv.FormatBool(done))
}

// GetInstanceName returns the name shown in the app header.
func (s *Store) GetInstanceName(ctx context.Context) (string, error) {
	return s.get(ctx, keyInstanceName, DefaultInstanceName)
}

// SetInstanceName stores the instance name, which must not be blank.
func (s *Store) SetInstanceName(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: instance name is blank", ErrInvalid)
	}
	return s.set(ctx, keyInstanceName, name)
}

// GetTimezone returns the IANA name of the instance timezone.
func (s *Store) GetTimezone(ctx context.Context) (string, error) {
	return s.get(ctx, keyTimezone, DefaultTimezone)
}

// SetTimezone stores the IANA name of the instance timezone.
func (s *Store) SetTimezone(ctx context.Context, name string) error {
	if _, err := time.LoadLocation(name); err != nil || name == "" || name == "Local" {
		return fmt.Errorf("%w: unknown timezone %q", ErrInvalid, name)
	}
	return s.set(ctx, keyTimezone, name)
}

// GetAuthMode returns the login mode set in the web UI, config.AuthModePassword or config.AuthModeDisabled.
func (s *Store) GetAuthMode(ctx context.Context) (string, error) {
	return getParsed(ctx, s, keyAuthMode, DefaultAuthMode, func(v string) (string, error) {
		if !validAuthMode(v) {
			return "", errors.New("unknown auth mode")
		}
		return v, nil
	})
}

// SetAuthMode stores the login mode, config.AuthModePassword or config.AuthModeDisabled.
func (s *Store) SetAuthMode(ctx context.Context, mode string) error {
	if !validAuthMode(mode) {
		return fmt.Errorf("%w: auth mode %q", ErrInvalid, mode)
	}
	return s.set(ctx, keyAuthMode, mode)
}

func validAuthMode(mode string) bool {
	return mode == config.AuthModePassword || mode == config.AuthModeDisabled
}

// GetAuthDisabledUserID returns the user that everyone acts as when login is disabled, 0 when none is set.
func (s *Store) GetAuthDisabledUserID(ctx context.Context) (int64, error) {
	return getParsed(ctx, s, keyAuthDisabledUserID, 0, func(v string) (int64, error) {
		return strconv.ParseInt(v, 10, 64)
	})
}

// SetAuthDisabledUserID stores the user that everyone acts as when login is disabled.
func (s *Store) SetAuthDisabledUserID(ctx context.Context, id int64) error {
	if id < 1 {
		return fmt.Errorf("%w: user id %d", ErrInvalid, id)
	}
	return s.set(ctx, keyAuthDisabledUserID, strconv.FormatInt(id, 10))
}

// ClearAuthDisabledUserID removes the stored user, so GetAuthDisabledUserID reads 0 again.
func (s *Store) ClearAuthDisabledUserID(ctx context.Context) error {
	if _, err := db.Q(s.db.W).DeleteSetting(ctx, keyAuthDisabledUserID); err != nil {
		return fmt.Errorf("clear setting %s: %w", keyAuthDisabledUserID, err)
	}
	return nil
}

// GetUnit returns the display unit chosen for a unit family; the default is the family's canonical unit.
func (s *Store) GetUnit(ctx context.Context, family units.Family) (units.Unit, error) {
	key, canonical, err := unitKey(family)
	if err != nil {
		return "", err
	}
	return getParsed(ctx, s, key, canonical, func(v string) (units.Unit, error) {
		if _, ok := units.Info(family, units.Unit(v)); !ok {
			return "", errors.New("unit is not in the family")
		}
		return units.Unit(v), nil
	})
}

// SetUnit stores the display unit for a unit family, which must be one of the family's units.
func (s *Store) SetUnit(ctx context.Context, family units.Family, unit units.Unit) error {
	key, _, err := unitKey(family)
	if err != nil {
		return err
	}
	if _, ok := units.Info(family, unit); !ok {
		return fmt.Errorf("%w: unit %q for %s", ErrInvalid, unit, family)
	}
	return s.set(ctx, key, string(unit))
}

// unitKey maps a family with a display unit choice to its settings key and canonical unit.
func unitKey(family units.Family) (key string, canonical units.Unit, err error) {
	info, ok := units.Lookup(family)
	if !ok || family == units.Fixed {
		return "", "", fmt.Errorf("%w: unit family %q has no display unit", ErrInvalid, family)
	}
	return keyUnitPrefix + string(family), info.Canonical, nil
}

// GetReminderDefaultTime returns the local time of day (HH:MM) when new reminders become due.
func (s *Store) GetReminderDefaultTime(ctx context.Context) (string, error) {
	return s.get(ctx, keyReminderDefaultTime, DefaultReminderTime)
}

// SetReminderDefaultTime stores the default reminder time as HH:MM on a 24-hour clock.
func (s *Store) SetReminderDefaultTime(ctx context.Context, hhmm string) error {
	if t, err := time.Parse(reminderTimeLayout, hhmm); err != nil || t.Format(reminderTimeLayout) != hhmm {
		return fmt.Errorf("%w: reminder time %q is not HH:MM", ErrInvalid, hhmm)
	}
	return s.set(ctx, keyReminderDefaultTime, hhmm)
}

func (s *Store) lookup(ctx context.Context, key string) (value string, found bool, err error) {
	v, err := db.Q(s.db.R).GetSetting(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read setting %s: %w", key, err)
	}
	return v, true, nil
}

func (s *Store) get(ctx context.Context, key, fallback string) (string, error) {
	v, found, err := s.lookup(ctx, key)
	if err != nil || !found {
		return fallback, err
	}
	return v, nil
}

func (s *Store) set(ctx context.Context, key, value string) error {
	if err := db.Q(s.db.W).SetSetting(ctx, dbgen.SetSettingParams{Key: key, Value: value}); err != nil {
		return fmt.Errorf("write setting %s: %w", key, err)
	}
	return nil
}

// getParsed reads a key and converts the stored text, so a hand-edited bad value is reported instead of ignored.
func getParsed[T any](ctx context.Context, s *Store, key string, fallback T, parse func(string) (T, error)) (T, error) {
	v, found, err := s.lookup(ctx, key)
	if err != nil || !found {
		return fallback, err
	}
	parsed, err := parse(v)
	if err != nil {
		return fallback, fmt.Errorf("setting %s has stored value %q: %w", key, v, err)
	}
	return parsed, nil
}
