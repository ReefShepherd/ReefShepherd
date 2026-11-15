package app

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/units"
)

// adminAndApp returns a set-up instance with the admin signed in.
func adminAndApp(t *testing.T) (*testApp, *apiClient) {
	t.Helper()
	a := newAPITestApp(t, config.Config{})
	c := newClient(t, a)
	c.setup(nil)
	return a, c
}

// addUser creates an account through the admin API and returns its id.
func addUser(t *testing.T, admin *apiClient, username, role string) int64 {
	t.Helper()
	rec := admin.do(http.MethodPost, "/api/v1/users", map[string]any{"username": username, "password": otherPassword, "role": role})
	expectStatus(t, rec, http.StatusCreated)
	var u meBody
	decodeBody(t, rec, &u)
	return u.ID
}

// signedInAs returns a new browser signed in as an account made with addUser.
func signedInAs(t *testing.T, a *testApp, username string) *apiClient {
	t.Helper()
	c := newClient(t, a)
	c.mustLogin(username, otherPassword)
	return c
}

func TestMeShowsTheUserAndPreferences(t *testing.T) {
	_, c := adminAndApp(t)
	var me meBody
	rec := c.do(http.MethodGet, "/api/v1/me", nil)
	decodeBody(t, rec, &me)
	if rec.Code != 200 || me.Username != adminName || me.Role != "admin" || me.Theme != "system" || me.Locale != "en" || me.DefaultTankID != nil || me.TOTPEnabled || !me.HasPassword {
		t.Errorf("me = %+v", me)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("/me must not be cached")
	}
	// Never leaks credentials.
	body := rec.Body.String()
	for _, banned := range []string{"password_hash", "argon2", "secret", "token"} {
		if containsFold(body, banned) {
			t.Errorf("/me body mentions %q: %s", banned, body)
		}
	}
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

// userPath is the URL of a user resource, with an optional suffix such as "/reset-mfa".
func userPath(id int64, suffix string) string {
	return "/api/v1/users/" + strconv.FormatInt(id, 10) + suffix
}

func TestPatchMeUpdatesProfileAndPreferences(t *testing.T) {
	a, c := adminAndApp(t)
	a.exec(t, `INSERT INTO tanks (id, name) VALUES (7, 'Display')`)

	var me meBody
	rec := c.do(http.MethodPatch, "/api/v1/me", map[string]any{
		"display_name": "  Jay Reef ", "email": "jay@example.com", "theme": "dark", "locale": "de", "default_tank_id": 7,
	})
	decodeBody(t, rec, &me)
	if rec.Code != 200 || me.DisplayName != "Jay Reef" || me.Email == nil || *me.Email != "jay@example.com" || me.Theme != "dark" || me.Locale != "de" || me.DefaultTankID == nil || *me.DefaultTankID != 7 {
		t.Fatalf("patch: %d %+v", rec.Code, me)
	}

	// Fields left out stay as they were.
	decodeBody(t, c.do(http.MethodPatch, "/api/v1/me", map[string]any{"theme": "light"}), &me)
	if me.Theme != "light" || me.Locale != "de" || me.DisplayName != "Jay Reef" || me.Email == nil || me.DefaultTankID == nil {
		t.Errorf("partial patch changed other fields: %+v", me)
	}

	// null clears the nullable ones, a blank email also clears.
	decodeBody(t, c.do(http.MethodPatch, "/api/v1/me", map[string]any{"default_tank_id": nil, "email": nil}), &me)
	if me.DefaultTankID != nil || me.Email != nil {
		t.Errorf("null did not clear: %+v", me)
	}
	decodeBody(t, c.do(http.MethodPatch, "/api/v1/me", map[string]any{"email": "again@example.com"}), &me)
	decodeBody(t, c.do(http.MethodPatch, "/api/v1/me", map[string]any{"email": "  "}), &me)
	if me.Email != nil {
		t.Errorf("blank email kept: %v", *me.Email)
	}

	var again meBody
	decodeBody(t, c.do(http.MethodGet, "/api/v1/me", nil), &again)
	if again.Theme != "light" || again.Locale != "de" {
		t.Errorf("not persisted: %+v", again)
	}
}

func TestPatchMeValidates(t *testing.T) {
	_, c := adminAndApp(t)
	b := expect(t, c.do(http.MethodPatch, "/api/v1/me", map[string]any{
		"display_name": string(make([]rune, 101)), "email": "not-an-email", "theme": "neon", "locale": "English", "default_tank_id": 999,
	}), http.StatusUnprocessableEntity, "validation")
	for _, f := range []string{"display_name", "email", "theme", "locale", "default_tank_id"} {
		if b.Error.Fields[f] == "" {
			t.Errorf("no message for %s: %v", f, b.Error.Fields)
		}
	}
	expect(t, c.do(http.MethodPatch, "/api/v1/me", map[string]any{"role": "admin"}), http.StatusUnprocessableEntity, "validation")
	expect(t, c.do(http.MethodPatch, "/api/v1/me", map[string]any{"username": "other"}), http.StatusUnprocessableEntity, "validation")
	expect(t, c.do(http.MethodPatch, "/api/v1/me", map[string]any{"default_tank_id": "seven"}), http.StatusUnprocessableEntity, "validation")
	// One bad field means nothing is saved.
	var me meBody
	expect(t, c.do(http.MethodPatch, "/api/v1/me", map[string]any{"display_name": "Saved?", "theme": "neon"}), http.StatusUnprocessableEntity, "validation")
	decodeBody(t, c.do(http.MethodGet, "/api/v1/me", nil), &me)
	if me.DisplayName == "Saved?" {
		t.Error("a rejected patch was partly applied")
	}
}

func TestUserManagementIsAdminOnly(t *testing.T) {
	a, admin := adminAndApp(t)
	addUser(t, admin, "mia", "member")
	member := signedInAs(t, a, "mia")
	anon := newClient(t, a)

	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/users"},
		{http.MethodPost, "/api/v1/users"},
		{http.MethodPatch, "/api/v1/users/1"},
		{http.MethodDelete, "/api/v1/users/1"},
		{http.MethodPost, "/api/v1/users/1/reset-mfa"},
		{http.MethodGet, "/api/v1/settings"},
		{http.MethodPatch, "/api/v1/settings"},
	}
	for _, r := range routes {
		expect(t, member.do(r.method, r.path, map[string]any{}), http.StatusForbidden, "forbidden")
		expect(t, anon.do(r.method, r.path, map[string]any{}), http.StatusUnauthorized, "unauthorized")
	}
	// Members still reach what is theirs.
	expectStatus(t, member.do(http.MethodGet, "/api/v1/me", nil), http.StatusOK)
	expectStatus(t, member.do(http.MethodGet, "/api/v1/units", nil), http.StatusOK)
	if a.count(t, `SELECT COUNT(*) FROM users`) != 2 {
		t.Error("a forbidden call changed the users")
	}
}

