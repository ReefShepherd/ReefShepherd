package auth_test

import (
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"net/netip"
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
	"github.com/reefshepherd/reefshepherd/internal/testutil"
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
	database := testutil.OpenDB(t)
	e := &env{t: t, db: database, store: settings.NewStore(database), hasher: password.NewHasher(params), now: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)}
	e.svc = auth.NewService(auth.Deps{DB: database, Settings: e.store, Passwords: e.hasher, Config: cfg, Logger: testutil.Logger(t)})
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
	return testutil.Insert(e.t, e.db, `INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)`, username, hash, role)
}

func tokenHashCount(e *env, token string) int {
	sum := sha256.Sum256([]byte(token))
	return testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM sessions WHERE token_hash = ?`, sum[:])
}

// resp is a recorded response.
type resp struct {
	*httptest.ResponseRecorder
	t *testing.T
}

func (r *resp) decode(v any) {
	r.t.Helper()
	testutil.DecodeInto(r.t, r.ResponseRecorder, v)
}

func (r *resp) expect(status int, code string) {
	r.t.Helper()
	testutil.ExpectError(r.t, r.ResponseRecorder, status, code)
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
	rec := testutil.DoJSON(c.e.t, c.e.h, method, path, body,
		func(r *http.Request) { r.RemoteAddr = c.ip + ":5000" },
		testutil.WithHeader("User-Agent", "test-browser"),
		func(r *http.Request) {
			for k, vs := range c.header {
				r.Header[k] = vs
			}
			if c.cookie != "" {
				r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: c.cookie})
			}
		},
	)
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
