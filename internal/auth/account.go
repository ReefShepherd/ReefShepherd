package auth

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/reefshepherd/reefshepherd/internal/auth/password"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

// beginAttempt counts one guess against the username and the request's address, answering 429 while either is locked out.
func (s *Service) beginAttempt(r *http.Request, username string) (attempt, error) {
	try, wait := s.limiter.begin(httpx.ClientIP(r, s.Config.TrustedProxies), normalizeUsername(username))
	if wait > 0 {
		return attempt{}, httpx.TooManyRequests("too many failed attempts, try again later", wait)
	}
	return try, nil
}

// ConfirmPassword checks a signed-in user's own password, counting a mistake against the same limits as a login; a failure is an *httpx.APIError whose field is field.
func (s *Service) ConfirmPassword(ctx context.Context, r *http.Request, userID int64, plain, field string) error {
	u, err := db.Q(s.DB.R).GetUserCredentialsByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("read user: %w", err)
	}
	if u.PasswordHash == nil {
		return httpx.Validation("this account has no password", map[string]string{field: "this account has no password"})
	}
	try, err := s.beginAttempt(r, u.Username)
	if err != nil {
		return err
	}
	ok, rehash, err := s.Passwords.Verify(ctx, plain, *u.PasswordHash)
	if err != nil {
		return fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		return httpx.Validation("incorrect password", map[string]string{field: "incorrect password"})
	}
	s.limiter.succeed(try)
	if rehash {
		s.rehash(ctx, u.ID, plain, *u.PasswordHash)
	}
	return nil
}

// rehash upgrades a stored hash made with older parameters; failing only costs the upgrade, so it is logged.
func (s *Service) rehash(ctx context.Context, userID int64, plain, old string) {
	fresh, err := s.Passwords.Hash(ctx, plain)
	if err == nil {
		_, err = db.Q(s.DB.W).RehashPassword(ctx, dbgen.RehashPasswordParams{PasswordHash: &fresh, ID: userID, PasswordHash_2: &old})
	}
	if err != nil {
		s.Logger.Warn("upgrade password hash", slog.Int64("user_id", userID), slog.Any("error", err))
	}
}

// ChangePassword sets a new password after checking the current one, which a passwordless account may omit, and ends the user's other sessions.
func (s *Service) ChangePassword(ctx context.Context, r *http.Request, userID int64, current, next string) error {
	if err := password.Validate(next); err != nil {
		return httpx.Validation("invalid password", map[string]string{"new_password": err.Error()})
	}
	u, err := db.Q(s.DB.R).GetUserCredentialsByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("read user: %w", err)
	}
	if u.PasswordHash != nil {
		if err := s.ConfirmPassword(ctx, r, userID, current, "current_password"); err != nil {
			return err
		}
	}
	hash, err := s.Passwords.Hash(ctx, next)
	if err != nil {
		return err
	}
	keep, _ := SessionID(r.Context())
	return s.SetPassword(ctx, userID, hash, keep)
}

// SetPassword stores a new password hash and ends the user's sessions except the one with id keep, together or not at all.
func (s *Service) SetPassword(ctx context.Context, userID int64, hash string, keep int64) error {
	now := db.FormatTime(s.now())
	return s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		err := q.SetUserPassword(ctx, dbgen.SetUserPasswordParams{PasswordHash: &hash, PasswordChangedAt: &now, UpdatedAt: now, ID: userID})
		if err != nil {
			return fmt.Errorf("store password: %w", err)
		}
		if _, err := q.DeleteOtherUserSessions(ctx, dbgen.DeleteOtherUserSessionsParams{UserID: userID, ID: keep}); err != nil {
			return fmt.Errorf("revoke sessions: %w", err)
		}
		return nil
	})
}
