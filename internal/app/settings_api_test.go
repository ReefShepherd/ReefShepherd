package app

import (
	"net/http"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

type settingsBody struct {
	InstanceName        string            `json:"instance_name"`
	Timezone            string            `json:"timezone"`
	ReminderDefaultTime string            `json:"reminder_default_time"`
	Units               map[string]string `json:"units"`
	AuthMode            string            `json:"auth_mode"`
	EnvLocked           map[string]string `json:"env_locked"`
}

func getSettings(t *testing.T, c *apiClient) settingsBody {
	t.Helper()
	rec := c.do(http.MethodGet, "/api/v1/settings", nil)
	testutil.ExpectStatus(t, rec, http.StatusOK)
	var s settingsBody
	testutil.DecodeInto(t, rec, &s)
	return s
}

func TestSettingsDefaultsAndUpdate(t *testing.T) {
	_, c := adminAndApp(t)
	s := getSettings(t, c)
	if s.InstanceName != "ReefShepherd" || s.Timezone != "UTC" || s.ReminderDefaultTime != "09:00" || s.AuthMode != "password" || len(s.EnvLocked) != 0 {
		t.Errorf("defaults = %+v", s)
	}
	for _, f := range []string{"temperature", "salinity", "alkalinity", "phosphate", "volume"} {
		if s.Units[f] == "" {
			t.Errorf("no unit for %s: %v", f, s.Units)
		}
	}
	if _, ok := s.Units["fixed"]; ok {
		t.Error("the fixed family has no display unit setting")
	}

	rec := c.do(http.MethodPatch, "/api/v1/settings", map[string]any{
		"instance_name": "Garage Reef", "timezone": "America/New_York", "reminder_default_time": "07:30",
		"units": map[string]string{"alkalinity": "meq_l", "salinity": "ppt"},
	})
	testutil.ExpectStatus(t, rec, http.StatusOK)
	testutil.DecodeInto(t, rec, &s)
	if s.InstanceName != "Garage Reef" || s.Timezone != "America/New_York" || s.ReminderDefaultTime != "07:30" || s.Units["alkalinity"] != "meq_l" || s.Units["salinity"] != "ppt" {
		t.Errorf("patched = %+v", s)
	}
	// Persisted, and the other units were not touched.
	again := getSettings(t, c)
	if again.InstanceName != "Garage Reef" || again.Units["temperature"] != s.Units["temperature"] {
		t.Errorf("again = %+v", again)
	}
	// An empty patch is fine and changes nothing.
	testutil.ExpectStatus(t, c.do(http.MethodPatch, "/api/v1/settings", map[string]any{}), http.StatusOK)
}

func TestSettingsPatchIsAllOrNothing(t *testing.T) {
	a, c := adminAndApp(t)
	b := testutil.ExpectError(t, c.do(http.MethodPatch, "/api/v1/settings", map[string]any{
		"instance_name": "Valid Name", "timezone": "Mars/Base", "reminder_default_time": "25:00",
		"units": map[string]string{"temperature": "kelvin", "nonsense": "x", "volume": "us_gal"},
	}), http.StatusUnprocessableEntity, "validation")
	for _, f := range []string{"timezone", "reminder_default_time", "units.temperature", "units.nonsense"} {
		if b.Error.Fields[f] == "" {
			t.Errorf("no message for %s: %v", f, b.Error.Fields)
		}
	}
	if b.Error.Fields["instance_name"] != "" || b.Error.Fields["units.volume"] != "" {
		t.Errorf("valid fields were reported: %v", b.Error.Fields)
	}
	s := getSettings(t, c)
	if s.InstanceName == "Valid Name" || s.Units["volume"] == "us_gal" {
		t.Errorf("a rejected patch was partly applied: %+v", s)
	}
	testutil.ExpectError(t, c.do(http.MethodPatch, "/api/v1/settings", map[string]any{"instance_name": "  "}), http.StatusUnprocessableEntity, "validation")
	testutil.ExpectError(t, c.do(http.MethodPatch, "/api/v1/settings", map[string]any{"mystery": true}), http.StatusUnprocessableEntity, "validation")
	_ = a
}

func TestUnitsCatalog(t *testing.T) {
	a, admin := adminAndApp(t)
	addUser(t, admin, "mia", "member")
	for _, c := range []*apiClient{admin, signedInAs(t, a, "mia")} {
		rec := c.do(http.MethodGet, "/api/v1/units", nil)
		testutil.ExpectStatus(t, rec, http.StatusOK)
		var catalog []struct {
			Family    string `json:"family"`
			Canonical string `json:"canonical"`
			Units     []struct {
				Unit  string `json:"unit"`
				Label string `json:"label"`
			} `json:"units"`
		}
		testutil.DecodeInto(t, rec, &catalog)
		if len(catalog) < 5 || catalog[0].Family != "temperature" || catalog[0].Canonical != "c" || len(catalog[0].Units) != 2 || catalog[0].Units[1].Label != "°F" {
			t.Errorf("catalog = %+v", catalog)
		}
	}
	anon := newClient(t, a).do(http.MethodGet, "/api/v1/units", nil)
	testutil.ExpectStatus(t, anon, http.StatusOK)
	if got := anon.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("anonymous catalog Cache-Control = %q", got)
	}
}

