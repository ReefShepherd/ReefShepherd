package settings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/units"
)

// Values is every instance setting with defaults filled in.
type Values struct {
	InstanceName        string
	Timezone            string
	ReminderDefaultTime string
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

// Values reads every setting.
func (s *Store) Values(ctx context.Context) (Values, error) {
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
	for family, unit := range c.Units {
		if err := s.SetUnit(ctx, family, unit); err != nil {
			return err
		}
	}
	return nil
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
