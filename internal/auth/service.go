// Package auth owns passwords, sessions, TOTP and the login endpoints.
package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/auth/password"
	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

// Method values recorded in sessions.auth_method.
const (
	MethodPassword = "password"
)

// Session stages recorded in sessions.stage.
const (
	stageActive     = "active"
	stageMFAPending = "mfa_pending"
)

const (
	sessionLifetime = 30 * 24 * time.Hour
	// touchInterval is how stale last_seen_at may get before a request writes it again.
	touchInterval = time.Hour
	// mfaWindow is how long a password-verified session may wait for its second step.
	mfaWindow = 5 * time.Minute
)

// Settings is what the package reads from the instance settings; *settings.Store satisfies it.
type Settings interface {
	GetSetupCompleted(ctx context.Context) (bool, error)
	GetAuthMode(ctx context.Context) (string, error)
	GetAuthDisabledUserID(ctx context.Context) (int64, error)
	GetInstanceName(ctx context.Context) (string, error)
}

// Deps are the dependencies of the package.
type Deps struct {
	DB        *db.DB
	Settings  Settings
	Passwords *password.Hasher
	Config    config.Config
	// Logger receives the errors behind 500 responses; nil means slog.Default().
	Logger *slog.Logger
}

// Service holds the login state of the process: the rate limiter and the clock.
type Service struct {
	Deps
	limiter *limiter
	now     func() time.Time
}

// NewService builds the service; the same instance must serve every route so the rate limits are shared.
func NewService(deps Deps) *Service {
	s := &Service{Deps: deps, now: time.Now}
	s.limiter = newLimiter(func() time.Time { return s.now() })
	s.limiter.log = s.logger()
	return s
}

func formatTime(t time.Time) string { return t.UTC().Format(db.TimeLayout) }

func parseTime(s string) (time.Time, error) { return time.Parse(db.TimeLayout, s) }

// EffectiveMode is the login mode in force: the environment override when set, else the stored setting.
func (s *Service) EffectiveMode(ctx context.Context) (string, error) {
	if s.Config.AuthMode != "" {
		return s.Config.AuthMode, nil
	}
	mode, err := s.Settings.GetAuthMode(ctx)
	if err != nil {
		return "", fmt.Errorf("read auth mode: %w", err)
	}
	return mode, nil
}

// SetupRequired reports whether first-run setup is still to be done: it has not been completed and no user exists.
func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	done, err := s.Settings.GetSetupCompleted(ctx)
	if err != nil {
		return false, fmt.Errorf("read setup state: %w", err)
	}
	if done {
		return false, nil
	}
	n, err := db.Q(s.DB.R).CountUsers(ctx)
	if err != nil {
		return false, fmt.Errorf("count users: %w", err)
	}
	return n == 0, nil
}

// disabledModeUser is the user everyone acts as when login is off: the stored one while it is an enabled admin, else the first enabled admin.
func (s *Service) disabledModeUser(ctx context.Context) (httpx.User, bool, error) {
	q := db.Q(s.DB.R)
	id, err := s.Settings.GetAuthDisabledUserID(ctx)
	if err != nil {
		return httpx.User{}, false, fmt.Errorf("read disabled-mode user: %w", err)
	}
	if id != 0 {
		u, err := q.GetUserCredentialsByID(ctx, id)
		switch {
		case err == nil && u.Disabled == 0 && u.Role == httpx.RoleAdmin:
			return httpx.User{ID: u.ID, Username: u.Username, DisplayName: u.DisplayName, Role: u.Role}, true, nil
		case err != nil && !errors.Is(err, sql.ErrNoRows):
			return httpx.User{}, false, fmt.Errorf("read disabled-mode user: %w", err)
		}
	}
	first, err := q.FirstEnabledAdminID(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return httpx.User{}, false, nil
	}
	if err != nil {
		return httpx.User{}, false, fmt.Errorf("find first admin: %w", err)
	}
	u, err := q.GetUserCredentialsByID(ctx, first)
	if err != nil {
		return httpx.User{}, false, fmt.Errorf("read first admin: %w", err)
	}
	return httpx.User{ID: u.ID, Username: u.Username, DisplayName: u.DisplayName, Role: u.Role}, true, nil
}

func normalizeUsername(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
