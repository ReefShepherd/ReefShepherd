package auth

import (
	"context"
	"database/sql"
	"errors"
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

// ConfirmPassword checks a signed-in user's own password, counting a mistake against the same limits as a login; a failure is a field error for field.
func (s *Service) ConfirmPassword(ctx context.Context, r *http.Request, userID int64, plain, field string) error {
	u, err := db.Q(s.DB.R).GetUserCredentialsByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("read user: %w", err)
	}
	if u.PasswordHash == nil {
		return httpx.Reject("this account has no password", field, httpx.CodeInvalid)
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
		return httpx.Reject("incorrect password", field, httpx.CodeIncorrect)
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
		_, err = db.Q(s.DB.W).RehashPassword(ctx, dbgen.RehashPasswordParams{NewHash: &fresh, ID: userID, OldHash: &old})
	}
	if err != nil {
		s.Logger.Warn("upgrade password hash", slog.Int64("user_id", userID), slog.Any("error", err))
	}
}

// CheckPassword adds a field error for field when plain breaks the password length or text rules.
func CheckPassword(fields *httpx.FieldErrors, field, plain string) {
	switch err := password.Validate(plain); {
	case err == nil:
	case errors.Is(err, password.ErrTooShort):
		fields.Add(field, httpx.CodeTooShort)
	case errors.Is(err, password.ErrTooLong):
		fields.Add(field, httpx.CodeTooLong)
	default:
		fields.Add(field, httpx.CodeInvalid)
	}
}

// ChangePassword sets a new password after checking the current one, which a passwordless account may omit, and ends the user's other sessions.
func (s *Service) ChangePassword(ctx context.Context, r *http.Request, userID int64, current, next string) error {
	fields := httpx.FieldErrors{Message: "invalid password"}
	CheckPassword(&fields, "new_password", next)
	if err := fields.Err(); err != nil {
		return err
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
	return s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		if err := SetPasswordTx(ctx, q, userID, hash, db.FormatTime(s.now())); err != nil {
			return err
		}
		return RevokeSessionsTx(ctx, q, userID, keep)
	})
}

// SetPasswordTx stores an already hashed password inside the caller's transaction, stamped with now in stored form; hashing stays outside so the write lock is not held for it.
func SetPasswordTx(ctx context.Context, q *dbgen.Queries, userID int64, hash, now string) error {
	if err := q.SetUserPassword(ctx, dbgen.SetUserPasswordParams{PasswordHash: &hash, Now: now, ID: userID}); err != nil {
		return fmt.Errorf("store password: %w", err)
	}
	return nil
}
