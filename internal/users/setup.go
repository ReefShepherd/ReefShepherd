package users

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/settings"
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

// setup signs the first admin in when they chose a password, so setup flows straight into the app.
func (s *Service) setup(w http.ResponseWriter, r *http.Request) error {
	var req setupRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	me, err := s.Setup(r.Context(), SetupInput(req))
	if err != nil {
		return err
	}
	if me.HasPassword {
		if err := s.Auth.StartSession(w, r, me.ID, auth.MethodPassword); err != nil {
			return err
		}
	}
	httpx.WriteJSON(w, http.StatusCreated, me)
	return nil
}

// SetupInput is what first-run setup is given; empty fields take their defaults.
type SetupInput struct {
	InstanceName *string
	Timezone     *string
	Units        map[string]string
	Theme        string
	AuthMode     string
	Username     string
	DisplayName  string
	Password     string
}

// Setup creates the first admin and the instance settings, refusing every later call with errSetupDone, and returns the new admin.
func (s *Service) Setup(ctx context.Context, in SetupInput) (Me, error) {
	// Refuse early so a closed instance never spends time hashing for strangers.
	if required, err := s.Auth.SetupRequired(ctx); err != nil {
		return Me{}, err
	} else if !required {
		return Me{}, errSetupDone
	}

	fields := httpx.FieldErrors{Message: "invalid setup"}
	mode, err := s.validateSetup(&in, &fields)
	if err != nil {
		return Me{}, err
	}
	changes := settings.Changes{InstanceName: in.InstanceName, Timezone: in.Timezone, Units: settings.UnitsFromStrings(in.Units)}
	fields.AddFrom(changes.Validate())
	if err := fields.Err(); err != nil {
		return Me{}, err
	}
	var hash *string
	if in.Password != "" {
		h, err := s.Auth.Passwords.Hash(ctx, in.Password)
		if err != nil {
			return Me{}, err
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
		admin := CreateInput{Username: in.Username, DisplayName: in.DisplayName, Role: httpx.RoleAdmin}
		if id, err = createAccount(ctx, q, admin, hash, now); err != nil {
			return err
		}
		err = q.UpsertPreferences(ctx, dbgen.UpsertPreferencesParams{UserID: id, Theme: in.Theme, Locale: "en", UpdatedAt: now})
		if err != nil {
			return fmt.Errorf("store preferences: %w", err)
		}
		if err := store.Update(ctx, changes, mode, "invalid setup"); err != nil {
			return err
		}
		return store.SetSetupCompleted(ctx, true)
	})
	if err != nil {
		return Me{}, err
	}
	return s.Me(ctx, id)
}

// validateSetup fills in defaults, returns the login mode to store and reports the problems with the request.
func (s *Service) validateSetup(in *SetupInput, fields *httpx.FieldErrors) (mode string, err error) {
	mode = cmp.Or(in.AuthMode, s.Auth.Config.AuthMode, config.AuthModePassword)
	switch {
	case mode != config.AuthModePassword && mode != config.AuthModeDisabled:
		fields.Add("auth_mode", httpx.CodeInvalid)
	case s.Auth.ModeLocked() && mode != s.Auth.Config.AuthMode:
		return "", auth.ErrEnvLocked
	}

	if in.Theme == "" {
		in.Theme = "system"
	}
	if !slices.Contains(themes, in.Theme) {
		fields.Add("theme", httpx.CodeInvalid)
	}

	in.Username = strings.TrimSpace(in.Username)
	if mode == config.AuthModeDisabled && in.Username == "" {
		in.Username = defaultAdminName
	}
	probe := CreateInput{Username: in.Username, DisplayName: in.DisplayName, Role: httpx.RoleAdmin}
	validateNewAccount(&probe, fields)
	in.DisplayName = probe.DisplayName

	switch {
	case in.Password != "":
		auth.CheckPassword(fields, "password", in.Password)
	case mode == config.AuthModePassword:
		fields.Add("password", httpx.CodeRequired)
	}
	return mode, nil
}