func TestAdminCreatesAndListsUsers(t *testing.T) {
	a, admin := adminAndApp(t)
	rec := admin.do(http.MethodPost, "/api/v1/users", map[string]any{
		"username": "Mia", "display_name": "Mia R", "email": "mia@example.com", "password": otherPassword,
	})
	expectStatus(t, rec, http.StatusCreated)
	var u meBody
	decodeBody(t, rec, &u)
	if u.Username != "Mia" || u.Role != "member" || u.DisplayName != "Mia R" || !u.HasPassword || u.Disabled {
		t.Errorf("created = %+v", u)
	}
	signedInAs(t, a, "mia") // usernames are case-insensitive

	var list []meBody
	decodeBody(t, admin.do(http.MethodGet, "/api/v1/users", nil), &list)
	if len(list) != 2 || list[0].Username != adminName || list[1].Username != "Mia" {
		t.Errorf("list = %+v", list)
	}
	if body := admin.do(http.MethodGet, "/api/v1/users", nil).Body.String(); containsFold(body, "argon2") || containsFold(body, "password_hash") {
		t.Errorf("list leaks credentials: %s", body)
	}
}

func TestCreateUserValidation(t *testing.T) {
	a, admin := adminAndApp(t)
	addUser(t, admin, "mia", "member")

	b := expect(t, admin.do(http.MethodPost, "/api/v1/users", map[string]any{"username": "MIA", "password": otherPassword}), http.StatusUnprocessableEntity, "validation")
	if b.Error.Fields["username"] == "" {
		t.Errorf("duplicate username (different case) accepted: %v", b.Error.Fields)
	}
	b = expect(t, admin.do(http.MethodPost, "/api/v1/users", map[string]any{"username": "a b", "password": "short", "role": "root", "email": "x"}), http.StatusUnprocessableEntity, "validation")
	for _, f := range []string{"username", "password", "role", "email"} {
		if b.Error.Fields[f] == "" {
			t.Errorf("no message for %s", f)
		}
	}
	expect(t, admin.do(http.MethodPost, "/api/v1/users", map[string]any{"username": "nopass"}), http.StatusUnprocessableEntity, "validation")
	if a.count(t, `SELECT COUNT(*) FROM users`) != 2 {
		t.Error("an invalid request created a user")
	}
}