func TestTurningLoginOffNeedsTheAdminPassword(t *testing.T) {
	a, c := adminAndApp(t)
	off := map[string]any{"auth_mode": "disabled"}

	b := testutil.ExpectError(t, c.do(http.MethodPatch, "/api/v1/settings", off), http.StatusUnprocessableEntity, "validation")
	if b.Error.Fields["password"] == "" {
		t.Errorf("fields = %v", b.Error.Fields)
	}
	testutil.ExpectError(t, c.doWith(http.MethodPatch, "/api/v1/settings", map[string]any{"auth_mode": "disabled", "password": "the wrong one!!"}, nil), http.StatusUnprocessableEntity, "validation")
	if mode, _ := a.store.GetAuthMode(t.Context()); mode != "password" {
		t.Fatalf("mode changed to %q without the right password", mode)
	}
	// Anonymous callers are still locked out.
	testutil.ExpectError(t, newClient(t, a).do(http.MethodGet, "/api/v1/me", nil), http.StatusUnauthorized, "unauthorized")

	rec := c.do(http.MethodPatch, "/api/v1/settings", map[string]any{"auth_mode": "disabled", "password": adminPassword})
	testutil.ExpectStatus(t, rec, http.StatusOK)
	var s settingsBody
	testutil.DecodeInto(t, rec, &s)
	if s.AuthMode != "disabled" {
		t.Errorf("auth_mode = %q", s.AuthMode)
	}
	var me meBody
	testutil.DecodeInto(t, newClient(t, a).do(http.MethodGet, "/api/v1/me", nil), &me)
	if me.Username != adminName {
		t.Errorf("anonymous acts as %q", me.Username)
	}
	if id, _ := a.store.GetAuthDisabledUserID(t.Context()); id != me.ID {
		t.Errorf("disabled-mode user = %d", id)
	}
}

func TestWrongPasswordWhenTurningLoginOffIsRateLimited(t *testing.T) {
	_, c := adminAndApp(t)
	for range 5 {
		testutil.ExpectError(t, c.do(http.MethodPatch, "/api/v1/settings", map[string]any{"auth_mode": "disabled", "password": "the wrong one!!"}), http.StatusUnprocessableEntity, "validation")
	}
	rec := c.do(http.MethodPatch, "/api/v1/settings", map[string]any{"auth_mode": "disabled", "password": adminPassword})
	testutil.ExpectError(t, rec, http.StatusTooManyRequests, "too_many_requests")
	if rec.Header().Get("Retry-After") == "" {
		t.Error("no Retry-After header")
	}
}

