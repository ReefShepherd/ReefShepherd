package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

// CookieName is the session cookie, which holds a random token and nothing else.
const CookieName = "rs_session"

const (
	tokenBytes = 32
	// maxUserAgentLen keeps a hostile User-Agent header from bloating the sessions table.
	maxUserAgentLen = 255
)

func newToken() (token string, hash []byte, err error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("generate session token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, hashToken(token), nil
}

// hashToken is what the database stores, so a copy of it cannot be used as a cookie.
func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func (s *Service) secureCookie(r *http.Request) bool {
	return strings.HasPrefix(strings.ToLower(s.Config.PublicURL), "https://") || httpx.IsHTTPS(r, s.Config.TrustedProxies)
}

func (s *Service) setCookie(w http.ResponseWriter, r *http.Request, token string, lifetime time.Duration) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure follows the public URL, which may be plain http on a LAN
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(lifetime / time.Second),
		Expires:  s.now().Add(lifetime),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.secureCookie(r),
	})
}

func (s *Service) clearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure follows the public URL, which may be plain http on a LAN
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.secureCookie(r),
	})
}

// StartSession records that userID proved who they are by method and signs the request's browser in, replacing any session it already held.
func (s *Service) StartSession(w http.ResponseWriter, r *http.Request, userID int64, method string) error {
	return s.startSession(w, r, userID, method, stageActive)
}

func (s *Service) startSession(w http.ResponseWriter, r *http.Request, userID int64, method, stage string) error {
	ctx := r.Context()
	token, hash, err := newToken()
	if err != nil {
		return err
	}
	now := s.now()
	lifetime := sessionLifetime
	if stage == stageMFAPending {
		lifetime = mfaWindow
	}
	ua := r.UserAgent()
	if len(ua) > maxUserAgentLen {
		ua = ua[:maxUserAgentLen]
	}
	params := dbgen.CreateSessionParams{
		TokenHash:  hash,
		UserID:     userID,
		Stage:      stage,
		AuthMethod: method,
		UserAgent:  strings.ToValidUTF8(ua, ""),
		IpAddress:  httpx.ClientIP(r, s.Config.TrustedProxies).String(),
		CreatedAt:  db.FormatTime(now),
		LastSeenAt: db.FormatTime(now),
		ExpiresAt:  db.FormatTime(now.Add(lifetime)),
	}
	err = s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		// Presenting an old token to a fresh login must not leave that token usable.
		if c, cerr := r.Cookie(CookieName); cerr == nil {
			if _, err := q.DeleteSessionByTokenHash(ctx, hashToken(c.Value)); err != nil {
				return fmt.Errorf("drop previous session: %w", err)
			}
		}
		if _, err := q.DeleteExpiredSessions(ctx, db.FormatTime(now)); err != nil {
			return fmt.Errorf("delete expired sessions: %w", err)
		}
		if _, err := q.CreateSession(ctx, params); err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.setCookie(w, r, token, lifetime)
	return nil
}

// CleanupExpired deletes the sessions that have run out.
func (s *Service) CleanupExpired(ctx context.Context) error {
	if _, err := db.Q(s.DB.W).DeleteExpiredSessions(ctx, db.FormatTime(s.now())); err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}

// sessionFromCookie finds the unexpired session of an enabled user the request's cookie points at, whatever its stage.
func (s *Service) sessionFromCookie(ctx context.Context, r *http.Request) (dbgen.GetSessionByTokenHashRow, bool, error) {
	c, err := r.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return dbgen.GetSessionByTokenHashRow{}, false, nil
	}
	row, err := db.Q(s.DB.R).GetSessionByTokenHash(ctx, dbgen.GetSessionByTokenHashParams{
		TokenHash: hashToken(c.Value),
		ExpiresAt: db.FormatTime(s.now()),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return dbgen.GetSessionByTokenHashRow{}, false, nil
	}
	if err != nil {
		return dbgen.GetSessionByTokenHashRow{}, false, fmt.Errorf("look up session: %w", err)
	}
	return row, true, nil
}

// authenticate resolves who is making the request, from the disabled-mode user or the session cookie.
func (s *Service) authenticate(w http.ResponseWriter, r *http.Request) (httpx.User, sessionInfo, bool, error) {
	ctx := r.Context()
	mode, err := s.EffectiveMode(ctx)
	if err != nil {
		return httpx.User{}, sessionInfo{}, false, err
	}
	if mode == config.AuthModeDisabled {
		u, ok, err := s.disabledModeUser(ctx)
		return u, sessionInfo{}, ok, err
	}
	row, ok, err := s.sessionFromCookie(ctx, r)
	if err != nil || !ok || row.Stage != stageActive {
		return httpx.User{}, sessionInfo{}, false, err
	}
	now := s.now()
	if seen, perr := db.ParseTime(row.LastSeenAt); perr != nil || now.Sub(seen) >= touchInterval {
		if err := s.touch(w, r, row, now); err != nil {
			return httpx.User{}, sessionInfo{}, false, err
		}
	}
	user := newUser(row.UserID, row.Username, row.DisplayName, row.Role)
	return user, sessionInfo{ID: row.SessionID, Method: row.AuthMethod}, true, nil
}

// touch slides the expiry forward and re-issues the cookie so the browser's copy lives as long as the row.
func (s *Service) touch(w http.ResponseWriter, r *http.Request, row dbgen.GetSessionByTokenHashRow, now time.Time) error {
	err := db.Q(s.DB.W).TouchSession(r.Context(), dbgen.TouchSessionParams{
		LastSeenAt: db.FormatTime(now),
		ExpiresAt:  db.FormatTime(now.Add(sessionLifetime)),
		ID:         row.SessionID,
	})
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	c, cerr := r.Cookie(CookieName)
	if cerr == nil {
		s.setCookie(w, r, c.Value, sessionLifetime)
	}
	return nil
}

// RevokeSessions ends all of a user's sessions except the one with id keep; pass 0 to keep none.
func (s *Service) RevokeSessions(ctx context.Context, userID, keep int64) error {
	_, err := db.Q(s.DB.W).DeleteOtherUserSessions(ctx, dbgen.DeleteOtherUserSessionsParams{UserID: userID, ID: keep})
	if err != nil {
		return fmt.Errorf("revoke sessions: %w", err)
	}
	return nil
}
