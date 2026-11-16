package settings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/units"
)

// Deps are the dependencies the settings service needs.
type Deps struct {
	DB       *db.DB
	Settings *Store
	Auth     *auth.Service
	Logger   *slog.Logger
}

// Service is the settings logic behind the HTTP handlers; the errors it returns for a client mistake are field errors and auth.ErrEnvLocked.
type Service struct {
	Deps
}

// NewService returns the service for deps.
func NewService(deps Deps) *Service {
	return &Service{Deps: deps}
}

// Snapshot is the settings as the API shows them.
type Snapshot struct {
	InstanceName        string            `json:"instance_name"`
	Timezone            string            `json:"timezone"`
	ReminderDefaultTime string            `json:"reminder_default_time"`
	BackupEnabled       bool              `json:"backup_enabled"`
	BackupTime          string            `json:"backup_time"`
	BackupKeep          int               `json:"backup_keep"`
	Units               map[string]string `json:"units"`
	// AuthMode is the mode in force, which an environment override may set.
	AuthMode string `json:"auth_mode"`
	// EnvLocked maps each setting the server environment decides to the variable that does.
	EnvLocked map[string]string `json:"env_locked"`
}

// PatchInput are the changes to make; nil or empty fields are left alone.
type PatchInput struct {
	InstanceName        *string
	Timezone            *string
	ReminderDefaultTime *string
	BackupEnabled       *bool
	BackupTime          *string
	BackupKeep          *int
	Units               map[string]string
	AuthMode            *string
	// Password is the admin's own password, required to turn login off.
	Password string
}

// Get returns the settings with the login mode in force.
func (s *Service) Get(ctx context.Context) (Snapshot, error) {
	v, err := s.Settings.GetValues(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	mode, err := s.Auth.EffectiveMode(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	out := Snapshot{
		InstanceName: v.InstanceName, Timezone: v.Timezone, ReminderDefaultTime: v.ReminderDefaultTime,
		BackupEnabled: v.BackupEnabled, BackupTime: v.BackupTime, BackupKeep: v.BackupKeep,
		Units: UnitsToStrings(v.Units), AuthMode: mode, EnvLocked: map[string]string{},
	}
	if s.Auth.ModeLocked() {
		out.EnvLocked["auth_mode"] = config.EnvAuthMode
	}
	return out, nil
}

// Patch applies the changes all together or not at all and returns the settings afterwards; r is the request the admin's password check counts against, and a login mode the environment decides is refused with auth.ErrEnvLocked.
func (s *Service) Patch(ctx context.Context, r *http.Request, adminID int64, in PatchInput) (Snapshot, error) {
	changes := Changes{
		InstanceName: in.InstanceName, Timezone: in.Timezone, ReminderDefaultTime: in.ReminderDefaultTime,
		BackupEnabled: in.BackupEnabled, BackupTime: in.BackupTime, BackupKeep: in.BackupKeep,
		Units: UnitsFromStrings(in.Units),
	}
	newMode := ""
	if in.AuthMode != nil {
		effective, err := s.Auth.EffectiveMode(ctx)
		if err != nil {
			return Snapshot{}, err
		}
		if *in.AuthMode != effective {
			if s.Auth.ModeLocked() {
				return Snapshot{}, auth.ErrEnvLocked
			}
			newMode = *in.AuthMode
		}
	}
	if newMode == config.AuthModeDisabled {
		// Turning login off hands the app to anyone who can reach it, so the admin has to prove who they are first.
		if err := s.Auth.ConfirmPassword(ctx, r, adminID, in.Password, "password"); err != nil {
			return Snapshot{}, err
		}
	}
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		return s.Settings.WithTx(tx).Update(ctx, changes, newMode, "invalid settings")
	})
	if err != nil {
		return Snapshot{}, err
	}
	return s.Get(ctx)
}

// UnitsToStrings converts the display units per family to the form the API uses.
func UnitsToStrings(m map[units.Family]units.Unit) map[string]string {
	out := make(map[string]string, len(m))
	for family, unit := range m {
		out[string(family)] = string(unit)
	}
	return out
}

// UnitsFromStrings converts the API form of the display units per family; names are not checked, which Changes.Validate does.
func UnitsFromStrings(m map[string]string) map[units.Family]units.Unit {
	out := make(map[units.Family]units.Unit, len(m))
	for family, unit := range m {
		out[units.Family(family)] = units.Unit(unit)
	}
	return out
}

// Values is every instance setting with defaults filled in.
type Values struct {
	InstanceName        string
	Timezone            string
	ReminderDefaultTime string
	BackupEnabled       bool
	BackupTime          string
	BackupKeep          int
	// Units maps each unit family that has a display unit to the chosen unit.
	Units map[units.Family]units.Unit
	// AuthMode is the mode stored in the database, which an environment override may shadow.
	AuthMode string
}

// Changes are the settings to write; nil fields and absent map entries are left alone.
type Changes struct {
	InstanceName        *string
	Timezone            *string
	ReminderDefaultTime *string
	BackupEnabled       *bool
	BackupTime          *string
	BackupKeep          *int
	Units               map[units.Family]units.Unit
}

// DisplayFamilies lists the unit families whose display unit is a setting.
func DisplayFamilies() []units.Family {
	var out []units.Family
	for _, f := range units.Catalog() {
		if f.Family != units.Fixed {
			out = append(out, f.Family)
		}
	}
	return out
}