func TestAdminWithoutAPasswordCannotTurnLoginOffOrOn(t *testing.T) {
	a := newAPITestApp(t, config.Config{})
	c := newClient(t, a)
	c.do(http.MethodPost, "/api/v1/setup", map[string]any{"auth_mode": "disabled"})

	// Login is already off; with nobody holding a password it cannot be switched on.
	b := testutil.ExpectError(t, c.do(http.MethodPatch, "/api/v1/settings", map[string]any{"auth_mode": "password"}), http.StatusUnprocessableEntity, "validation")
	if b.Error.Fields["auth_mode"] == "" {
		t.Errorf("fields = %v", b.Error.Fields)
	}
	if mode, _ := a.store.GetAuthMode(t.Context()); mode != "disabled" {
		t.Fatalf("mode = %q", mode)
	}

	// Setting a password first makes it possible.
	testutil.ExpectStatus(t, c.do(http.MethodPost, "/api/v1/me/password", map[string]string{"new_password": adminPassword}), http.StatusNoContent)
	testutil.ExpectStatus(t, c.do(http.MethodPatch, "/api/v1/settings", map[string]any{"auth_mode": "password"}), http.StatusOK)
	if id, _ := a.store.GetAuthDisabledUserID(t.Context()); id != 0 {
		t.Errorf("the disabled-mode user %d was not cleared", id)
	}
	testutil.ExpectError(t, newClient(t, a).do(http.MethodGet, "/api/v1/me", nil), http.StatusUnauthorized, "unauthorized")
	newClient(t, a).mustLogin("admin", adminPassword)
}

func TestEnvironmentOverrideLocksTheLoginMode(t *testing.T) {
	a := newAPITestApp(t, config.Config{AuthMode: config.AuthModePassword})
	c := newClient(t, a)
	c.setup(nil)

	s := getSettings(t, c)
	if s.AuthMode != "password" || s.EnvLocked["auth_mode"] != "REEFSHEPHERD_AUTH_MODE" {
		t.Errorf("settings = %+v", s)
	}
	b := testutil.ExpectError(t, c.do(http.MethodPatch, "/api/v1/settings", map[string]any{"auth_mode": "disabled", "password": adminPassword}), http.StatusConflict, "env_locked")
	if b.Error.Fields["auth_mode"] == "" {
		t.Errorf("fields = %v", b.Error.Fields)
	}
	if mode, _ := a.store.GetAuthMode(t.Context()); mode != "password" {
		t.Error("the locked mode was changed")
	}
	// Asking for the value already in force, or changing other settings, is fine.
	testutil.ExpectStatus(t, c.do(http.MethodPatch, "/api/v1/settings", map[string]any{"auth_mode": "password", "instance_name": "Locked But Named"}), http.StatusOK)
	if s := getSettings(t, c); s.InstanceName != "Locked But Named" {
		t.Errorf("instance name = %q", s.InstanceName)
	}
}

func TestEnvironmentOverrideWinsOverTheStoredMode(t *testing.T) {
	a := newAPITestApp(t, config.Config{AuthMode: config.AuthModeDisabled})
	c := newClient(t, a)
	c.setup(nil)
	// Setup followed the override; storing password mode afterwards must not bring login back, because the environment wins.
	if err := a.store.SetAuthMode(t.Context(), "password"); err != nil {
		t.Fatal(err)
	}
	anon := newClient(t, a)
	testutil.ExpectStatus(t, anon.do(http.MethodGet, "/api/v1/me", nil), http.StatusOK)
	s := getSettings(t, anon)
	if s.AuthMode != "disabled" || s.EnvLocked["auth_mode"] == "" {
		t.Errorf("settings = %+v", s)
	}
	testutil.ExpectError(t, anon.login(adminName, adminPassword), http.StatusConflict, "conflict")
}
