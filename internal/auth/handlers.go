package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/version"
)

// PublicRoutes registers the endpoints that work without a session.
func (s *Service) PublicRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/auth/config", s.handle(s.getConfig))
	mux.HandleFunc("POST /api/v1/auth/login", s.handle(s.login))
	mux.HandleFunc("POST /api/v1/auth/mfa", s.handle(s.mfa))
	mux.HandleFunc("POST /api/v1/auth/logout", s.handle(s.logout))
}

// Routes registers the signed-in user's own security endpoints; the caller wraps mux in RequireUser.
func (s *Service) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/me/password", s.handle(s.changePassword))
	mux.HandleFunc("POST /api/v1/me/totp/enroll", s.handle(s.enrollTOTP))
	mux.HandleFunc("POST /api/v1/me/totp/confirm", s.handle(s.confirmTOTP))
	mux.HandleFunc("DELETE /api/v1/me/totp", s.handle(s.disableTOTP))
	mux.HandleFunc("GET /api/v1/me/sessions", s.handle(s.listSessions))
	mux.HandleFunc("DELETE /api/v1/me/sessions/{id}", s.handle(s.revokeSession))
}

type handlerFunc func(w http.ResponseWriter, r *http.Request) error

// handle adapts a handler that returns an error, so every failure takes the same path to the client.
func (s *Service) handle(h handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			httpx.Fail(s.Logger, w, r, err)
		}
	}
}

type configResponse struct {
	Mode          string   `json:"mode"`
	SetupRequired bool     `json:"setup_required"`
	Methods       []string `json:"methods"`
	InstanceName  string   `json:"instance_name"`
	Version       string   `json:"version"`
	// ModeLocked is true when the server environment decides the login mode, so the web setting cannot change it.
	ModeLocked bool `json:"mode_locked"`
}

func (s *Service) getConfig(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	mode, err := s.EffectiveMode(ctx)
	if err != nil {
		return err
	}
	setup, err := s.SetupRequired(ctx)
	if err != nil {
		return err
	}
	name, err := s.Settings.GetInstanceName(ctx)
	if err != nil {
		return fmt.Errorf("read instance name: %w", err)
	}
	methods := []string{}
	if mode == config.AuthModePassword {
		methods = append(methods, MethodPassword)
	}
	httpx.WriteJSON(w, http.StatusOK, configResponse{Mode: mode, SetupRequired: setup, Methods: methods, InstanceName: name, Version: version.Version(), ModeLocked: s.Config.AuthMode != ""})
	return nil
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	MFARequired bool `json:"mfa_required"`
}

func (s *Service) login(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	mode, err := s.EffectiveMode(ctx)
	if err != nil {
		return err
	}
	if mode != config.AuthModePassword {
		return httpx.Conflict("login is turned off")
	}
	var req loginRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	if req.Username == "" || req.Password == "" {
		fields := map[string]string{}
		if req.Username == "" {
			fields["username"] = "required"
		}
		if req.Password == "" {
			fields["password"] = "required"
		}
		return httpx.Validation("username and password are required", fields)
	}

	ip := httpx.ClientIP(r, s.Config.TrustedProxies)
	name := normalizeUsername(req.Username)
	try, wait := s.limiter.begin(ip, name)
	if wait > 0 {
		return tooManyAttempts(wait)
	}

	invalid := func() error {
		return httpx.NewError(http.StatusUnauthorized, "invalid_credentials", "incorrect username or password")
	}
	u, err := db.Q(s.DB.R).GetUserCredentialsByUsername(ctx, strings.TrimSpace(req.Username))
	if err != nil || u.Disabled != 0 || u.PasswordHash == nil {
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("look up user: %w", err)
		}
		// The same work as a real check, so response time does not tell which usernames exist.
		if err := s.Passwords.SpendVerifyTime(ctx, req.Password); err != nil {
			return err
		}
		return invalid()
	}
	ok, rehash, err := s.Passwords.Verify(ctx, req.Password, *u.PasswordHash)
	if err != nil {
		s.logger().Warn("verify password", "user_id", u.ID, "error", err)
		return invalid()
	}
	if !ok {
		return invalid()
	}
	if rehash {
		s.rehash(ctx, u.ID, req.Password, *u.PasswordHash)
	}

	totpRow, err := db.Q(s.DB.R).GetTOTP(ctx, u.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read totp: %w", err)
	}
	if err == nil && totpRow.ConfirmedAt != nil {
		// The username keeps its failures until the second step succeeds, so guessing codes stays rate limited.
		s.limiter.partial(try)
		if err := s.startSession(w, r, u.ID, MethodPassword, stageMFAPending); err != nil {
			return err
		}
		httpx.WriteJSON(w, http.StatusOK, loginResponse{MFARequired: true})
		return nil
	}
	s.limiter.succeed(try)
	if err := s.StartSession(w, r, u.ID, MethodPassword); err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, loginResponse{})
	return nil
}

