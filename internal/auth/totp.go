package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image/png"
	"net/http"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

const (
	totpPeriod = 30
	// totpSkew is the number of 30-second steps either side of now that a code may come from.
	totpSkew     = 1
	totpDigits   = 6
	totpSecret   = 20
	qrSize       = 256
	recoveryN    = 10
	recoveryLen  = 10
	recoveryAbc  = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	recoveryHalf = recoveryLen / 2
)

// Errors of the TOTP operations.
var (
	ErrTOTPEnabled  = errors.New("two-step login is already enabled")
	ErrNoEnrollment = errors.New("two-step login enrolment has not been started")
	ErrNotEnabled   = errors.New("two-step login is not enabled")
	ErrBadCode      = errors.New("the code is not valid")
)

// Enrollment is what the user needs to add the account to an authenticator app.
type Enrollment struct {
	Secret     string
	OTPAuthURL string
	// QRPNG is the otpauth URL as a QR code image.
	QRPNG []byte
}

func totpOpts() totp.ValidateOpts {
	return totp.ValidateOpts{Period: totpPeriod, Skew: totpSkew, Digits: totpDigits, Algorithm: otp.AlgorithmSHA1}
}

// matchStep returns the newest time step within the skew window whose code equals code and that is later than after.
func matchStep(secret, code string, now time.Time, after int64) (int64, bool) {
	if len(code) != totpDigits {
		return 0, false
	}
	current := now.Unix() / totpPeriod
	var best int64
	// Every candidate is compared, so timing does not reveal which step matched.
	for step := current - totpSkew; step <= current+totpSkew; step++ {
		want, err := totp.GenerateCodeCustom(secret, time.Unix(step*totpPeriod, 0), totpOpts())
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 && step > after {
			best = max(best, step)
		}
	}
	return best, best != 0
}

// BeginTOTP creates a fresh unconfirmed secret for the user, replacing an earlier unconfirmed one.
func (s *Service) BeginTOTP(ctx context.Context, userID int64, username string) (Enrollment, error) {
	name, err := s.Settings.GetInstanceName(ctx)
	if err != nil {
		return Enrollment{}, fmt.Errorf("read instance name: %w", err)
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      strings.ReplaceAll(name, ":", " "),
		AccountName: username,
		Period:      totpPeriod,
		SecretSize:  totpSecret,
		Digits:      totpDigits,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return Enrollment{}, fmt.Errorf("generate totp key: %w", err)
	}
	img, err := key.Image(qrSize, qrSize)
	if err != nil {
		return Enrollment{}, fmt.Errorf("render qr code: %w", err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return Enrollment{}, fmt.Errorf("encode qr code: %w", err)
	}
	rows, err := db.Q(s.DB.W).StartTOTPEnrollment(ctx, dbgen.StartTOTPEnrollmentParams{
		UserID: userID, Secret: key.Secret(), CreatedAt: db.FormatTime(s.now()),
	})
	if err != nil {
		return Enrollment{}, fmt.Errorf("store totp secret: %w", err)
	}
	if rows == 0 {
		return Enrollment{}, ErrTOTPEnabled
	}
	return Enrollment{Secret: key.Secret(), OTPAuthURL: key.URL(), QRPNG: buf.Bytes()}, nil
}

// ConfirmTOTP turns on two-step login once the user proves their app works, and returns the recovery codes, which are never shown again.
func (s *Service) ConfirmTOTP(ctx context.Context, userID int64, code string) ([]string, error) {
	var codes []string
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		row, err := q.GetTOTP(ctx, userID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNoEnrollment
		}
		if err != nil {
			return fmt.Errorf("read totp: %w", err)
		}
		if row.ConfirmedAt != nil {
			return ErrTOTPEnabled
		}
		now := s.now()
		step, ok := matchStep(row.Secret, normalizeCode(code), now, 0)
		if !ok {
			return ErrBadCode
		}
		if _, err := q.ConfirmTOTP(ctx, dbgen.ConfirmTOTPParams{ConfirmedAt: ptr(db.FormatTime(now)), LastUsedStep: step, UserID: userID}); err != nil {
			return fmt.Errorf("confirm totp: %w", err)
		}
		codes, err = replaceRecoveryCodes(ctx, q, userID, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	return codes, nil
}

// DisableTOTP removes the user's secret and recovery codes, including an unfinished enrolment.
func (s *Service) DisableTOTP(ctx context.Context, userID int64) error {
	return s.DB.InTx(ctx, func(tx *sql.Tx) error {
		return removeTOTP(ctx, db.Q(tx), userID)
	})
}

func removeTOTP(ctx context.Context, q *dbgen.Queries, userID int64) error {
	n, err := q.DeleteTOTP(ctx, userID)
	if err != nil {
		return fmt.Errorf("delete totp: %w", err)
	}
	if err := q.DeleteRecoveryCodes(ctx, userID); err != nil {
		return fmt.Errorf("delete recovery codes: %w", err)
	}
	if n == 0 {
		return ErrNotEnabled
	}
	return nil
}

// ResetMFA removes a user's two-step login and ends their sessions except the one with id keep, for a lost authenticator; ErrNotEnabled is not an error here.
func (s *Service) ResetMFA(ctx context.Context, userID, keep int64) error {
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		if err := removeTOTP(ctx, q, userID); err != nil && !errors.Is(err, ErrNotEnabled) {
			return err
		}
		if _, err := q.DeleteOtherUserSessions(ctx, dbgen.DeleteOtherUserSessionsParams{UserID: userID, ID: keep}); err != nil {
			return fmt.Errorf("revoke sessions: %w", err)
		}
		return nil
	})
	return err
}