// GetValues reads every setting.
func (s *Store) GetValues(ctx context.Context) (Values, error) {
	var (
		v   Values
		err error
	)
	if v.InstanceName, err = s.GetInstanceName(ctx); err != nil {
		return Values{}, err
	}
	if v.Timezone, err = s.GetTimezone(ctx); err != nil {
		return Values{}, err
	}
	if v.ReminderDefaultTime, err = s.GetReminderDefaultTime(ctx); err != nil {
		return Values{}, err
	}
	if v.BackupEnabled, err = s.GetBackupEnabled(ctx); err != nil {
		return Values{}, err
	}
	if v.BackupTime, err = s.GetBackupTime(ctx); err != nil {
		return Values{}, err
	}
	if v.BackupKeep, err = s.GetBackupKeep(ctx); err != nil {
		return Values{}, err
	}
	if v.AuthMode, err = s.GetAuthMode(ctx); err != nil {
		return Values{}, err
	}
	v.Units = map[units.Family]units.Unit{}
	for _, f := range DisplayFamilies() {
		if v.Units[f], err = s.GetUnit(ctx, f); err != nil {
			return Values{}, err
		}
	}
	return v, nil
}

// Validate returns the rejections Apply would make, joined into one error that is nil when every field is acceptable.
func (c Changes) Validate() error {
	var rejected []error
	if c.InstanceName != nil {
		_, err := cleanInstanceName(*c.InstanceName)
		rejected = append(rejected, err)
	}
	if c.Timezone != nil {
		rejected = append(rejected, checkTimezone(*c.Timezone))
	}
	if c.ReminderDefaultTime != nil {
		rejected = append(rejected, checkReminderTime(*c.ReminderDefaultTime))
	}
	if c.BackupTime != nil {
		rejected = append(rejected, checkBackupTime(*c.BackupTime))
	}
	if c.BackupKeep != nil {
		rejected = append(rejected, checkBackupKeep(*c.BackupKeep))
	}
	for family, unit := range c.Units {
		_, err := checkUnit(family, unit)
		rejected = append(rejected, err)
	}
	return errors.Join(rejected...)
}

// Apply writes the changes unless some field is invalid, in which case it writes nothing and returns Validate's error.
func (s *Store) Apply(ctx context.Context, c Changes) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.InstanceName != nil {
		if err := s.SetInstanceName(ctx, *c.InstanceName); err != nil {
			return err
		}
	}
	if c.Timezone != nil {
		if err := s.SetTimezone(ctx, *c.Timezone); err != nil {
			return err
		}
	}
	if c.ReminderDefaultTime != nil {
		if err := s.SetReminderDefaultTime(ctx, *c.ReminderDefaultTime); err != nil {
			return err
		}
	}
	if c.BackupEnabled != nil {
		if err := s.SetBackupEnabled(ctx, *c.BackupEnabled); err != nil {
			return err
		}
	}
	if c.BackupTime != nil {
		if err := s.SetBackupTime(ctx, *c.BackupTime); err != nil {
			return err
		}
	}
	if c.BackupKeep != nil {
		if err := s.SetBackupKeep(ctx, *c.BackupKeep); err != nil {
			return err
		}
	}
	for family, unit := range c.Units {
		if err := s.SetUnit(ctx, family, unit); err != nil {
			return err
		}
	}
	return nil
}

// Update applies the changes and, when mode is not empty, switches the login mode; every field either step rejects is reported together as one error with the summary message.
func (s *Store) Update(ctx context.Context, c Changes, mode, message string) error {
	rejected := httpx.FieldErrors{Message: message}
	if err := s.Apply(ctx, c); err != nil && !rejected.AddFrom(err) {
		return err
	}
	if mode != "" {
		if err := s.SwitchAuthMode(ctx, mode); err != nil && !rejected.AddFrom(err) {
			return err
		}
	}
	return rejected.Err()
}

// SwitchAuthMode changes the login mode and keeps the user that everyone acts as in step with it, refusing a change that would leave nobody able to sign in; the error is an ErrInvalid for the auth_mode field.
func (s *Store) SwitchAuthMode(ctx context.Context, mode string) error {
	switch mode {
	case config.AuthModeDisabled:
		id, err := db.Q(s.r).FirstEnabledAdminID(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			return reject("auth_mode", codeInvalid, "there is no enabled admin to act as")
		}
		if err != nil {
			return fmt.Errorf("find first admin: %w", err)
		}
		// The user goes first so a failure in between never leaves login off with nobody to act as.
		if err := s.SetAuthDisabledUserID(ctx, id); err != nil {
			return err
		}
		return s.SetAuthMode(ctx, mode)
	case config.AuthModePassword:
		n, err := db.Q(s.r).CountEnabledAdminsWithPassword(ctx)
		if err != nil {
			return fmt.Errorf("count admins with a password: %w", err)
		}
		if n == 0 {
			return reject("auth_mode", codeInvalid, "set a password for an admin first")
		}
		if err := s.SetAuthMode(ctx, mode); err != nil {
			return err
		}
		return s.ClearAuthDisabledUserID(ctx)
	}
	return reject("auth_mode", codeInvalid, "auth mode %q", mode)
}