type mfaRequest struct {
	Code         string `json:"code"`
	RecoveryCode string `json:"recovery_code"`
}

func (s *Service) mfa(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var req mfaRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	row, ok, err := s.sessionFromCookie(ctx, r)
	if err != nil {
		return err
	}
	if !ok || row.Stage != stageMFAPending {
		return httpx.Unauthorized("sign in again")
	}
	if (req.Code == "") == (req.RecoveryCode == "") {
		return httpx.Validation("send either code or recovery_code", map[string]string{"code": "send either code or recovery_code"})
	}

	ip := httpx.ClientIP(r, s.Config.TrustedProxies)
	name := normalizeUsername(row.Username)
	try, wait := s.limiter.begin(ip, name)
	if wait > 0 {
		return tooManyAttempts(wait)
	}
	if req.Code != "" {
		ok, err = s.verifyTOTP(ctx, row.UserID, req.Code)
	} else {
		ok, err = s.verifyRecoveryCode(ctx, row.UserID, req.RecoveryCode)
	}
	if err != nil {
		return err
	}
	if !ok {
		return httpx.NewError(http.StatusUnauthorized, "invalid_code", "that code is not valid")
	}

	token, hash, err := newToken()
	if err != nil {
		return err
	}
	now := s.now()
	n, err := db.Q(s.DB.W).UpgradeSession(ctx, dbgen.UpgradeSessionParams{
		TokenHash:  hash,
		CreatedAt:  formatTime(now),
		LastSeenAt: formatTime(now),
		ExpiresAt:  formatTime(now.Add(sessionLifetime)),
		ID:         row.SessionID,
	})
	if err != nil {
		return fmt.Errorf("upgrade session: %w", err)
	}
	if n != 1 {
		return httpx.Unauthorized("sign in again")
	}
	s.limiter.succeed(try)
	s.setCookie(w, r, token, sessionLifetime)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Service) logout(w http.ResponseWriter, r *http.Request) error {
	if err := httpx.RequireJSONOrNoBody(r); err != nil {
		return err
	}
	if c, err := r.Cookie(CookieName); err == nil {
		if _, err := db.Q(s.DB.W).DeleteSessionByTokenHash(r.Context(), hashToken(c.Value)); err != nil {
			return fmt.Errorf("delete session: %w", err)
		}
	}
	s.clearCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *Service) changePassword(w http.ResponseWriter, r *http.Request) error {
	user, _ := httpx.CurrentUser(r.Context())
	var req changePasswordRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	if err := s.ChangePassword(r.Context(), r, user.ID, req.CurrentPassword, req.NewPassword); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type enrollResponse struct {
	Secret     string `json:"secret"`
	OTPAuthURL string `json:"otpauth_url"`
	QRCode     string `json:"qr_code"`
}

func (s *Service) enrollTOTP(w http.ResponseWriter, r *http.Request) error {
	if err := httpx.RequireJSONOrNoBody(r); err != nil {
		return err
	}
	user, _ := httpx.CurrentUser(r.Context())
	e, err := s.BeginTOTP(r.Context(), user.ID, user.Username)
	if errors.Is(err, ErrTOTPEnabled) {
		return httpx.Conflict("two-step login is already enabled")
	}
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, enrollResponse{Secret: e.Secret, OTPAuthURL: e.OTPAuthURL, QRCode: qrDataURL(e.QRPNG)})
	return nil
}

type confirmRequest struct {
	Code string `json:"code"`
}

type confirmResponse struct {
	RecoveryCodes []string `json:"recovery_codes"`
}

