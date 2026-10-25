// Package users owns user accounts, per-user preferences and first-run setup.
package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/auth/password"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/settings"
)

// Deps are the dependencies the package's handlers need.
type Deps struct {
	DB       *db.DB
	Settings *settings.Store
	Auth     *auth.Service
	Logger   *slog.Logger
}

// Themes a user can pick.
var themes = []string{"system", "light", "dark", "oled"}

const (
	maxDisplayName = 100
	maxEmail       = 254
)

var (
	usernamePattern = regexp.MustCompile(`^[A-Za-z0-9._@-]{2,64}$`)
	localePattern   = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})?$`)
)

// User is a user account as the API shows it.
type User struct {
	ID          int64   `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"display_name"`
	Email       *string `json:"email"`
	Role        string  `json:"role"`
	Disabled    bool    `json:"disabled"`
	HasPassword bool    `json:"has_password"`
	TOTPEnabled bool    `json:"totp_enabled"`
	CreatedAt   string  `json:"created_at"`
}

// Instance is the instance-wide display information every signed-in user needs.
type Instance struct {
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
	// Units maps each unit family to the display unit chosen for it.
	Units map[string]string `json:"units"`
	// AuthMode is the login mode in force, which an environment override may set.
	AuthMode string `json:"auth_mode"`
}

// Me is the signed-in user with their preferences and the instance they are signed in to.
type Me struct {
	User
	Theme         string   `json:"theme"`
	Locale        string   `json:"locale"`
	DefaultTankID *int64   `json:"default_tank_id"`
	Instance      Instance `json:"instance"`
}

// Service is the user logic behind the HTTP handlers; its errors are *httpx.APIError when the client caused them.
type Service struct {
	Deps
	now func() time.Time
}

// NewService returns the service for deps.
func NewService(deps Deps) *Service {
	return &Service{Deps: deps, now: time.Now}
}

func userFromRow(id int64, username, displayName string, email *string, role string, disabled, hasPassword, totp int64, created string) User {
	return User{
		ID: id, Username: username, DisplayName: displayName, Email: email, Role: role,
		Disabled: disabled != 0, HasPassword: hasPassword != 0, TOTPEnabled: totp != 0, CreatedAt: created,
	}
}

func userOfRow(r dbgen.GetUserByIDRow) User {
	return userFromRow(r.ID, r.Username, r.DisplayName, r.Email, r.Role, r.Disabled, r.HasPassword, r.TotpEnabled, r.CreatedAt)
}

var errUserNotFound = httpx.NotFound("no such user")

func (s *Service) row(ctx context.Context, id int64) (dbgen.GetUserByIDRow, error) {
	row, err := db.Q(s.DB.R).GetUserByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, errUserNotFound
	}
	if err != nil {
		return row, fmt.Errorf("read user: %w", err)
	}
	return row, nil
}

// account returns the user without preferences or instance information.
func (s *Service) account(ctx context.Context, id int64) (User, error) {
	row, err := s.row(ctx, id)
	if err != nil {
		return User{}, err
	}
	return userOfRow(row), nil
}

// Me returns the user with their preferences and the instance information.
func (s *Service) Me(ctx context.Context, id int64) (Me, error) {
	row, err := s.row(ctx, id)
	if err != nil {
		return Me{}, err
	}
	instance, err := s.instance(ctx)
	if err != nil {
		return Me{}, err
	}
	return Me{User: userOfRow(row), Theme: row.Theme, Locale: row.Locale, DefaultTankID: row.DefaultTankID, Instance: instance}, nil
}

func (s *Service) instance(ctx context.Context) (Instance, error) {
	v, err := s.Settings.Values(ctx)
	if err != nil {
		return Instance{}, err
	}
	mode, err := s.Auth.EffectiveMode(ctx)
	if err != nil {
		return Instance{}, err
	}
	out := Instance{Name: v.InstanceName, Timezone: v.Timezone, Units: map[string]string{}, AuthMode: mode}
	for family, unit := range v.Units {
		out.Units[string(family)] = string(unit)
	}
	return out, nil
}

// List returns every user.
func (s *Service) List(ctx context.Context) ([]User, error) {
	rows, err := db.Q(s.DB.R).ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	out := make([]User, len(rows))
	for i, r := range rows {
		out[i] = userFromRow(r.ID, r.Username, r.DisplayName, r.Email, r.Role, r.Disabled, r.HasPassword, r.TotpEnabled, r.CreatedAt)
	}
	return out, nil
}

// UpdateMeInput are the changes a user may make to their own profile; nil or unset fields are left alone.
type UpdateMeInput struct {
	DisplayName   *string
	Email         httpx.Nullable[string]
	Theme         *string
	Locale        *string
	DefaultTankID httpx.Nullable[int64]
}

// UpdateMe changes the user's own profile and preferences.
func (s *Service) UpdateMe(ctx context.Context, id int64, in UpdateMeInput) (Me, error) {
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		cur, err := q.GetUserByID(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return errUserNotFound
		}
		if err != nil {
			return fmt.Errorf("read user: %w", err)
		}
		fields := map[string]string{}
		displayName, email := cur.DisplayName, cur.Email
		if in.DisplayName != nil {
			if displayName, err = cleanDisplayName(*in.DisplayName); err != nil {
				fields["display_name"] = err.Error()
			}
		}
		if in.Email.Set {
			if email, err = cleanEmail(in.Email.Value); err != nil {
				fields["email"] = err.Error()
			}
		}
		theme, locale, tank := cur.Theme, cur.Locale, cur.DefaultTankID
		if in.Theme != nil {
			if !slices.Contains(themes, *in.Theme) {
				fields["theme"] = "must be one of " + strings.Join(themes, ", ")
			}
			theme = *in.Theme
		}
		if in.Locale != nil {
			if !localePattern.MatchString(*in.Locale) {
				fields["locale"] = "must be a language code such as en"
			}
			locale = *in.Locale
		}
		if in.DefaultTankID.Set {
			tank = in.DefaultTankID.Value
			if tank != nil {
				if n, err := q.TankExists(ctx, *tank); err != nil {
					return fmt.Errorf("check tank: %w", err)
				} else if n == 0 {
					fields["default_tank_id"] = "no such tank"
				}
			}
		}
		if len(fields) > 0 {
			return httpx.Validation("invalid profile", fields)
		}
		now := db.FormatTime(s.now())
		if err := q.UpdateUserProfile(ctx, dbgen.UpdateUserProfileParams{DisplayName: displayName, Email: email, UpdatedAt: now, ID: id}); err != nil {
			return fmt.Errorf("update user: %w", err)
		}
		err = q.UpsertPreferences(ctx, dbgen.UpsertPreferencesParams{UserID: id, Theme: theme, Locale: locale, DefaultTankID: tank, UpdatedAt: now})
		if err != nil {
			return fmt.Errorf("update preferences: %w", err)
		}
		return nil
	})
	if err != nil {
		return Me{}, err
	}
	return s.Me(ctx, id)
}

// CreateInput describes a new account.
type CreateInput struct {
	Username    string
	DisplayName string
	Email       *string
	Role        string
	Password    string
}

// Create adds a user with a password.
func (s *Service) Create(ctx context.Context, in CreateInput) (User, error) {
	fields := validateNewAccount(&in)
	if err := password.Validate(in.Password); err != nil {
		fields["password"] = err.Error()
	}
	if len(fields) > 0 {
		return User{}, httpx.Validation("invalid user", fields)
	}
	hash, err := s.Auth.Passwords.Hash(ctx, in.Password)
	if err != nil {
		return User{}, err
	}
	var id int64
	err = s.DB.InTx(ctx, func(tx *sql.Tx) error {
		id, err = createAccount(ctx, db.Q(tx), in, &hash, db.FormatTime(s.now()))
		return err
	})
	if err != nil {
		return User{}, err
	}
	return s.account(ctx, id)
}

// validateNewAccount cleans the account fields in place and returns the problems found.
func validateNewAccount(in *CreateInput) map[string]string {
	fields := map[string]string{}
	in.Username = strings.TrimSpace(in.Username)
	if !usernamePattern.MatchString(in.Username) {
		fields["username"] = "must be 2 to 64 letters, digits, dots, dashes, underscores or @"
	}
	var err error
	if in.DisplayName, err = cleanDisplayName(in.DisplayName); err != nil {
		fields["display_name"] = err.Error()
	}
	if in.Email, err = cleanEmail(in.Email); err != nil {
		fields["email"] = err.Error()
	}
	if in.Role != httpx.RoleAdmin && in.Role != httpx.RoleMember {
		fields["role"] = "must be admin or member"
	}
	return fields
}

// createAccount inserts the user row; the caller supplies the transaction so setup can add its own writes.
func createAccount(ctx context.Context, q *dbgen.Queries, in CreateInput, hash *string, now string) (int64, error) {
	n, err := q.UserExistsByUsername(ctx, in.Username)
	if err != nil {
		return 0, fmt.Errorf("check username: %w", err)
	}
	if n > 0 {
		return 0, httpx.Validation("username is taken", map[string]string{"username": "already taken"})
	}
	var changed *string
	if hash != nil {
		changed = &now
	}
	id, err := q.CreateUser(ctx, dbgen.CreateUserParams{
		Username: in.Username, DisplayName: in.DisplayName, Email: in.Email,
		PasswordHash: hash, Role: in.Role, PasswordChangedAt: changed,
	})
	if err != nil {
		return 0, fmt.Errorf("create user: %w", err)
	}
	return id, nil
}

// UpdateInput are the changes an admin may make to an account; nil or unset fields are left alone.
type UpdateInput struct {
	DisplayName *string
	Email       httpx.Nullable[string]
	Role        *string
	Disabled    *bool
	Password    *string
}

var errLastAdmin = httpx.Conflict("at least one enabled administrator is required")

// Update changes another user's account; actor is the admin making the change and keepSession is the session id to leave signed in when they change themselves.
func (s *Service) Update(ctx context.Context, actor httpx.User, keepSession, id int64, in UpdateInput) (User, error) {
	var hash *string
	if in.Password != nil {
		if err := password.Validate(*in.Password); err != nil {
			return User{}, httpx.Validation("invalid user", map[string]string{"password": err.Error()})
		}
		h, err := s.Auth.Passwords.Hash(ctx, *in.Password)
		if err != nil {
			return User{}, err
		}
		hash = &h
	}
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		cur, err := q.GetUserByID(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return errUserNotFound
		}
		if err != nil {
			return fmt.Errorf("read user: %w", err)
		}
		fields := map[string]string{}
		displayName, email, role, disabled := cur.DisplayName, cur.Email, cur.Role, cur.Disabled != 0
		if in.DisplayName != nil {
			if displayName, err = cleanDisplayName(*in.DisplayName); err != nil {
				fields["display_name"] = err.Error()
			}
		}
		if in.Email.Set {
			if email, err = cleanEmail(in.Email.Value); err != nil {
				fields["email"] = err.Error()
			}
		}
		if in.Role != nil {
			if *in.Role != httpx.RoleAdmin && *in.Role != httpx.RoleMember {
				fields["role"] = "must be admin or member"
			}
			role = *in.Role
		}
		if in.Disabled != nil {
			disabled = *in.Disabled
		}
		if len(fields) > 0 {
			return httpx.Validation("invalid user", fields)
		}
		if disabled && actor.ID == id {
			return httpx.Conflict("you cannot disable your own account")
		}
		wasEnabledAdmin := cur.Role == httpx.RoleAdmin && cur.Disabled == 0
		if wasEnabledAdmin && (role != httpx.RoleAdmin || disabled) {
			if err := requireOtherAdmin(ctx, q, id); err != nil {
				return err
			}
			if err := s.handOverActingUser(ctx, tx, id); err != nil {
				return err
			}
		}
		now := db.FormatTime(s.now())
		err = q.UpdateUserAccount(ctx, dbgen.UpdateUserAccountParams{
			DisplayName: displayName, Email: email, Role: role, Disabled: db.Flag(disabled), UpdatedAt: now, ID: id,
		})
		if err != nil {
			return fmt.Errorf("update user: %w", err)
		}
		keep := int64(0)
		if actor.ID == id {
			keep = keepSession
		}
		if hash != nil {
			if err := q.SetUserPassword(ctx, dbgen.SetUserPasswordParams{PasswordHash: hash, PasswordChangedAt: &now, UpdatedAt: now, ID: id}); err != nil {
				return fmt.Errorf("set password: %w", err)
			}
		}
		if hash != nil || (disabled && cur.Disabled == 0) {
			if _, err := q.DeleteOtherUserSessions(ctx, dbgen.DeleteOtherUserSessionsParams{UserID: id, ID: keep}); err != nil {
				return fmt.Errorf("revoke sessions: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return User{}, err
	}
	return s.account(ctx, id)
}

// Delete removes another user and, through the database, their sessions and two-step login.
func (s *Service) Delete(ctx context.Context, actor httpx.User, id int64) error {
	if actor.ID == id {
		return httpx.Conflict("you cannot delete your own account")
	}
	return s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		cur, err := q.GetUserByID(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return errUserNotFound
		}
		if err != nil {
			return fmt.Errorf("read user: %w", err)
		}
		if cur.Role == httpx.RoleAdmin && cur.Disabled == 0 {
			if err := requireOtherAdmin(ctx, q, id); err != nil {
				return err
			}
			if err := s.handOverActingUser(ctx, tx, id); err != nil {
				return err
			}
		}
		if _, err := q.DeleteUser(ctx, id); err != nil {
			return fmt.Errorf("delete user: %w", err)
		}
		return nil
	})
}

// ResetMFA turns off another user's two-step login and signs them out everywhere but the admin's own session.
func (s *Service) ResetMFA(ctx context.Context, actor httpx.User, keepSession, id int64) error {
	if _, err := db.Q(s.DB.R).GetUserCredentialsByID(ctx, id); errors.Is(err, sql.ErrNoRows) {
		return errUserNotFound
	} else if err != nil {
		return fmt.Errorf("read user: %w", err)
	}
	keep := int64(0)
	if actor.ID == id {
		keep = keepSession
	}
	return s.Auth.ResetMFA(ctx, id, keep)
}

// handOverActingUser moves the user everyone acts as while login is off from id to another enabled admin, so the instance never ends up acting as someone without admin rights.
func (s *Service) handOverActingUser(ctx context.Context, tx *sql.Tx, id int64) error {
	store := s.Settings.WithTx(tx)
	acting, err := store.GetAuthDisabledUserID(ctx)
	if err != nil {
		return err
	}
	if acting != id {
		return nil
	}
	next, err := db.Q(tx).FirstOtherEnabledAdminID(ctx, id)
	if err != nil {
		return fmt.Errorf("find another admin: %w", err)
	}
	return store.SetAuthDisabledUserID(ctx, next)
}

// requireOtherAdmin fails unless an enabled admin other than id exists.
func requireOtherAdmin(ctx context.Context, q *dbgen.Queries, id int64) error {
	n, err := q.CountOtherEnabledAdmins(ctx, id)
	if err != nil {
		return fmt.Errorf("count admins: %w", err)
	}
	if n == 0 {
		return errLastAdmin
	}
	return nil
}

func cleanDisplayName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxDisplayName {
		return "", fmt.Errorf("must be at most %d characters", maxDisplayName)
	}
	if !utf8.ValidString(s) {
		return "", errors.New("must be valid text")
	}
	return s, nil
}

// cleanEmail returns nil for a blank address.
func cleanEmail(s *string) (*string, error) {
	if s == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil, nil
	}
	addr, err := mail.ParseAddress(v)
	if err != nil || addr.Address != v || len(v) > maxEmail {
		return nil, errors.New("must be an email address")
	}
	return &v, nil
}
