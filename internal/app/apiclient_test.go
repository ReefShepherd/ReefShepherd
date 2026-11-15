package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/auth/password"
	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/settings"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
	"github.com/reefshepherd/reefshepherd/internal/webui"
)

const (
	adminName     = "admin1"
	adminPassword = "correct horse battery"
	otherPassword = "another long passphrase"
)

// testApp is the full application plus the database it runs on, for direct inspection.
type testApp struct {
	*App
	db    *db.DB
	store *settings.Store
}

// newAPITestApp builds the full application on a temporary database with cheap password hashing.
func newAPITestApp(t *testing.T, cfg config.Config) *testApp {
	t.Helper()
	database := openTestDB(t)
	store := settings.NewStore(database)
	hasher := password.NewHasher(password.Params{Time: 1, MemoryKiB: 64, Threads: 1})
	a, err := newApp(cfg, testutil.Logger(t), database, store, hasher, webui.Handler(testutil.Logger(t)))
	if err != nil {
		t.Fatal(err)
	}
	return &testApp{App: a, db: database, store: store}
}

// apiClient is one browser talking to the app through the whole middleware stack.
type apiClient struct {
	t       *testing.T
	a       *testApp
	cookie  string
	headers map[string]string
}

func newClient(t *testing.T, a *testApp) *apiClient {
	t.Helper()
	return &apiClient{t: t, a: a, headers: map[string]string{}}
}

func (c *apiClient) raw(method, path, contentType, body string, extra map[string]string) *httptest.ResponseRecorder {
	c.t.Helper()
	rec := testutil.DoRaw(c.t, c.a.Handler(), method, path, contentType, body, func(r *http.Request) {
		r.RemoteAddr = "192.0.2.10:4000"
		for k, v := range c.headers {
			r.Header.Set(k, v)
		}
		for k, v := range extra {
			r.Header.Set(k, v)
		}
		if c.cookie != "" {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: c.cookie})
		}
	})
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == auth.CookieName {
			if ck.MaxAge < 0 || ck.Value == "" {
				c.cookie = ""
			} else {
				c.cookie = ck.Value
			}
		}
	}
	return rec
}

func (c *apiClient) do(method, path string, body any) *httptest.ResponseRecorder {
	c.t.Helper()
	if body == nil {
		return c.raw(method, path, "", "", nil)
	}
	b, err := json.Marshal(body)
	if err != nil {
		c.t.Fatal(err)
	}
	return c.raw(method, path, "application/json", string(b), nil)
}

// doWith is do with extra request headers.
func (c *apiClient) doWith(method, path string, body any, extra map[string]string) *httptest.ResponseRecorder {
	c.t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		c.t.Fatal(err)
	}
	return c.raw(method, path, "application/json", string(b), extra)
}

// setup completes first-run setup with defaults, overridden by over, and returns the response.
func (c *apiClient) setup(over map[string]any) *httptest.ResponseRecorder {
	c.t.Helper()
	body := map[string]any{"username": adminName, "password": adminPassword}
	for k, v := range over {
		body[k] = v
	}
	rec := c.do(http.MethodPost, "/api/v1/setup", body)
	if rec.Code != http.StatusCreated {
		c.t.Fatalf("setup: %d %s", rec.Code, rec.Body)
	}
	return rec
}

func (c *apiClient) login(username, pw string) *httptest.ResponseRecorder {
	return c.do(http.MethodPost, "/api/v1/auth/login", map[string]string{"username": username, "password": pw})
}

func (c *apiClient) mustLogin(username, pw string) {
	c.t.Helper()
	if rec := c.login(username, pw); rec.Code != http.StatusOK {
		c.t.Fatalf("login %s: %d %s", username, rec.Code, rec.Body)
	}
}

// totpCode is the authenticator code for secret right now.
func totpCode(t *testing.T, secret string) string {
	t.Helper()
	code, err := totp.GenerateCodeCustom(secret, time.Now(), totp.ValidateOpts{Period: 30, Skew: 1, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		t.Fatal(err)
	}
	return code
}

// enableTOTP turns on two-step login for the signed-in client and returns the secret.
func (c *apiClient) enableTOTP() string {
	c.t.Helper()
	rec := c.do(http.MethodPost, "/api/v1/me/totp/enroll", nil)
	testutil.ExpectStatus(c.t, rec, http.StatusOK)
	var e struct {
		Secret string `json:"secret"`
	}
	testutil.DecodeInto(c.t, rec, &e)
	testutil.ExpectStatus(c.t, c.do(http.MethodPost, "/api/v1/me/totp/confirm", map[string]string{"code": totpCode(c.t, e.Secret)}), http.StatusOK)
	return e.Secret
}
