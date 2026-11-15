package users

import (
	"cmp"
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

	fields := httpx.FieldErrors{Message: "invalid setup"}
	mode, err := s.validateSetup(&req, &fields)
	if err != nil {
		return err
	}
	changes := settings.Changes{InstanceName: req.InstanceName, Timezone: req.Timezone, Units: map[units.Family]units.Unit{}}
	for family, unit := range req.Units {
		changes.Units[units.Family(family)] = units.Unit(unit)
	}
	fields.AddFrom(changes.Validate())
	if err := fields.Err(); err != nil {
		return err
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
		rejected := httpx.FieldErrors{Message: "invalid setup"}
		if err := store.Apply(ctx, changes); err != nil && !rejected.AddFrom(err) {
			return err
		}
		if err := store.SwitchAuthMode(ctx, mode); err != nil && !rejected.AddFrom(err) {
			return err
		}
		if err := rejected.Err(); err != nil {
			return err
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

// validateSetup fills in defaults, returns the login mode to store and reports the problems with the request.
func (s *Service) validateSetup(req *setupRequest, fields *httpx.FieldErrors) (mode string, err error) {
	locked := s.Auth.Config.AuthMode
	mode = req.AuthMode
	if mode == "" {
		mode = cmp.Or(locked, config.AuthModePassword)
	}
	switch {
	case mode != config.AuthModePassword && mode != config.AuthModeDisabled:
		fields.Add("auth_mode", httpx.CodeInvalid)
	case locked != "" && mode != locked:
		return "", settings.AuthModeLocked()
	}

	if req.Theme == "" {
		req.Theme = "system"
	}
	if !slices.Contains(themes, req.Theme) {
		fields.Add("theme", httpx.CodeInvalid)
	}

	req.Username = strings.TrimSpace(req.Username)
	if mode == config.AuthModeDisabled && req.Username == "" {
		req.Username = defaultAdminName
	}
	probe := CreateInput{Username: req.Username, DisplayName: req.DisplayName, Role: httpx.RoleAdmin}
	validateNewAccount(&probe, fields)
	req.DisplayName = probe.DisplayName

	switch {
	case req.Password != "":
		auth.CheckPassword(fields, "password", req.Password)
	case mode == config.AuthModePassword:
		fields.Add("password", httpx.CodeRequired)
	}
	return mode, nil
}
