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
	minUsername    = 2
	maxUsername    = 64
)

var (
	usernamePattern = regexp.MustCompile(fmt.Sprintf(`^[A-Za-z0-9._@-]{%d,%d}$`, minUsername, maxUsername))
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

// Service is the user logic behind the HTTP handlers; the errors it returns for a client mistake are field errors and the sentinels below.
type Service struct {
	Deps
	now func() time.Time
}

// NewService returns the service for deps.
func NewService(deps Deps) *Service {
	return &Service{Deps: deps, now: time.Now}
}

func userFromRow(r dbgen.GetUserByIDRow) User {
	return User{
		ID: r.ID, Username: r.Username, DisplayName: r.DisplayName, Email: r.Email, Role: r.Role,
		Disabled: r.Disabled != 0, HasPassword: r.HasPassword != 0, TOTPEnabled: r.TotpEnabled != 0, CreatedAt: r.CreatedAt,
	}
}

// Refusals the handlers turn into 404 and 409 responses; the text is the message sent.
var (
	errUserNotFound = errors.New("no such user")
	errLastAdmin    = errors.New("at least one enabled administrator is required")
	errDisableSelf  = errors.New("you cannot disable your own account")
	errDeleteSelf   = errors.New("you cannot delete your own account")
	errSetupDone    = errors.New("setup has already been completed")
)

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
	return userFromRow(row), nil
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
	return Me{User: userFromRow(row), Theme: row.Theme, Locale: row.Locale, DefaultTankID: row.DefaultTankID, Instance: instance}, nil
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
		out[i] = userFromRow(dbgen.GetUserByIDRow(r))
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
		fields := httpx.FieldErrors{Message: "invalid profile"}
		displayName, email := cur.DisplayName, cur.Email
		if in.DisplayName != nil {
			displayName = cleanDisplayName(*in.DisplayName, &fields)
		}
		if in.Email.Set {
			email = cleanEmail(in.Email.Value, &fields)
		}
		theme, locale, tank := cur.Theme, cur.Locale, cur.DefaultTankID
		if in.Theme != nil {
			if !slices.Contains(themes, *in.Theme) {
				fields.Add("theme", httpx.CodeInvalid)
			}
			theme = *in.Theme
		}
		if in.Locale != nil {
			if !localePattern.MatchString(*in.Locale) {
				fields.Add("locale", httpx.CodeInvalid)
			}
			locale = *in.Locale
		}
		if in.DefaultTankID.Set {
			tank = in.DefaultTankID.Value
			if tank != nil {
				if n, err := q.TankExists(ctx, *tank); err != nil {
					return fmt.Errorf("check tank: %w", err)
				} else if n == 0 {
					fields.Add("default_tank_id", httpx.CodeInvalid)
				}
			}
		}
		if err := fields.Err(); err != nil {
			return err
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
	fields := httpx.FieldErrors{Message: "invalid user"}
	validateNewAccount(&in, &fields)
	auth.CheckPassword(&fields, "password", in.Password)
	if err := fields.Err(); err != nil {
		return User{}, err
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

// validateNewAccount cleans the account fields in place and reports the problems found.
func validateNewAccount(in *CreateInput, fields *httpx.FieldErrors) {
	in.Username = strings.TrimSpace(in.Username)
	if !usernamePattern.MatchString(in.Username) {
		switch n := utf8.RuneCountInString(in.Username); {
		case n < minUsername:
			fields.Add("username", httpx.CodeTooShort)
		case n > maxUsername:
			fields.Add("username", httpx.CodeTooLong)
		default:
			fields.Add("username", httpx.CodeInvalid)
		}
	}
	in.DisplayName = cleanDisplayName(in.DisplayName, fields)
	in.Email = cleanEmail(in.Email, fields)
	if in.Role != httpx.RoleAdmin && in.Role != httpx.RoleMember {
		fields.Add("role", httpx.CodeInvalid)
	}
}

// createAccount inserts the user row; the caller supplies the transaction so setup can add its own writes.
func createAccount(ctx context.Context, q *dbgen.Queries, in CreateInput, hash *string, now string) (int64, error) {
	n, err := q.UserExistsByUsername(ctx, in.Username)
	if err != nil {
		return 0, fmt.Errorf("check username: %w", err)
	}
	if n > 0 {
		return 0, httpx.Reject("username is taken", "username", httpx.CodeTaken)
	}
	id, err := q.CreateUser(ctx, dbgen.CreateUserParams{
		Username: in.Username, DisplayName: in.DisplayName, Email: in.Email, Role: in.Role, Now: now,
	})
	if err != nil {
		return 0, fmt.Errorf("create user: %w", err)
	}
	if hash != nil {
		if err := auth.SetPasswordTx(ctx, q, id, *hash, now); err != nil {
			return 0, err
		}
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

// Update changes another user's account; actor is the admin making the change and keepSession is the session id to leave signed in when they change themselves.
func (s *Service) Update(ctx context.Context, actor httpx.User, keepSession, id int64, in UpdateInput) (User, error) {
	var hash *string
	if in.Password != nil {
		fields := httpx.FieldErrors{Message: "invalid user"}
		auth.CheckPassword(&fields, "password", *in.Password)
		if err := fields.Err(); err != nil {
			return User{}, err
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
		fields := httpx.FieldErrors{Message: "invalid user"}
		displayName, email, role, disabled := cur.DisplayName, cur.Email, cur.Role, cur.Disabled != 0
		if in.DisplayName != nil {
			displayName = cleanDisplayName(*in.DisplayName, &fields)
		}
		if in.Email.Set {
			email = cleanEmail(in.Email.Value, &fields)
		}
		if in.Role != nil {
			if *in.Role != httpx.RoleAdmin && *in.Role != httpx.RoleMember {
				fields.Add("role", httpx.CodeInvalid)
			}
			role = *in.Role
		}
		if in.Disabled != nil {
			disabled = *in.Disabled
		}
		if err := fields.Err(); err != nil {
			return err
		}
		if disabled && actor.ID == id {
			return errDisableSelf
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
		if hash != nil {
			if err := auth.SetPasswordTx(ctx, q, id, *hash, now); err != nil {
				return err
			}
		}
		if hash != nil || (disabled && cur.Disabled == 0) {
			keep := int64(0)
			if actor.ID == id {
				keep = keepSession
			}
			return auth.RevokeSessionsTx(ctx, q, id, keep)
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
		return errDeleteSelf
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

// cleanDisplayName trims the name and reports it when it is too long or not valid text.
func cleanDisplayName(s string, fields *httpx.FieldErrors) string {
	s = strings.TrimSpace(s)
	switch {
	case utf8.RuneCountInString(s) > maxDisplayName:
		fields.Add("display_name", httpx.CodeTooLong)
	case !utf8.ValidString(s):
		fields.Add("display_name", httpx.CodeInvalid)
	}
	return s
}

// cleanEmail returns nil for a blank address and reports one that is not an email address.
func cleanEmail(s *string, fields *httpx.FieldErrors) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	addr, err := mail.ParseAddress(v)
	if err != nil || addr.Address != v || len(v) > maxEmail {
		fields.Add("email", httpx.CodeInvalid)
		return nil
	}
	return &v
}
