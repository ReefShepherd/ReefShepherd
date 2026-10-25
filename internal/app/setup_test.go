package app

import (
	"net/http"
	"sync"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/units"
)

type meBody struct {
	ID            int64   `json:"id"`
	Username      string  `json:"username"`
	DisplayName   string  `json:"display_name"`
	Email         *string `json:"email"`
	Role          string  `json:"role"`
	Disabled      bool    `json:"disabled"`
	HasPassword   bool    `json:"has_password"`
	TOTPEnabled   bool    `json:"totp_enabled"`
	Theme         string  `json:"theme"`
	Locale        string  `json:"locale"`
	DefaultTankID *int64  `json:"default_tank_id"`
}

func TestProtectedCallsSayWhenSetupIsPending(t *testing.T) {
	a := newAPITestApp(t, config.Config{})
	c := newClient(t, a)
	for _, p := range []string{"/api/v1/me", "/api/v1/settings", "/api/v1/users", "/api/v1/units", "/api/v1/me/sessions", "/api/v1/nothing"} {
		expect(t, c.do(http.MethodGet, p, nil), http.StatusConflict, "setup_required")
	}
	expect(t, c.do(http.MethodPost, "/api/v1/me/password", map[string]string{}), http.StatusConflict, "setup_required")

	c.setup(nil)
	anon := newClient(t, a)
	for _, p := range []string{"/api/v1/me", "/api/v1/settings", "/api/v1/users", "/api/v1/units", "/api/v1/me/sessions", "/api/v1/nothing"} {
		expect(t, anon.do(http.MethodGet, p, nil), http.StatusUnauthorized, "unauthorized")
	}
}

func TestPublicRoutesWorkWithoutASession(t *testing.T) {
	a := newAPITestApp(t, config.Config{})
	c := newClient(t, a)
	rec := c.do(http.MethodGet, "/api/v1/auth/config", nil)
	expectStatus(t, rec, http.StatusOK)
	var cfg struct {
		SetupRequired bool `json:"setup_required"`
	}
	decodeBody(t, rec, &cfg)
	if !cfg.SetupRequired {
		t.Error("setup_required should be true on a fresh instance")
	}
	c.setup(nil)
	newClient(t, a).do(http.MethodGet, "/api/v1/auth/config", nil)
	rec = newClient(t, a).do(http.MethodGet, "/api/v1/auth/config", nil)
	decodeBody(t, rec, &cfg)
	if cfg.SetupRequired {
		t.Error("setup_required should be false after setup")
	}
}

func TestSetupCreatesTheAdminSettingsAndSignsThemIn(t *testing.T) {
	a := newAPITestApp(t, config.Config{})
	c := newClient(t, a)
	rec := c.setup(map[string]any{
		"instance_name": "Reef HQ",
		"timezone":      "Europe/Berlin",
		"units":         map[string]string{"temperature": "f", "volume": "us_gal"},
		"theme":         "oled",
		"display_name":  "  Jay  ",
	})
	var me meBody
	decodeBody(t, rec, &me)
	if me.Username != adminName || me.Role != "admin" || me.Theme != "oled" || me.DisplayName != "Jay" || !me.HasPassword || me.Disabled || me.Locale != "en" {
		t.Errorf("setup response = %+v", me)
	}
	if rec.Result().Cookies() == nil || len(rec.Result().Cookies()) != 1 {
		t.Fatal("setup did not sign the admin in")
	}

	// The cookie from setup works for protected calls and shows the same user.
	var again meBody
	decodeBody(t, c.do(http.MethodGet, "/api/v1/me", nil), &again)
	if again.ID != me.ID || again.Theme != "oled" {
		t.Errorf("me = %+v", again)
	}

	defaultSalinity, err := a.store.GetUnit(t.Context(), units.Salinity)
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		InstanceName string            `json:"instance_name"`
		Timezone     string            `json:"timezone"`
		Units        map[string]string `json:"units"`
		AuthMode     string            `json:"auth_mode"`
	}
	decodeBody(t, c.do(http.MethodGet, "/api/v1/settings", nil), &s)
	if s.InstanceName != "Reef HQ" || s.Timezone != "Europe/Berlin" || s.Units["temperature"] != "f" || s.Units["volume"] != "us_gal" || s.Units["salinity"] != string(defaultSalinity) || s.AuthMode != "password" {
		t.Errorf("settings = %+v", s)
	}
	if done, _ := a.store.GetSetupCompleted(t.Context()); !done {
		t.Error("setup_completed was not recorded")
	}
	if a.count(t, `SELECT COUNT(*) FROM users WHERE password_hash LIKE '$argon2id$%'`) != 1 {
		t.Error("the password was not stored as an argon2id hash")
	}
	if a.count(t, `SELECT COUNT(*) FROM users WHERE password_hash LIKE ?`, "%"+adminPassword+"%") != 0 {
		t.Error("the password was stored in the clear")
	}
}

