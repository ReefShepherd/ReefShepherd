package users

import (
	"cmp"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/auth/password"
	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/settings"
	"github.com/reefshepherd/reefshepherd/internal/units"
)

// defaultAdminName is the account made when setup chooses to run without login.
const defaultAdminName = "admin"

type setupRequest struct {
	InstanceName *string           `json:"instance_name"`
	Timezone     *string           `json:"timezone"`
	Units        map[string]string `json:"units"`
	Theme        string            `json:"theme"`
	AuthMode     string            `json:"auth_mode"`
	Username     string            `json:"username"`
	DisplayName  string            `json:"display_name"`
	Password     string            `json:"password"`
}

var errSetupDone = httpx.Conflict("setup has already been completed")

// setup creates the first admin and the instance settings; only the first caller succeeds.
func (s *Service) setup(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var req setupRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	// Refuse early so a closed instance never spends time hashing for strangers.
	if required, err := s.Auth.SetupRequired(ctx); err != nil {
		return err
	} else if !required {
		return errSetupDone
	}

	mode, fields, err := s.validateSetup(&req)
	if err != nil {
		return err
	}
	changes := settings.Changes{InstanceName: req.InstanceName, Timezone: req.Timezone, Units: map[units.Family]units.Unit{}}
	for family, unit := range req.Units {
		changes.Units[units.Family(family)] = units.Unit(unit)
	}
	maps.Copy(fields, changes.Validate())
	if len(fields) > 0 {
		return httpx.Validation("invalid setup", fields)
	}
	var hash *string
	if req.Password != "" {
		h, err := s.Auth.Passwords.Hash(ctx, req.Password)
		if err != nil {
			return err
		}
		hash = &h
	}

	var id int64
	err = s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		store := s.Settings.WithTx(tx)
		done, err := store.GetSetupCompleted(ctx)
		if err != nil {
			return err
		}
		n, err := q.CountUsers(ctx)
		if err != nil {
			return fmt.Errorf("count users: %w", err)
		}
		if done || n > 0 {
			return errSetupDone
		}
		now := db.FormatTime(s.now())
		in := CreateInput{Username: req.Username, DisplayName: req.DisplayName, Role: httpx.RoleAdmin}
		if id, err = createAccount(ctx, q, in, hash, now); err != nil {
			return err
		}
		err = q.UpsertPreferences(ctx, dbgen.UpsertPreferencesParams{UserID: id, Theme: req.Theme, Locale: "en", UpdatedAt: now})
		if err != nil {
			return fmt.Errorf("store preferences: %w", err)
		}
		rejected, err := store.Apply(ctx, changes)
		if err != nil {
			return err
		}
		if len(rejected) > 0 {
			return httpx.Validation("invalid setup", rejected)
		}
		if err := store.SwitchAuthMode(ctx, mode); err != nil {
			return setupModeError(err)
		}
		return store.SetSetupCompleted(ctx, true)
	})
	if err != nil {
		return err
	}

	if hash != nil {
		// The admin who just chose a password is signed in, so setup flows straight into the app.
		if err := s.Auth.StartSession(w, r, id, auth.MethodPassword); err != nil {
			return err
		}
	}
	me, err := s.Me(ctx, id)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusCreated, me)
	return nil
}

// validateSetup fills in defaults, returns the login mode to store and the problems with the request.
func (s *Service) validateSetup(req *setupRequest) (mode string, fields map[string]string, err error) {
	fields = map[string]string{}
	locked := s.Auth.Config.AuthMode
	mode = req.AuthMode
	if mode == "" {
		mode = cmp.Or(locked, config.AuthModePassword)
	}
	switch {
	case mode != config.AuthModePassword && mode != config.AuthModeDisabled:
		fields["auth_mode"] = "must be password or disabled"
	case locked != "" && mode != locked:
		return "", nil, &httpx.APIError{
			Status:  http.StatusConflict,
			Code:    "env_locked",
			Message: "the login mode is set by " + config.EnvAuthMode + " on the server",
			Fields:  map[string]string{"auth_mode": "set by " + config.EnvAuthMode + " on the server"},
		}
	}

	if req.Theme == "" {
		req.Theme = "system"
	}
	if !slices.Contains(themes, req.Theme) {
		fields["theme"] = "must be one of " + strings.Join(themes, ", ")
	}

	req.Username = strings.TrimSpace(req.Username)
	if mode == config.AuthModeDisabled && req.Username == "" {
		req.Username = defaultAdminName
	}
	probe := CreateInput{Username: req.Username, DisplayName: req.DisplayName, Role: httpx.RoleAdmin}
	for k, v := range validateNewAccount(&probe) {
		fields[k] = v
	}
	req.DisplayName = probe.DisplayName

	switch {
	case req.Password != "":
		if err := password.Validate(req.Password); err != nil {
			fields["password"] = err.Error()
		}
	case mode == config.AuthModePassword:
		fields["password"] = "required"
	}
	return mode, fields, nil
}

// setupModeError turns the settings refusal to switch the mode into a field message.
func setupModeError(err error) error {
	if !errors.Is(err, settings.ErrInvalid) {
		return err
	}
	return httpx.Validation("invalid setup", map[string]string{"auth_mode": strings.TrimPrefix(err.Error(), settings.ErrInvalid.Error()+": ")})
}
