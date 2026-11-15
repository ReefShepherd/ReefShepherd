package auth_test

import (
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/auth/password"
	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/settings"
)

// testParams keeps hashing cheap in tests.
var testParams = password.Params{Time: 1, MemoryKiB: 64, Threads: 1}

const goodPassword = "correct horse battery"

type env struct {
	t      *testing.T
	db     *db.DB
	store  *settings.Store
	svc    *auth.Service
	hasher *password.Hasher
	h      http.Handler
	now    time.Time
}

// newEnv serves the auth routes, plus two probe routes behind the sign-in check, on a real temporary database.
func newEnv(t *testing.T, cfg config.Config) *env {
	t.Helper()
	return newEnvParams(t, cfg, testParams)
}

// newEnvParams is newEnv with the password hashing cost chosen by the test.
func newEnvParams(t *testing.T, cfg config.Config, params password.Params) *env {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "reef.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(t.Context(), filepath.Join(t.TempDir(), "backups")); err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, db: database, store: settings.NewStore(database), hasher: password.NewHasher(params), now: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)}
	e.svc = auth.NewService(auth.Deps{DB: database, Settings: e.store, Passwords: e.hasher, Config: cfg})
	e.svc.SetClock(func() time.Time { return e.now })

	mux := http.NewServeMux()
	e.svc.PublicRoutes(mux)
	protected := http.NewServeMux()
	e.svc.Routes(protected)
	protected.HandleFunc("GET /api/v1/whoami", func(w http.ResponseWriter, r *http.Request) {
		u, _ := httpx.CurrentUser(r.Context())
		sid, _ := auth.SessionID(r.Context())
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": u.ID, "username": u.Username, "role": u.Role, "session_id": sid})
	})
	protected.Handle("GET /api/v1/admin-only", auth.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"ok": "yes"})
	})))
	mux.Handle("/api/", e.svc.RequireUser(protected))
	e.h = mux
	return e
}

func (e *env) exec(query string, args ...any) {
	e.t.Helper()
	if _, err := e.db.W.ExecContext(e.t.Context(), query, args...); err != nil {
		e.t.Fatalf("%s: %v", query, err)
	}
}

func (e *env) count(query string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.db.R.QueryRowContext(e.t.Context(), query, args...).Scan(&n); err != nil {
		e.t.Fatalf("%s: %v", query, err)
	}
	return n
}

// addUser inserts an account; an empty pw leaves the password unset.
func (e *env) addUser(username, pw, role string) int64 {
	e.t.Helper()
	var hash any
	if pw != "" {
		h, err := e.hasher.Hash(e.t.Context(), pw)
		if err != nil {
			e.t.Fatal(err)
		}
		hash = h
	}
	res, err := e.db.W.ExecContext(e.t.Context(), `INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)`, username, hash, role)
	if err != nil {
		e.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func tokenHashCount(e *env, token string) int {
	sum := sha256.Sum256([]byte(token))
	return e.count(`SELECT COUNT(*) FROM sessions WHERE token_hash = ?`, sum[:])
}

// resp is a recorded response.
type resp struct {
	*httptest.ResponseRecorder
	t *testing.T
}

func (r *resp) decode(v any) {
	r.t.Helper()
	if err := json.Unmarshal(r.Body.Bytes(), v); err != nil {
		r.t.Fatalf("body %q: %v", r.Body.String(), err)
	}
}

// errCode returns the code of the JSON error body, or "" when there is none.
func (r *resp) errCode() string {
	var b struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(r.Body.Bytes(), &b)
	return b.Error.Code
}

func (r *resp) expect(status int, code string) {
	r.t.Helper()
	if r.Code != status || r.errCode() != code {
		r.t.Fatalf("status=%d code=%q, want %d %q; body=%s", r.Code, r.errCode(), status, code, r.Body.String())
	}
}

func (r *resp) sessionCookie() *http.Cookie {
	for _, c := range r.Result().Cookies() {
		if c.Name == auth.CookieName {
			return c
		}
	}
	return nil
}

// client is one browser: it keeps the session cookie between requests.
type client struct {
	e      *env
	cookie string
	ip     string
	header http.Header
}

func (e *env) client() *client {
	return &client{e: e, ip: "192.0.2.10", header: http.Header{}}
}

func (c *client) do(method, path string, body any) *resp {
	c.e.t.Helper()
	var rd *strings.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.e.t.Fatal(err)
		}
		rd = strings.NewReader(string(b))
	} else {
		rd = strings.NewReader("")
	}
	r := httptest.NewRequest(method, path, rd)
	r.RemoteAddr = c.ip + ":5000"
	r.Header.Set("User-Agent", "test-browser")
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	for k, vs := range c.header {
		r.Header[k] = vs
	}
	if c.cookie != "" {
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: c.cookie})
	}
	rec := httptest.NewRecorder()
	c.e.h.ServeHTTP(rec, r)
	out := &resp{ResponseRecorder: rec, t: c.e.t}
	if sc := out.sessionCookie(); sc != nil {
		if sc.MaxAge < 0 || sc.Value == "" {
			c.cookie = ""
		} else {
			c.cookie = sc.Value
		}
	}
	return out
}

func (c *client) login(username, pw string) *resp {
	return c.do(http.MethodPost, "/api/v1/auth/login", map[string]string{"username": username, "password": pw})
}

// mustLogin signs in and fails the test unless a full session results.
func (c *client) mustLogin(username, pw string) {
	c.e.t.Helper()
	r := c.login(username, pw)
	if r.Code != http.StatusOK {
		c.e.t.Fatalf("login %s: %d %s", username, r.Code, r.Body.String())
	}
	var out struct {
		MFARequired bool `json:"mfa_required"`
	}
	r.decode(&out)
	if out.MFARequired {
		c.e.t.Fatalf("login %s asked for a second step", username)
	}
}

func (c *client) whoami() *resp { return c.do(http.MethodGet, "/api/v1/whoami", nil) }

func prefix(s string) netip.Prefix { return netip.MustParsePrefix(s) }

// codeAt is the authenticator app's code for secret at time t.
func codeAt(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	code, err := totp.GenerateCodeCustom(secret, at, totp.ValidateOpts{Period: 30, Skew: 1, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		t.Fatal(err)
	}
	return code
}

// enableTOTP runs the whole enrolment for a signed-in client and returns the secret and recovery codes.
func (c *client) enableTOTP() (secret string, recovery []string) {
	c.e.t.Helper()
	r := c.do(http.MethodPost, "/api/v1/me/totp/enroll", nil)
	if r.Code != http.StatusOK {
		c.e.t.Fatalf("enroll: %d %s", r.Code, r.Body.String())
	}
	var enroll struct {
		Secret string `json:"secret"`
	}
	r.decode(&enroll)
	r = c.do(http.MethodPost, "/api/v1/me/totp/confirm", map[string]string{"code": codeAt(c.e.t, enroll.Secret, c.e.now)})
	if r.Code != http.StatusOK {
		c.e.t.Fatalf("confirm: %d %s", r.Code, r.Body.String())
	}
	var confirm struct {
		RecoveryCodes []string `json:"recovery_codes"`
	}
	r.decode(&confirm)
	return enroll.Secret, confirm.RecoveryCodes
}