func TestUpdateUser(t *testing.T) {
	_, admin := adminAndApp(t)
	id := addUser(t, admin, "mia", "member")

	var u meBody
	rec := admin.do(http.MethodPatch, userPath(id, ""), map[string]any{"display_name": "Mia R", "email": "mia@example.com", "role": "admin"})
	decodeBody(t, rec, &u)
	if rec.Code != 200 || u.DisplayName != "Mia R" || u.Role != "admin" || u.Email == nil {
		t.Fatalf("patch: %d %+v", rec.Code, u)
	}
	decodeBody(t, admin.do(http.MethodPatch, userPath(id, ""), map[string]any{"email": nil, "role": "member"}), &u)
	if u.Email != nil || u.Role != "member" || u.DisplayName != "Mia R" {
		t.Errorf("after second patch: %+v", u)
	}

	b := expect(t, admin.do(http.MethodPatch, userPath(id, ""), map[string]any{"role": "root", "email": "bad", "password": "short"}), http.StatusUnprocessableEntity, "validation")
	if b.Error.Fields["password"] == "" {
		t.Errorf("fields = %v", b.Error.Fields)
	}
	expect(t, admin.do(http.MethodPatch, userPath(id, ""), map[string]any{"role": "root"}), http.StatusUnprocessableEntity, "validation")
	expect(t, admin.do(http.MethodPatch, "/api/v1/users/9999", map[string]any{"role": "member"}), http.StatusNotFound, "not_found")
	expect(t, admin.do(http.MethodPatch, "/api/v1/users/abc", map[string]any{"role": "member"}), http.StatusNotFound, "not_found")
	expect(t, admin.do(http.MethodPatch, userPath(id, ""), map[string]any{"username": "renamed"}), http.StatusUnprocessableEntity, "validation")
}

func TestAdminSettingAPasswordEndsThatUsersSessions(t *testing.T) {
	a, admin := adminAndApp(t)
	id := addUser(t, admin, "mia", "member")
	phone, laptop := signedInAs(t, a, "mia"), signedInAs(t, a, "mia")

	expectStatus(t, admin.do(http.MethodPatch, userPath(id, ""), map[string]any{"password": "a brand new passphrase"}), http.StatusOK)
	expect(t, phone.do(http.MethodGet, "/api/v1/me", nil), http.StatusUnauthorized, "unauthorized")
	expect(t, laptop.do(http.MethodGet, "/api/v1/me", nil), http.StatusUnauthorized, "unauthorized")
	newClient(t, a).mustLogin("mia", "a brand new passphrase")
	expect(t, newClient(t, a).login("mia", otherPassword), http.StatusUnauthorized, "invalid_credentials")
	// The admin's own session is untouched.
	expectStatus(t, admin.do(http.MethodGet, "/api/v1/me", nil), http.StatusOK)
}

func TestDisablingAUserEndsTheirSessionsAndBlocksLogin(t *testing.T) {
	a, admin := adminAndApp(t)
	id := addUser(t, admin, "mia", "member")
	mia := signedInAs(t, a, "mia")

	expectStatus(t, admin.do(http.MethodPatch, userPath(id, ""), map[string]any{"disabled": true}), http.StatusOK)
	expect(t, mia.do(http.MethodGet, "/api/v1/me", nil), http.StatusUnauthorized, "unauthorized")
	expect(t, newClient(t, a).login("mia", otherPassword), http.StatusUnauthorized, "invalid_credentials")
	if a.count(t, `SELECT COUNT(*) FROM sessions WHERE user_id = ?`, id) != 0 {
		t.Error("sessions of a disabled user were kept")
	}

	expectStatus(t, admin.do(http.MethodPatch, userPath(id, ""), map[string]any{"disabled": false}), http.StatusOK)
	newClient(t, a).mustLogin("mia", otherPassword)
}