func (s *Service) confirmTOTP(w http.ResponseWriter, r *http.Request) error {
	user, _ := httpx.CurrentUser(r.Context())
	var req confirmRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	codes, err := s.ConfirmTOTP(r.Context(), user.ID, req.Code)
	switch {
	case errors.Is(err, ErrBadCode):
		return httpx.Validation("that code is not valid", map[string]string{"code": "that code is not valid"})
	case errors.Is(err, ErrNoEnrollment):
		return httpx.Conflict("start enrolment first")
	case errors.Is(err, ErrTOTPEnabled):
		return httpx.Conflict("two-step login is already enabled")
	case err != nil:
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, confirmResponse{RecoveryCodes: codes})
	return nil
}

type disableTOTPRequest struct {
	// Password is the user's current password; Code is a current authenticator code, for accounts without a password.
	Password string `json:"password"`
	Code     string `json:"code"`
}

func (s *Service) disableTOTP(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	user, _ := httpx.CurrentUser(ctx)
	row, err := db.Q(s.DB.R).GetTOTP(ctx, user.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return httpx.NotFound("two-step login is not enabled")
	}
	if err != nil {
		return fmt.Errorf("read totp: %w", err)
	}
	var req disableTOTPRequest
	// A client that sends no body at all still gets the field error below rather than a media type error.
	if r.ContentLength != 0 || r.Header.Get("Content-Type") != "" {
		if err := httpx.DecodeJSON(r, &req); err != nil {
			return err
		}
	}
	// Cancelling an enrolment that was never confirmed protects nothing, so only a working second step needs proof.
	if row.ConfirmedAt != nil {
		if err := s.reauthenticate(ctx, r, user.ID, req); err != nil {
			return err
		}
	}
	if err := s.DisableTOTP(ctx, user.ID); err != nil {
		if errors.Is(err, ErrNotEnabled) {
			return httpx.NotFound("two-step login is not enabled")
		}
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// reauthenticate asks for the current password, or for a current authenticator code when the account has no password.
func (s *Service) reauthenticate(ctx context.Context, r *http.Request, userID int64, req disableTOTPRequest) error {
	u, err := db.Q(s.DB.R).GetUserCredentialsByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("read user: %w", err)
	}
	field, given := "password", req.Password
	if u.PasswordHash == nil {
		field, given = "code", req.Code
	}
	if given == "" {
		return httpx.Validation("confirmation required", map[string]string{field: "required"})
	}
	if u.PasswordHash == nil {
		return s.ConfirmTOTPCode(ctx, r, userID, given, field)
	}
	return s.ConfirmPassword(ctx, r, userID, given, field)
}

type sessionJSON struct {
	ID         int64  `json:"id"`
	AuthMethod string `json:"auth_method"`
	UserAgent  string `json:"user_agent"`
	IPAddress  string `json:"ip_address"`
	CreatedAt  string `json:"created_at"`
	LastSeenAt string `json:"last_seen_at"`
	ExpiresAt  string `json:"expires_at"`
	Current    bool   `json:"current"`
}

func (s *Service) listSessions(w http.ResponseWriter, r *http.Request) error {
	user, _ := httpx.CurrentUser(r.Context())
	current, _ := SessionID(r.Context())
	rows, err := db.Q(s.DB.R).ListUserSessions(r.Context(), dbgen.ListUserSessionsParams{UserID: user.ID, ExpiresAt: formatTime(s.now())})
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}
	out := make([]sessionJSON, len(rows))
	for i, row := range rows {
		out[i] = sessionJSON{
			ID: row.ID, AuthMethod: row.AuthMethod, UserAgent: row.UserAgent, IPAddress: row.IpAddress,
			CreatedAt: row.CreatedAt, LastSeenAt: row.LastSeenAt, ExpiresAt: row.ExpiresAt, Current: row.ID == current,
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
	return nil
}

func (s *Service) revokeSession(w http.ResponseWriter, r *http.Request) error {
	if err := httpx.RequireJSONOrNoBody(r); err != nil {
		return err
	}
	user, _ := httpx.CurrentUser(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return httpx.NotFound("no such session")
	}
	n, err := db.Q(s.DB.W).DeleteUserSession(r.Context(), dbgen.DeleteUserSessionParams{ID: id, UserID: user.ID})
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	if n == 0 {
		return httpx.NotFound("no such session")
	}
	if current, ok := SessionID(r.Context()); ok && current == id {
		s.clearCookie(w, r)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
