package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/config"
)

var (
	crossSite  = map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}
	sameOrigin = map[string]string{"Sec-Fetch-Site": "same-origin"}
)

// passwordStillWorks reports whether the admin can still log in with adminPassword.
func passwordStillWorks(a *testApp) bool {
	c := &apiClient{a: a, headers: map[string]string{}}
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"`+adminName+`","password":"`+adminPassword+`"}`))
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr = "192.0.2.99:1"
	c.a.Handler().ServeHTTP(rec, r)
	return rec.Code == http.StatusOK
}

func TestCrossSiteRequestsCannotChangeStateWithAValidSession(t *testing.T) {
	a, c := adminAndApp(t)
	addUser(t, c, "mia", "member")

	changes := []struct {
		name, method, path string
		body               any
	}{
		{"change password", http.MethodPost, "/api/v1/me/password", map[string]string{"current_password": adminPassword, "new_password": "hijacked passphrase"}},
		{"update profile", http.MethodPatch, "/api/v1/me", map[string]string{"display_name": "Hijacked"}},
		{"create user", http.MethodPost, "/api/v1/users", map[string]string{"username": "evil", "password": "hijacked passphrase", "role": "admin"}},
		{"update user", http.MethodPatch, "/api/v1/users/2", map[string]any{"role": "admin"}},
		{"delete user", http.MethodDelete, "/api/v1/users/2", nil},
		{"change settings", http.MethodPatch, "/api/v1/settings", map[string]string{"instance_name": "Hijacked"}},
		{"turn login off", http.MethodPatch, "/api/v1/settings", map[string]string{"auth_mode": "disabled", "password": adminPassword}},
		{"enrol totp", http.MethodPost, "/api/v1/me/totp/enroll", nil},
		{"disable totp", http.MethodDelete, "/api/v1/me/totp", nil},
		{"revoke session", http.MethodDelete, "/api/v1/me/sessions/1", nil},
		{"log out", http.MethodPost, "/api/v1/auth/logout", nil},
	}
	for _, tc := range changes {
		t.Run(tc.name, func(t *testing.T) {
			var rec *httptest.ResponseRecorder
			if tc.body == nil {
				rec = c.raw(tc.method, tc.path, "", "", crossSite)
			} else {
				rec = c.doWith(tc.method, tc.path, tc.body, crossSite)
			}
			expect(t, rec, http.StatusForbidden, "forbidden")
		})
	}

	// Nothing happened: same session, same password, same data.
	expectStatus(t, c.do(http.MethodGet, "/api/v1/me", nil), http.StatusOK)
	if !passwordStillWorks(a) {
		t.Error("a cross-site request changed the password")
	}
	if a.count(t, `SELECT COUNT(*) FROM users`) != 2 || a.count(t, `SELECT COUNT(*) FROM users WHERE role = 'admin'`) != 1 {
		t.Error("a cross-site request changed the users")
	}
	if name, _ := a.store.GetInstanceName(t.Context()); name == "Hijacked" {
		t.Error("a cross-site request changed the settings")
	}
	if mode, _ := a.store.GetAuthMode(t.Context()); mode != "password" {
		t.Error("a cross-site request turned login off")
	}
}

func TestCrossSiteLoginAndSetupAreRefusedToo(t *testing.T) {
	a := newAPITestApp(t, config.Config{})
	c := newClient(t, a)
	// Setup from another site must not let a malicious page create the first admin for the victim's server.
	expect(t, c.doWith(http.MethodPost, "/api/v1/setup", map[string]any{"username": "evil", "password": otherPassword}, crossSite), http.StatusForbidden, "forbidden")
	if a.count(t, `SELECT COUNT(*) FROM users`) != 0 {
		t.Fatal("cross-site setup created a user")
	}
	c.setup(nil)

	// Login CSRF: a page that logs the victim into the attacker's account.
	rec := c.doWith(http.MethodPost, "/api/v1/auth/login", map[string]string{"username": adminName, "password": adminPassword}, crossSite)
	expect(t, rec, http.StatusForbidden, "forbidden")
	if len(rec.Result().Cookies()) != 0 {
		t.Error("a refused cross-site login set a cookie")
	}
	expect(t, c.doWith(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"code": "123456"}, crossSite), http.StatusForbidden, "forbidden")

	// A browser says same-site for sibling subdomains: that is still not same-origin.
	sibling := map[string]string{"Sec-Fetch-Site": "same-site", "Origin": "https://other.example.org"}
	expect(t, c.doWith(http.MethodPost, "/api/v1/auth/login", map[string]string{"username": adminName, "password": adminPassword}, sibling), http.StatusForbidden, "forbidden")
}

func TestOriginHeaderAloneIsEnoughToRefuseAForeignSite(t *testing.T) {
	a, c := adminAndApp(t)
	old := map[string]string{"Origin": "https://evil.example"} // an older browser without Sec-Fetch-Site
	expect(t, c.doWith(http.MethodPost, "/api/v1/me/password", map[string]string{"current_password": adminPassword, "new_password": "hijacked passphrase"}, old), http.StatusForbidden, "forbidden")
	if !passwordStillWorks(a) {
		t.Error("the password changed")
	}
}

func TestSameOriginAndNonBrowserRequestsAreAccepted(t *testing.T) {
	a, c := adminAndApp(t)
	// Same origin, as the web UI sends it.
	rec := c.doWith(http.MethodPatch, "/api/v1/me", map[string]string{"display_name": "Same"}, sameOrigin)
	expectStatus(t, rec, http.StatusOK)
	// No browser headers at all: curl, the CLI, scripts.
	expectStatus(t, c.do(http.MethodPatch, "/api/v1/me", map[string]string{"display_name": "Script"}), http.StatusOK)
	// The Origin header naming the app's own host.
	own := map[string]string{"Origin": "http://example.com"}
	rec = c.raw(http.MethodPatch, "/api/v1/me", "application/json", `{"display_name":"Own"}`, own)
	expectStatus(t, rec, http.StatusOK)
	// Reads are never blocked by the origin check; they change nothing.
	rec = c.raw(http.MethodGet, "/api/v1/me", "", "", crossSite)
	expectStatus(t, rec, http.StatusOK)
	_ = a
}

func TestTrustedOriginMayMakeCrossOriginRequests(t *testing.T) {
	a := newAPITestApp(t, config.Config{TrustedOrigins: []string{"https://reef.home.lan"}})
	c := newClient(t, a)
	c.setup(nil)
	trusted := map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://reef.home.lan"}
	expectStatus(t, c.doWith(http.MethodPatch, "/api/v1/me", map[string]string{"display_name": "Proxy"}, trusted), http.StatusOK)
	expect(t, c.doWith(http.MethodPatch, "/api/v1/me", map[string]string{"display_name": "Evil"}, crossSite), http.StatusForbidden, "forbidden")
}

func TestStateChangingRequestsMustBeJSON(t *testing.T) {
	a, c := adminAndApp(t)
	form := "current_password=" + adminPassword + "&new_password=hijacked+passphrase"
	for _, ct := range []string{"application/x-www-form-urlencoded", "text/plain", "text/plain;charset=UTF-8", "multipart/form-data; boundary=x", ""} {
		rec := c.raw(http.MethodPost, "/api/v1/me/password", ct, form, sameOrigin)
		expect(t, rec, http.StatusUnsupportedMediaType, "unsupported_media_type")
	}
	// A JSON body smuggled in as text/plain, which a cross-site form can send without a preflight.
	rec := c.raw(http.MethodPost, "/api/v1/me/password", "text/plain", `{"current_password":"`+adminPassword+`","new_password":"hijacked passphrase"}`, nil)
	expect(t, rec, http.StatusUnsupportedMediaType, "unsupported_media_type")
	rec = c.raw(http.MethodPost, "/api/v1/auth/login", "text/plain", `{"username":"`+adminName+`","password":"`+adminPassword+`"}`, nil)
	expect(t, rec, http.StatusUnsupportedMediaType, "unsupported_media_type")
	if !passwordStillWorks(a) {
		t.Error("the password changed")
	}
}

func TestSessionCookieAttributesThroughTheFullStack(t *testing.T) {
	check := func(t *testing.T, cfg config.Config, wantSecure bool, headers map[string]string) {
		t.Helper()
		a := newAPITestApp(t, cfg)
		c := newClient(t, a)
		for k, v := range headers {
			c.headers[k] = v
		}
		rec := c.setup(nil)
		raw := rec.Header().Get("Set-Cookie")
		for _, want := range []string{"rs_session=", "HttpOnly", "SameSite=Lax", "Path=/", "Max-Age=2592000"} {
			if !strings.Contains(raw, want) {
				t.Errorf("Set-Cookie %q lacks %q", raw, want)
			}
		}
		if strings.Contains(raw, "Secure") != wantSecure {
			t.Errorf("Secure = %v in %q", !wantSecure, raw)
		}
		// The token reaches the browser only as a cookie, never in a body.
		if strings.Contains(rec.Body.String(), c.cookie) {
			t.Error("the session token appears in the response body")
		}
		list := c.do(http.MethodGet, "/api/v1/me/sessions", nil)
		if strings.Contains(list.Body.String(), c.cookie) || containsFold(list.Body.String(), "token") {
			t.Errorf("session list exposes the token: %s", list.Body)
		}
		login := newClient(t, a)
		lr := login.login(adminName, adminPassword)
		if strings.Contains(lr.Body.String(), login.cookie) {
			t.Error("the login response body holds the token")
		}
		if !strings.Contains(lr.Header().Get("Set-Cookie"), "HttpOnly") {
			t.Error("login cookie is readable by scripts")
		}
	}
	t.Run("plain http", func(t *testing.T) { check(t, config.Config{}, false, nil) })
	t.Run("https public url", func(t *testing.T) { check(t, config.Config{PublicURL: "https://reef.home.lan"}, true, nil) })
}

func TestResponsesAreNeverCached(t *testing.T) {
	_, c := adminAndApp(t)
	for _, p := range []string{"/api/v1/me", "/api/v1/settings", "/api/v1/me/sessions", "/api/v1/auth/config", "/api/v1/units"} {
		if got := c.do(http.MethodGet, p, nil).Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: Cache-Control = %q", p, got)
		}
	}
	rec := newClient(t, c.a).do(http.MethodGet, "/api/v1/me", nil)
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("error responses must not be cached either")
	}
}

func TestLogoutThroughTheStackEndsTheSession(t *testing.T) {
	a, c := adminAndApp(t)
	token := c.cookie
	rec := c.do(http.MethodPost, "/api/v1/auth/logout", nil)
	expectStatus(t, rec, http.StatusNoContent)
	if !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=0") && !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=-1") {
		// net/http writes a negative MaxAge as "Max-Age=0".
		t.Errorf("Set-Cookie = %q", rec.Header().Get("Set-Cookie"))
	}
	replay := newClient(t, a)
	replay.cookie = token
	expect(t, replay.do(http.MethodGet, "/api/v1/me", nil), http.StatusUnauthorized, "unauthorized")
}

func TestLoginThroughTheStackFailsGenerically(t *testing.T) {
	a, _ := adminAndApp(t)
	c := newClient(t, a)
	wrong := c.login(adminName, "wrong password here")
	unknown := newClient(t, a).login("ghost", "wrong password here")
	expect(t, wrong, http.StatusUnauthorized, "invalid_credentials")
	expect(t, unknown, http.StatusUnauthorized, "invalid_credentials")
	if wrong.Body.String() != unknown.Body.String() {
		t.Errorf("responses differ: %s vs %s", wrong.Body, unknown.Body)
	}
}

func TestCrossSiteRequestsAreRefusedWhenLoginIsOff(t *testing.T) {
	// With no login every visitor is the admin, so the origin check is the only thing between a malicious page and the instance.
	a := newAPITestApp(t, config.Config{})
	c := newClient(t, a)
	expectStatus(t, c.do(http.MethodPost, "/api/v1/setup", map[string]any{"auth_mode": "disabled"}), http.StatusCreated)

	expect(t, c.doWith(http.MethodPost, "/api/v1/users", map[string]string{"username": "evil", "password": "hijacked passphrase", "role": "admin"}, crossSite), http.StatusForbidden, "forbidden")
	expect(t, c.doWith(http.MethodPatch, "/api/v1/settings", map[string]string{"instance_name": "Hijacked"}, crossSite), http.StatusForbidden, "forbidden")
	expect(t, c.doWith(http.MethodPost, "/api/v1/me/password", map[string]string{"new_password": "hijacked passphrase"}, crossSite), http.StatusForbidden, "forbidden")
	if a.count(t, `SELECT COUNT(*) FROM users`) != 1 || a.count(t, `SELECT COUNT(*) FROM users WHERE password_hash IS NOT NULL`) != 0 {
		t.Error("a cross-site request changed accounts while login was off")
	}
	// The same calls without cross-site headers go through.
	expectStatus(t, c.do(http.MethodPost, "/api/v1/users", map[string]string{"username": "mia", "password": otherPassword}), http.StatusCreated)
}

func TestDisablingTOTPThroughTheStackNeedsTheCurrentPassword(t *testing.T) {
	a, c := adminAndApp(t)
	c.enableTOTP()

	expect(t, c.do(http.MethodDelete, "/api/v1/me/totp", nil), http.StatusUnprocessableEntity, "validation")
	b := expect(t, c.do(http.MethodDelete, "/api/v1/me/totp", map[string]string{"password": "not the password!!"}), http.StatusUnprocessableEntity, "validation")
	if b.Error.Fields["password"] == "" {
		t.Errorf("fields = %v", b.Error.Fields)
	}
	if a.count(t, `SELECT COUNT(*) FROM user_totp WHERE confirmed_at IS NOT NULL`) != 1 {
		t.Fatal("two-step login was removed without the password")
	}
	// A cross-site page cannot do it even if it somehow had the password.
	expect(t, c.doWith(http.MethodDelete, "/api/v1/me/totp", map[string]string{"password": adminPassword}, crossSite), http.StatusForbidden, "forbidden")
	expectStatus(t, c.do(http.MethodDelete, "/api/v1/me/totp", map[string]string{"password": adminPassword}), http.StatusNoContent)
	if a.count(t, `SELECT COUNT(*) FROM user_totp`) != 0 {
		t.Error("secret survived")
	}
}

func TestEndpointsWithoutABodyStillRefuseFormAndTextPosts(t *testing.T) {
	a, admin := adminAndApp(t)
	id := addUser(t, admin, "mia", "member")
	mia := signedInAs(t, a, "mia")

	endpoints := []struct {
		name, method, path string
		client             func() *apiClient
	}{
		{"logout", http.MethodPost, "/api/v1/auth/logout", func() *apiClient { return mia }},
		{"enrol totp", http.MethodPost, "/api/v1/me/totp/enroll", func() *apiClient { return mia }},
		{"revoke session", http.MethodDelete, "/api/v1/me/sessions/1", func() *apiClient { return mia }},
		{"reset mfa", http.MethodPost, userPath(id, "/reset-mfa"), func() *apiClient { return admin }},
		{"delete user", http.MethodDelete, userPath(id, ""), func() *apiClient { return admin }},
	}
	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			c := ep.client()
			for _, ct := range []string{"application/x-www-form-urlencoded", "text/plain", "text/plain;charset=UTF-8", "multipart/form-data; boundary=x"} {
				expect(t, c.raw(ep.method, ep.path, ct, "", sameOrigin), http.StatusUnsupportedMediaType, "unsupported_media_type")
				expect(t, c.raw(ep.method, ep.path, ct, "a=b", sameOrigin), http.StatusUnsupportedMediaType, "unsupported_media_type")
			}
			expect(t, c.raw(ep.method, ep.path, "", "a=b", sameOrigin), http.StatusUnsupportedMediaType, "unsupported_media_type")
		})
	}
	// Nothing happened to the account or its session.
	expectStatus(t, mia.do(http.MethodGet, "/api/v1/me", nil), http.StatusOK)
	if a.count(t, `SELECT COUNT(*) FROM user_totp`) != 0 || a.count(t, `SELECT COUNT(*) FROM users WHERE id = ?`, id) != 1 {
		t.Error("a refused request still changed state")
	}

	// A normal client sends no body and no content type, or a JSON one.
	expectStatus(t, mia.raw(http.MethodPost, "/api/v1/me/totp/enroll", "", "", nil), http.StatusOK)
	expectStatus(t, mia.raw(http.MethodDelete, "/api/v1/me/totp", "application/json", `{}`, nil), http.StatusNoContent)
	expectStatus(t, mia.raw(http.MethodPost, "/api/v1/me/totp/enroll", "application/json", `{}`, nil), http.StatusOK)
	expectStatus(t, mia.raw(http.MethodPost, "/api/v1/auth/logout", "", "", nil), http.StatusNoContent)
}