func TestTheLastAdminCannotBeRemovedDemotedOrDisabled(t *testing.T) {
	_, admin := adminAndApp(t)
	var me meBody
	decodeBody(t, admin.do(http.MethodGet, "/api/v1/me", nil), &me)
	self := userPath(me.ID, "")

	expect(t, admin.do(http.MethodPatch, self, map[string]any{"role": "member"}), http.StatusConflict, "conflict")
	expect(t, admin.do(http.MethodPatch, self, map[string]any{"disabled": true}), http.StatusConflict, "conflict")
	expect(t, admin.do(http.MethodDelete, self, nil), http.StatusConflict, "conflict")

	// A disabled second admin does not count as a spare.
	other := addUser(t, admin, "boss2", "admin")
	expectStatus(t, admin.do(http.MethodPatch, userPath(other, ""), map[string]any{"disabled": true}), http.StatusOK)
	expect(t, admin.do(http.MethodPatch, self, map[string]any{"role": "member"}), http.StatusConflict, "conflict")
	// With the spare gone for good the remaining admin is the last one again.
	expectStatus(t, admin.do(http.MethodDelete, userPath(other, ""), nil), http.StatusNoContent)
	expect(t, admin.do(http.MethodPatch, self, map[string]any{"role": "member"}), http.StatusConflict, "conflict")
}

func TestAdminsCanDemoteThemselvesWhenAnotherAdminExists(t *testing.T) {
	_, admin := adminAndApp(t)
	var me meBody
	decodeBody(t, admin.do(http.MethodGet, "/api/v1/me", nil), &me)
	addUser(t, admin, "boss2", "admin")

	var u meBody
	decodeBody(t, admin.do(http.MethodPatch, userPath(me.ID, ""), map[string]any{"role": "member"}), &u)
	if u.Role != "member" {
		t.Errorf("role = %q", u.Role)
	}
	// They are a member now, so the admin API is closed to them.
	expect(t, admin.do(http.MethodGet, "/api/v1/users", nil), http.StatusForbidden, "forbidden")
}

func TestNobodyCanDisableOrDeleteThemselves(t *testing.T) {
	_, admin := adminAndApp(t)
	var me meBody
	decodeBody(t, admin.do(http.MethodGet, "/api/v1/me", nil), &me)
	addUser(t, admin, "boss2", "admin")
	expect(t, admin.do(http.MethodPatch, userPath(me.ID, ""), map[string]any{"disabled": true}), http.StatusConflict, "conflict")
	expect(t, admin.do(http.MethodDelete, userPath(me.ID, ""), nil), http.StatusConflict, "conflict")
}