// verifyTOTP checks a login code and burns its time step, so the same code cannot be used twice.
func (s *Service) verifyTOTP(ctx context.Context, userID int64, code string) (bool, error) {
	row, err := db.Q(s.DB.R).GetTOTP(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read totp: %w", err)
	}
	if row.ConfirmedAt == nil {
		return false, nil
	}
	step, ok := matchStep(row.Secret, normalizeCode(code), s.now(), row.LastUsedStep)
	if !ok {
		return false, nil
	}
	// The update only succeeds while the stored step is older, which settles two concurrent uses of one code.
	n, err := db.Q(s.DB.W).AdvanceTOTPStep(ctx, dbgen.AdvanceTOTPStepParams{LastUsedStep: step, UserID: userID, LastUsedStep_2: step})
	if err != nil {
		return false, fmt.Errorf("record totp step: %w", err)
	}
	return n == 1, nil
}

// ConfirmTOTPCode checks a signed-in user's current authenticator code, counting a mistake against the same limits as a login; a failure is an *httpx.APIError whose field is field.
func (s *Service) ConfirmTOTPCode(ctx context.Context, r *http.Request, userID int64, code, field string) error {
	u, err := db.Q(s.DB.R).GetUserCredentialsByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("read user: %w", err)
	}
	try, err := s.beginAttempt(r, u.Username)
	if err != nil {
		return err
	}
	ok, err := s.verifyTOTP(ctx, userID, code)
	if err != nil {
		return err
	}
	if !ok {
		return httpx.Validation("that code is not valid", map[string]string{field: "that code is not valid"})
	}
	s.limiter.succeed(try)
	return nil
}

// verifyRecoveryCode spends a one-time recovery code.
func (s *Service) verifyRecoveryCode(ctx context.Context, userID int64, code string) (bool, error) {
	n, err := db.Q(s.DB.W).UseRecoveryCode(ctx, dbgen.UseRecoveryCodeParams{
		UsedAt: ptr(db.FormatTime(s.now())), UserID: userID, CodeHash: hashRecoveryCode(code),
	})
	if err != nil {
		return false, fmt.Errorf("use recovery code: %w", err)
	}
	return n == 1, nil
}

func normalizeCode(code string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' {
			return -1
		}
		return r
	}, code)
}

// hashRecoveryCode hashes a code ignoring case, spaces and dashes, the way it is stored.
func hashRecoveryCode(code string) string {
	clean := strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' {
			return -1
		}
		return r
	}, strings.ToUpper(code))
	sum := sha256.Sum256([]byte(clean))
	return hex.EncodeToString(sum[:])
}

// replaceRecoveryCodes stores a new set of codes, voiding any earlier set, and returns them for display.
func replaceRecoveryCodes(ctx context.Context, q *dbgen.Queries, userID int64, now time.Time) ([]string, error) {
	if err := q.DeleteRecoveryCodes(ctx, userID); err != nil {
		return nil, fmt.Errorf("delete recovery codes: %w", err)
	}
	codes := make([]string, recoveryN)
	for i := range codes {
		raw := make([]byte, recoveryLen)
		if _, err := rand.Read(raw); err != nil {
			return nil, fmt.Errorf("generate recovery code: %w", err)
		}
		for j, b := range raw {
			raw[j] = recoveryAbc[int(b)%len(recoveryAbc)]
		}
		codes[i] = string(raw[:recoveryHalf]) + "-" + string(raw[recoveryHalf:])
		err := q.InsertRecoveryCode(ctx, dbgen.InsertRecoveryCodeParams{
			UserID: userID, CodeHash: hashRecoveryCode(codes[i]), CreatedAt: db.FormatTime(now),
		})
		if err != nil {
			return nil, fmt.Errorf("store recovery code: %w", err)
		}
	}
	return codes, nil
}

// qrDataURL is a PNG as a data: URL, which the page's image policy allows.
func qrDataURL(png []byte) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
}

func ptr[T any](v T) *T { return &v }