func TestSetupIsRefusedOnceCompleted(t *testing.T) {
	a := newAPITestApp(t, config.Config{})
	newClient(t, a).setup(nil)

	rec := newClient(t, a).do(http.MethodPost, "/api/v1/setup", map[string]any{"username": "mallory", "password": otherPassword})
	expect(t, rec, http.StatusConflict, "conflict")
	if len(rec.Result().Cookies()) != 0 {
		t.Error("a refused setup set a cookie")
	}
	if a.count(t, `SELECT COUNT(*) FROM users`) != 1 || a.count(t, `SELECT COUNT(*) FROM users WHERE username = 'mallory'`) != 0 {
		t.Error("a second user was created")
	}
	// Also when the body is garbage: the answer must not depend on the input.
	expect(t, newClient(t, a).do(http.MethodPost, "/api/v1/setup", map[string]any{"auth_mode": "disabled"}), http.StatusConflict, "conflict")
	// And the settings were not touched by the refused attempts.
	expect(t, newClient(t, a).do(http.MethodPost, "/api/v1/setup", map[string]any{"username": "x", "password": otherPassword, "instance_name": "Hijacked"}), http.StatusConflict, "conflict")
	if name, _ := a.store.GetInstanceName(t.Context()); name == "Hijacked" {
		t.Error("a refused setup changed the instance name")
	}
}

func TestSetupIsRefusedWhenUsersAlreadyExist(t *testing.T) {
	a := newAPITestApp(t, config.Config{})
	a.exec(t, `INSERT INTO users (username, role) VALUES ('existing', 'admin')`)
	expect(t, newClient(t, a).do(http.MethodPost, "/api/v1/setup", map[string]any{"username": "mallory", "password": otherPassword}), http.StatusConflict, "conflict")
	if a.count(t, `SELECT COUNT(*) FROM users`) != 1 {
		t.Error("setup added a user next to an existing one")
	}
	// Protected calls report a plain 401 rather than sending people to a setup that would refuse them.
	expect(t, newClient(t, a).do(http.MethodGet, "/api/v1/me", nil), http.StatusUnauthorized, "unauthorized")
}

func TestOnlyOneConcurrentSetupWins(t *testing.T) {
	a := newAPITestApp(t, config.Config{})
	const n = 8
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := newClient(t, a)
			name := "racer" + string(rune('a'+i))
			codes[i] = c.do(http.MethodPost, "/api/v1/setup", map[string]any{"username": name, "password": adminPassword}).Code
		}()
	}
	wg.Wait()
	created, refused := 0, 0
	for _, code := range codes {
		switch code {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			refused++
		default:
			t.Errorf("unexpected status %d", code)
		}
	}
	if created != 1 || refused != n-1 {
		t.Errorf("created=%d refused=%d, want 1 and %d", created, refused, n-1)
	}
	if a.count(t, `SELECT COUNT(*) FROM users`) != 1 {
		t.Errorf("%d users after racing setups", a.count(t, `SELECT COUNT(*) FROM users`))
	}
}

func TestSetupValidationRejectsEverythingAtOnceAndWritesNothing(t *testing.T) {
	a := newAPITestApp(t, config.Config{})
	c := newClient(t, a)
	rec := c.do(http.MethodPost, "/api/v1/setup", map[string]any{
		"username": "x", "password": "short", "theme": "neon", "auth_mode": "magic",
		"timezone": "Mars/Olympus", "units": map[string]string{"temperature": "kelvin", "nonsense": "x"},
		"instance_name": "   ",
	})
	b := expect(t, rec, http.StatusUnprocessableEntity, "validation")
	for _, f := range []string{"username", "password", "theme", "auth_mode", "timezone", "units.temperature", "units.nonsense", "instance_name"} {
		if b.Error.Fields[f] == "" {
			t.Errorf("no message for field %q: %v", f, b.Error.Fields)
		}
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Error("a failed setup set a cookie")
	}
	if a.count(t, `SELECT COUNT(*) FROM users`) != 0 || a.count(t, `SELECT COUNT(*) FROM settings`) != 0 {
		t.Error("a failed setup left rows behind")
	}
	// Still open for a correct attempt.
	c.setup(nil)
}

func TestSetupRollsBackWhenALateStepFails(t *testing.T) {
	a := newAPITestApp(t, config.Config{})
	// The final write of setup fails, after the admin, preferences and settings were already written.
	a.exec(t, `CREATE TRIGGER fail_setup_flag BEFORE INSERT ON settings WHEN NEW.key = 'setup_completed' BEGIN SELECT RAISE(ABORT, 'disk full'); END`)
	rec := newClient(t, a).do(http.MethodPost, "/api/v1/setup", map[string]any{
		"username": adminName, "password": adminPassword, "instance_name": "Half Done", "timezone": "UTC",
	})
	expect(t, rec, http.StatusInternalServerError, "internal")
	if len(rec.Result().Cookies()) != 0 {
		t.Error("a failed setup set a cookie")
	}
	if a.count(t, `SELECT COUNT(*) FROM users`) != 0 || a.count(t, `SELECT COUNT(*) FROM user_preferences`) != 0 || a.count(t, `SELECT COUNT(*) FROM settings`) != 0 {
		t.Error("the writes before the failing step were not rolled back")
	}
	a.exec(t, `DROP TRIGGER fail_setup_flag`)
	newClient(t, a).setup(nil)
}