func TestDeleteUser(t *testing.T) {
	a, admin := adminAndApp(t)
	id := addUser(t, admin, "mia", "member")
	mia := signedInAs(t, a, "mia")
	mia.enableTOTP()

	if rec := admin.do(http.MethodDelete, userPath(id, ""), nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	expect(t, mia.do(http.MethodGet, "/api/v1/me", nil), http.StatusUnauthorized, "unauthorized")
	for _, table := range []string{"sessions", "user_totp", "user_recovery_codes", "user_preferences"} {
		if n := a.count(t, `SELECT COUNT(*) FROM `+table+` WHERE user_id = ?`, id); n != 0 {
			t.Errorf("%s rows left for the deleted user: %d", table, n)
		}
	}
	expect(t, admin.do(http.MethodDelete, userPath(id, ""), nil), http.StatusNotFound, "not_found")
	expect(t, newClient(t, a).login("mia", otherPassword), http.StatusUnauthorized, "invalid_credentials")
}

func TestResetMFAForAnotherUser(t *testing.T) {
	a, admin := adminAndApp(t)
	id := addUser(t, admin, "mia", "member")
	mia := signedInAs(t, a, "mia")
	mia.enableTOTP()

	// With two-step login on, a password alone no longer signs in.
	var login struct {
		MFARequired bool `json:"mfa_required"`
	}
	decodeBody(t, newClient(t, a).login("mia", otherPassword), &login)
	if !login.MFARequired {
		t.Fatal("two-step login should be required")
	}

	expectStatus(t, admin.do(http.MethodPost, userPath(id, "/reset-mfa"), nil), http.StatusNoContent)
	if a.count(t, `SELECT COUNT(*) FROM user_totp WHERE user_id = ?`, id) != 0 || a.count(t, `SELECT COUNT(*) FROM user_recovery_codes WHERE user_id = ?`, id) != 0 {
		t.Error("two-step login data survived the reset")
	}
	expect(t, mia.do(http.MethodGet, "/api/v1/me", nil), http.StatusUnauthorized, "unauthorized")
	fresh := newClient(t, a)
	decodeBody(t, fresh.login("mia", otherPassword), &login)
	if login.MFARequired {
		t.Error("two-step login still required after the reset")
	}
	expectStatus(t, fresh.do(http.MethodGet, "/api/v1/me", nil), http.StatusOK)

	expect(t, admin.do(http.MethodPost, "/api/v1/users/9999/reset-mfa", nil), http.StatusNotFound, "not_found")
	// Resetting an account that has none succeeds quietly.
	expectStatus(t, admin.do(http.MethodPost, userPath(id, "/reset-mfa"), nil), http.StatusNoContent)
}

func TestAdminResettingTheirOwnMFAKeepsTheirSession(t *testing.T) {
	a, admin := adminAndApp(t)
	var me meBody
	decodeBody(t, admin.do(http.MethodGet, "/api/v1/me", nil), &me)
	other := newClient(t, a)
	other.mustLogin(adminName, adminPassword)
	admin.enableTOTP()

	expectStatus(t, admin.do(http.MethodPost, userPath(me.ID, "/reset-mfa"), nil), http.StatusNoContent)
	expectStatus(t, admin.do(http.MethodGet, "/api/v1/me", nil), http.StatusOK)
	expect(t, other.do(http.MethodGet, "/api/v1/me", nil), http.StatusUnauthorized, "unauthorized")
}

func TestUsersAPIListsTwoStepStatus(t *testing.T) {
	a, admin := adminAndApp(t)
	addUser(t, admin, "mia", "member")
	signedInAs(t, a, "mia").enableTOTP()
	var list []meBody
	decodeBody(t, admin.do(http.MethodGet, "/api/v1/users", nil), &list)
	if len(list) != 2 || list[0].TOTPEnabled || !list[1].TOTPEnabled {
		t.Errorf("list = %+v", list)
	}
}

type instanceBody struct {
	Instance struct {
		Name     string            `json:"name"`
		Timezone string            `json:"timezone"`
		Units    map[string]string `json:"units"`
		AuthMode string            `json:"auth_mode"`
	} `json:"instance"`
}

func TestMeCarriesTheInstanceDisplayInfoForEveryUser(t *testing.T) {
	a, admin := adminAndApp(t)
	expectStatus(t, admin.do(http.MethodPatch, "/api/v1/settings", map[string]any{
		"instance_name": "Reef HQ", "timezone": "Europe/Berlin", "units": map[string]string{"temperature": "f", "volume": "us_gal"},
	}), http.StatusOK)
	addUser(t, admin, "mia", "member")
	member := signedInAs(t, a, "mia")

	defaultSalinity, err := a.store.GetUnit(t.Context(), units.Salinity)
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]*apiClient{"admin": admin, "member": member} {
		var b instanceBody
		rec := c.do(http.MethodGet, "/api/v1/me", nil)
		decodeBody(t, rec, &b)
		in := b.Instance
		if rec.Code != 200 || in.Name != "Reef HQ" || in.Timezone != "Europe/Berlin" || in.AuthMode != "password" {
			t.Errorf("%s: instance = %+v", name, in)
		}
		if in.Units["temperature"] != "f" || in.Units["volume"] != "us_gal" || in.Units["salinity"] != string(defaultSalinity) || len(in.Units) != 5 {
			t.Errorf("%s: units = %v", name, in.Units)
		}
	}
	if string(defaultSalinity) != "sg" {
		t.Errorf("salinity default = %q, want sg", defaultSalinity)
	}
	// The members' view stays read-only: the settings endpoints are still closed to them.
	expect(t, member.do(http.MethodGet, "/api/v1/settings", nil), http.StatusForbidden, "forbidden")
}

