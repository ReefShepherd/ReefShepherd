package app

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/auth/password"
	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/settings"
	"github.com/reefshepherd/reefshepherd/internal/webui"
)

// TestRegisterRoutesWiresRealDepsIntoPackages drives the route table directly, so a package left with empty dependencies shows up as a failure here.
func TestRegisterRoutesWiresRealDepsIntoPackages(t *testing.T) {
	database := openTestDB(t)
	store := settings.NewStore(database)
	if err := store.SetInstanceName(t.Context(), "Wired Reef"); err != nil {
		t.Fatal(err)
	}
	authSvc := auth.NewService(auth.Deps{
		DB: database, Settings: store, Passwords: password.NewHasher(password.Params{Time: 1, MemoryKiB: 64, Threads: 1}),
		Config: config.Config{}, Logger: testLogger(),
	})
	mux := http.NewServeMux()
	registerRoutes(mux, routeDeps{Config: config.Config{}, Logger: testLogger(), DB: database, Settings: store, Auth: authSvc}, webui.NewHandler(fstest.MapFS{".gitkeep": {}}))

	serve := func(method, target, body, cookie string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, strings.NewReader(body))
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		}
		rec := httptest.NewRecorder()
		// The same panic guard the real stack has, so a missing dependency shows up as a 500 here.
		httpx.Recover(testLogger())(mux).ServeHTTP(rec, r)
		return rec
	}

	// auth reads the instance name from the store it was given.
	rec := serve(http.MethodGet, "/api/v1/auth/config", "", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"instance_name":"Wired Reef"`) || !strings.Contains(rec.Body.String(), `"setup_required":true`) {
		t.Errorf("auth config: %d %s", rec.Code, rec.Body)
	}

	// users writes through the database and hashes with the auth service's hasher.
	rec = serve(http.MethodPost, "/api/v1/setup", `{"username":"wired","password":"correct horse battery","instance_name":"Setup Named"}`, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup: %d %s", rec.Code, rec.Body)
	}
	var n int
	if err := database.R.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM users WHERE username = 'wired' AND password_hash LIKE '$argon2id$%'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("users with the new account = %d, %v", n, err)
	}
	var cookie string
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			cookie = c.Value
		}
	}
	if cookie == "" {
		t.Fatal("setup did not return a session")
	}

	// settings reads through the same store the other packages share and gets the sign-in check from auth.
	rec = serve(http.MethodGet, "/api/v1/settings", "", cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"instance_name":"Setup Named"`) {
		t.Errorf("settings: %d %s", rec.Code, rec.Body)
	}
	rec = serve(http.MethodGet, "/api/v1/me", "", cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"username":"wired"`) {
		t.Errorf("me: %d %s", rec.Code, rec.Body)
	}
	// Without the cookie everything behind the sign-in check is closed, including paths no package serves.
	for _, target := range []string{"/api/v1/settings", "/api/v1/me", "/api/v1/tanks", "/api/v1/does-not-exist"} {
		if rec := serve(http.MethodGet, target, "", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without a session: %d", target, rec.Code)
		}
	}
	// Public pages and the unit catalog stay public.
	if rec := serve(http.MethodGet, "/api/v1/units", "", ""); rec.Code != http.StatusOK {
		t.Errorf("units without a session: %d", rec.Code)
	}
	if rec := serve(http.MethodGet, "/healthz", "", ""); rec.Code != http.StatusOK {
		t.Errorf("healthz: %d", rec.Code)
	}
	if rec := serve(http.MethodGet, "/some/page", "", ""); rec.Code != http.StatusOK {
		t.Errorf("web ui: %d", rec.Code)
	}
}

// TestFeatureRoutesLogUnexpectedFailuresToTheAppLogger breaks each feature's tables so its read fails unexpectedly, and expects the failure in the app's own log.
func TestFeatureRoutesLogUnexpectedFailuresToTheAppLogger(t *testing.T) {
	var logs bytes.Buffer
	database := openTestDB(t)
	store := settings.NewStore(database)
	hasher := password.NewHasher(password.Params{Time: 1, MemoryKiB: 64, Threads: 1})
	app, err := newApp(config.Config{}, slog.New(slog.NewTextHandler(&logs, nil)), database, store, hasher, webui.Handler())
	if err != nil {
		t.Fatal(err)
	}
	a := &testApp{App: app, db: database, store: store}
	c := newClient(t, a)
	c.setup(nil)
	tank := mustCreateTank(t, c)
	tankPath := "/api/v1/tanks/" + strconv.FormatInt(tank.ID, 10)

	a.exec(t, `ALTER TABLE parameter_definitions RENAME TO parameter_definitions_gone`)
	a.exec(t, `ALTER TABLE dashboard_cards RENAME TO dashboard_cards_gone`)
	a.exec(t, `ALTER TABLE tanks RENAME TO tanks_gone`)
	for _, path := range []string{"/api/v1/parameters", tankPath + "/dashboard", "/api/v1/tanks"} {
		expect(t, c.do(http.MethodGet, path, nil), http.StatusInternalServerError, "internal")
		if !loggedFailure(logs.String(), path) {
			t.Errorf("no failure logged for %s in:\n%s", path, logs.String())
		}
	}
}

// loggedFailure reports whether log has an unexpected-failure line for path.
func loggedFailure(log, path string) bool {
	for _, line := range strings.Split(log, "\n") {
		if strings.Contains(line, `msg="request failed"`) && strings.Contains(line, "path="+path+" ") {
			return true
		}
	}
	return false
}