func TestSetupInputShapeErrors(t *testing.T) {
	a := newAPITestApp(t, config.Config{})
	c := newClient(t, a)
	expect(t, c.do(http.MethodPost, "/api/v1/setup", map[string]any{"surprise": 1}), http.StatusUnprocessableEntity, "validation")
	expect(t, c.do(http.MethodPost, "/api/v1/setup", nil), http.StatusUnsupportedMediaType, "unsupported_media_type")
	expect(t, c.do(http.MethodPost, "/api/v1/setup", map[string]any{"username": "ok", "password": ""}), http.StatusUnprocessableEntity, "validation")
	if a.count(t, `SELECT COUNT(*) FROM users`) != 0 {
		t.Error("a user was created from a bad request")
	}
}

func TestSetupWithoutLogin(t *testing.T) {
	a := newAPITestApp(t, config.Config{})
	c := newClient(t, a)
	rec := c.do(http.MethodPost, "/api/v1/setup", map[string]any{"auth_mode": "disabled", "timezone": "UTC"})
	expectStatus(t, rec, http.StatusCreated)
	if len(rec.Result().Cookies()) != 0 {
		t.Error("no-login setup should not create a session")
	}
	var me meBody
	decodeBody(t, rec, &me)
	if me.Username != "admin" || me.Role != "admin" || me.HasPassword {
		t.Errorf("admin = %+v", me)
	}
	if a.count(t, `SELECT COUNT(*) FROM users WHERE password_hash IS NULL`) != 1 || a.count(t, `SELECT COUNT(*) FROM sessions`) != 0 {
		t.Error("expected a passwordless admin and no sessions")
	}
	if mode, _ := a.store.GetAuthMode(t.Context()); mode != "disabled" {
		t.Errorf("mode = %q", mode)
	}
	if id, _ := a.store.GetAuthDisabledUserID(t.Context()); id != me.ID {
		t.Errorf("disabled-mode user = %d, want %d", id, me.ID)
	}

	// A browser with no cookie is the admin now.
	var who meBody
	decodeBody(t, newClient(t, a).do(http.MethodGet, "/api/v1/me", nil), &who)
	if who.ID != me.ID {
		t.Errorf("anonymous acts as %d", who.ID)
	}
	expect(t, newClient(t, a).login("admin", adminPassword), http.StatusConflict, "conflict")
}

func TestSetupWithoutLoginAcceptsAChosenUsername(t *testing.T) {
	a := newAPITestApp(t, config.Config{})
	var me meBody
	decodeBody(t, newClient(t, a).do(http.MethodPost, "/api/v1/setup", map[string]any{"auth_mode": "disabled", "username": "reefer"}), &me)
	if me.Username != "reefer" {
		t.Errorf("username = %q", me.Username)
	}
}

func TestSetupHonoursTheEnvironmentOverride(t *testing.T) {
	a := newAPITestApp(t, config.Config{AuthMode: config.AuthModePassword})
	c := newClient(t, a)
	b := expect(t, c.do(http.MethodPost, "/api/v1/setup", map[string]any{"auth_mode": "disabled"}), http.StatusConflict, "env_locked")
	if b.Error.Fields["auth_mode"] == "" {
		t.Errorf("body = %+v", b)
	}
	if a.count(t, `SELECT COUNT(*) FROM users`) != 0 {
		t.Error("a user was created")
	}
	// Leaving the mode out follows the override.
	c.setup(nil)

	b2 := newAPITestApp(t, config.Config{AuthMode: config.AuthModeDisabled})
	rec := newClient(t, b2).do(http.MethodPost, "/api/v1/setup", map[string]any{})
	expectStatus(t, rec, http.StatusCreated)
	if mode, _ := b2.store.GetAuthMode(t.Context()); mode != "disabled" {
		t.Errorf("mode = %q, want disabled from the override", mode)
	}
}

func TestSetupWithAPasswordInNoLoginModeKeepsIt(t *testing.T) {
	a := newAPITestApp(t, config.Config{})
	var me meBody
	decodeBody(t, newClient(t, a).do(http.MethodPost, "/api/v1/setup", map[string]any{"auth_mode": "disabled", "password": adminPassword}), &me)
	if !me.HasPassword {
		t.Error("the supplied password was dropped")
	}
}
