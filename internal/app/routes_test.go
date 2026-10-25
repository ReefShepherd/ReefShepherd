package app

import (
	"net/http"
	"net/http/httptest"
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
	for _, target := range []string{"/api/v1/settings", "/api/v1/me", "/api/v1/units", "/api/v1/tanks", "/api/v1/does-not-exist"} {
		if rec := serve(http.MethodGet, target, "", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without a session: %d", target, rec.Code)
		}
	}
	// Public pages stay public.
	if rec := serve(http.MethodGet, "/healthz", "", ""); rec.Code != http.StatusOK {
		t.Errorf("healthz: %d", rec.Code)
	}
	if rec := serve(http.MethodGet, "/some/page", "", ""); rec.Code != http.StatusOK {
		t.Errorf("web ui: %d", rec.Code)
	}
}
