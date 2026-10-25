package settings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

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

// Validate returns a message per field of c that Apply would reject, keyed by the API field name.
func (c Changes) Validate() map[string]string {
	rejected := map[string]string{}
	add := func(field string, err error) {
		if err != nil {
			rejected[field] = strings.TrimPrefix(err.Error(), ErrInvalid.Error()+": ")
		}
	}
	if c.InstanceName != nil {
		_, err := cleanInstanceName(*c.InstanceName)
		add("instance_name", err)
	}
	if c.Timezone != nil {
		add("timezone", checkTimezone(*c.Timezone))
	}
	if c.ReminderDefaultTime != nil {
		add("reminder_default_time", checkReminderTime(*c.ReminderDefaultTime))
	}
	for family, unit := range c.Units {
		_, err := checkUnit(family, unit)
		add("units."+string(family), err)
	}
	return rejected
}

// Apply writes the changes unless some field is invalid, in which case it writes nothing and returns a message per rejected field; the map is never nil.
func (s *Store) Apply(ctx context.Context, c Changes) (rejected map[string]string, err error) {
	if rejected = c.Validate(); len(rejected) > 0 {
		return rejected, nil
	}
	if c.InstanceName != nil {
		if err := s.SetInstanceName(ctx, *c.InstanceName); err != nil {
			return nil, err
		}
	}
	if c.Timezone != nil {
		if err := s.SetTimezone(ctx, *c.Timezone); err != nil {
			return nil, err
		}
	}
	if c.ReminderDefaultTime != nil {
		if err := s.SetReminderDefaultTime(ctx, *c.ReminderDefaultTime); err != nil {
			return nil, err
		}
	}
	for family, unit := range c.Units {
		if err := s.SetUnit(ctx, family, unit); err != nil {
			return nil, err
		}
	}
	return rejected, nil
}

// SwitchAuthMode changes the login mode and keeps the user that everyone acts as in step with it, refusing a change that would leave nobody able to sign in; the error wraps ErrInvalid.
func (s *Store) SwitchAuthMode(ctx context.Context, mode string) error {
	switch mode {
	case config.AuthModeDisabled:
		id, err := db.Q(s.r).FirstEnabledAdminID(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: there is no enabled admin to act as", ErrInvalid)
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
			return fmt.Errorf("%w: set a password for an admin first", ErrInvalid)
		}
		if err := s.SetAuthMode(ctx, mode); err != nil {
			return err
		}
		return s.ClearAuthDisabledUserID(ctx)
	}
	return fmt.Errorf("%w: auth mode %q", ErrInvalid, mode)
}