func TestMeInstanceShowsTheLoginModeInForceAndFollowsChanges(t *testing.T) {
	a := newAPITestApp(t, config.Config{AuthMode: config.AuthModeDisabled})
	c := newClient(t, a)
	c.setup(nil)
	if err := a.store.SetAuthMode(t.Context(), "password"); err != nil {
		t.Fatal(err)
	}
	var b instanceBody
	decodeBody(t, newClient(t, a).do(http.MethodGet, "/api/v1/me", nil), &b)
	if b.Instance.AuthMode != "disabled" {
		t.Errorf("auth_mode = %q, want the override in force", b.Instance.AuthMode)
	}
	expectStatus(t, c.do(http.MethodPatch, "/api/v1/settings", map[string]any{"instance_name": "Renamed"}), http.StatusOK)
	decodeBody(t, c.do(http.MethodPatch, "/api/v1/me", map[string]any{"theme": "dark"}), &b)
	if b.Instance.Name != "Renamed" {
		t.Errorf("PATCH /me answered with a stale instance: %+v", b.Instance)
	}
}

// loginOffWithTwoAdmins returns an instance with login off, the acting admin, a second admin and a browser that acts as the first.
func loginOffWithTwoAdmins(t *testing.T) (a *testApp, c *apiClient, acting, spare int64) {
	t.Helper()
	a = newAPITestApp(t, config.Config{})
	c = newClient(t, a)
	var me meBody
	decodeBody(t, c.do(http.MethodPost, "/api/v1/setup", map[string]any{"auth_mode": "disabled"}), &me)
	spare = addUser(t, c, "boss2", "admin")
	return a, c, me.ID, spare
}

func TestDemotingTheActingAdminWhileLoginIsOffHandsOverToAnotherAdmin(t *testing.T) {
	a, c, acting, spare := loginOffWithTwoAdmins(t)
	expectStatus(t, c.do(http.MethodPatch, userPath(acting, ""), map[string]any{"role": "member"}), http.StatusOK)

	if id, _ := a.store.GetAuthDisabledUserID(t.Context()); id != spare {
		t.Errorf("stored acting user = %d, want %d", id, spare)
	}
	var me meBody
	decodeBody(t, newClient(t, a).do(http.MethodGet, "/api/v1/me", nil), &me)
	if me.ID != spare || me.Role != "admin" {
		t.Errorf("visitors act as %+v, want the other admin", me)
	}
	// Settings are still reachable, which is the point.
	expectStatus(t, newClient(t, a).do(http.MethodGet, "/api/v1/settings", nil), http.StatusOK)
}

func TestDisablingOrDeletingTheStoredActingAdminHandsOverToo(t *testing.T) {
	// With login on again, a leftover acting-user setting can still point at an admin another admin is about to remove.
	changes := map[string]func(c *apiClient, id int64) *httptest.ResponseRecorder{
		"disable": func(c *apiClient, id int64) *httptest.ResponseRecorder {
			return c.do(http.MethodPatch, userPath(id, ""), map[string]any{"disabled": true})
		},
		"demote": func(c *apiClient, id int64) *httptest.ResponseRecorder {
			return c.do(http.MethodPatch, userPath(id, ""), map[string]any{"role": "member"})
		},
		"delete": func(c *apiClient, id int64) *httptest.ResponseRecorder {
			return c.do(http.MethodDelete, userPath(id, ""), nil)
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			a, admin := adminAndApp(t)
			var me meBody
			decodeBody(t, admin.do(http.MethodGet, "/api/v1/me", nil), &me)
			victim := addUser(t, admin, "boss2", "admin")
			if err := a.store.SetAuthDisabledUserID(t.Context(), victim); err != nil {
				t.Fatal(err)
			}
			if rec := change(admin, victim); rec.Code >= 300 {
				t.Fatalf("%s: %d %s", name, rec.Code, rec.Body)
			}
			if id, _ := a.store.GetAuthDisabledUserID(t.Context()); id != me.ID {
				t.Errorf("stored acting user = %d, want %d", id, me.ID)
			}
		})
	}
}

func TestChangingAnAdminWhoIsNotTheActingUserLeavesTheSettingAlone(t *testing.T) {
	a, c, _, spare := loginOffWithTwoAdmins(t)
	third := addUser(t, c, "boss3", "admin")
	// Visitors act as the third admin, who is neither the first nor the one being changed.
	if err := a.store.SetAuthDisabledUserID(t.Context(), third); err != nil {
		t.Fatal(err)
	}
	expectStatus(t, c.do(http.MethodPatch, userPath(spare, ""), map[string]any{"role": "member"}), http.StatusOK)
	if id, _ := a.store.GetAuthDisabledUserID(t.Context()); id != third {
		t.Errorf("stored acting user = %d, want %d", id, third)
	}
}
