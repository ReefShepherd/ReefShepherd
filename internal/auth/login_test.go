package auth_test

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/auth/password"
	"github.com/reefshepherd/reefshepherd/internal/config"
)

func TestProtectedRouteBeforeSetupSaysSetupRequired(t *testing.T) {
	e := newEnv(t, config.Config{})
	c := e.client()
	c.whoami().expect(http.StatusConflict, "setup_required")

	e.addUser("alice", goodPassword, "admin")
	c.whoami().expect(http.StatusUnauthorized, "unauthorized")
}

func TestSetupRequiredEndsOnceTheFlagIsSet(t *testing.T) {
	e := newEnv(t, config.Config{})
	if err := e.store.SetSetupCompleted(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	e.client().whoami().expect(http.StatusUnauthorized, "unauthorized")
}

func TestAuthConfig(t *testing.T) {
	e := newEnv(t, config.Config{})
	type cfg struct {
		Mode          string   `json:"mode"`
		SetupRequired bool     `json:"setup_required"`
		Methods       []string `json:"methods"`
		InstanceName  string   `json:"instance_name"`
	}
	var got cfg
	r := e.client().do(http.MethodGet, "/api/v1/auth/config", nil)
	r.decode(&got)
	if r.Code != 200 || got.Mode != "password" || !got.SetupRequired || len(got.Methods) != 1 || got.Methods[0] != "password" || got.InstanceName != "ReefShepherd" {
		t.Errorf("before setup: %d %+v", r.Code, got)
	}

	e.addUser("alice", goodPassword, "admin")
	if err := e.store.SetInstanceName(t.Context(), "Reef HQ"); err != nil {
		t.Fatal(err)
	}
	e.client().do(http.MethodGet, "/api/v1/auth/config", nil).decode(&got)
	if got.SetupRequired || got.InstanceName != "Reef HQ" {
		t.Errorf("after setup: %+v", got)
	}

	// With login off no method is offered.
	e2 := newEnv(t, config.Config{AuthMode: config.AuthModeDisabled})
	e2.addUser("alice", goodPassword, "admin")
	var off cfg
	e2.client().do(http.MethodGet, "/api/v1/auth/config", nil).decode(&off)
	if off.Mode != "disabled" || len(off.Methods) != 0 || off.Methods == nil {
		t.Errorf("disabled mode: %+v", off)
	}
}

func TestLoginSetsAHardenedCookieAndStoresOnlyItsHash(t *testing.T) {
	e := newEnv(t, config.Config{})
	id := e.addUser("alice", goodPassword, "admin")
	c := e.client()

	r := c.login("alice", goodPassword)
	if r.Code != http.StatusOK {
		t.Fatalf("login: %d %s", r.Code, r.Body)
	}
	cookie := r.sessionCookie()
	if cookie == nil {
		t.Fatal("no session cookie")
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.Secure {
		t.Errorf("cookie = %+v", cookie)
	}
	if cookie.MaxAge != int((30 * 24 * time.Hour).Seconds()) {
		t.Errorf("max age = %d", cookie.MaxAge)
	}
	if len(cookie.Value) < 43 {
		t.Errorf("token %q is too short for 32 random bytes", cookie.Value)
	}
	if tokenHashCount(e, cookie.Value) != 1 {
		t.Error("session row for the token hash not found")
	}
	// The raw token itself is nowhere in the table.
	if n := e.count(`SELECT COUNT(*) FROM sessions WHERE CAST(token_hash AS TEXT) = ?`, cookie.Value); n != 0 {
		t.Error("the plain token was stored")
	}
	var stage, method, ua, ip string
	if err := e.db.R.QueryRowContext(t.Context(), `SELECT stage, auth_method, user_agent, ip_address FROM sessions WHERE user_id = ?`, id).Scan(&stage, &method, &ua, &ip); err != nil {
		t.Fatal(err)
	}
	if stage != "active" || method != "password" || ua != "test-browser" || ip != "192.0.2.10" {
		t.Errorf("row = %s %s %s %s", stage, method, ua, ip)
	}

	var who struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		Role     string `json:"role"`
		Session  int64  `json:"session_id"`
	}
	w := c.whoami()
	w.decode(&who)
	if w.Code != 200 || who.ID != id || who.Username != "alice" || who.Role != "admin" || who.Session == 0 {
		t.Errorf("whoami: %d %+v", w.Code, who)
	}
}

func TestEachLoginGetsADifferentToken(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	a, b := e.client(), e.client()
	a.mustLogin("alice", goodPassword)
	b.mustLogin("alice", goodPassword)
	if a.cookie == b.cookie || a.cookie == "" {
		t.Errorf("tokens %q and %q", a.cookie, b.cookie)
	}
}

func TestCookieSecureFlag(t *testing.T) {
	secure := func(cfg config.Config, headers map[string]string, ip string) bool {
		t.Helper()
		e := newEnv(t, cfg)
		e.addUser("alice", goodPassword, "member")
		c := e.client()
		c.ip = ip
		for k, v := range headers {
			c.header.Set(k, v)
		}
		r := c.login("alice", goodPassword)
		if r.sessionCookie() == nil {
			t.Fatalf("login failed: %d %s", r.Code, r.Body)
		}
		return r.sessionCookie().Secure
	}
	if !secure(config.Config{PublicURL: "https://reef.home.lan"}, nil, "192.0.2.10") {
		t.Error("an https public URL must give a Secure cookie")
	}
	if secure(config.Config{PublicURL: "http://reef.home.lan"}, nil, "192.0.2.10") {
		t.Error("an http public URL must not give a Secure cookie")
	}
	trusted := config.Config{TrustedProxies: []netip.Prefix{prefix("10.0.0.0/8")}}
	if !secure(trusted, map[string]string{"X-Forwarded-Proto": "https"}, "10.1.2.3") {
		t.Error("a trusted proxy saying https must give a Secure cookie")
	}
	if secure(trusted, map[string]string{"X-Forwarded-Proto": "https"}, "192.0.2.77") {
		t.Error("X-Forwarded-Proto from an untrusted peer must be ignored")
	}
	if secure(config.Config{}, map[string]string{"X-Forwarded-Proto": "https"}, "10.1.2.3") {
		t.Error("with no trusted proxies the header must be ignored")
	}
}

func TestLoginFailuresAreIndistinguishable(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	e.addUser("nopass", "", "member")
	off := e.addUser("off", goodPassword, "member")
	e.exec(`UPDATE users SET disabled = 1 WHERE id = ?`, off)

	attempts := map[string][2]string{
		"wrong password":   {"alice", "wrong password entirely"},
		"unknown user":     {"ghost", goodPassword},
		"no password set":  {"nopass", goodPassword},
		"disabled account": {"off", goodPassword},
	}
	var first string
	i := 0
	for name, a := range attempts {
		// A fresh address each time keeps the rate limit out of the way.
		i++
		c := e.client()
		c.ip = fmt.Sprintf("198.51.100.%d", i)
		r := c.login(a[0], a[1])
		r.expect(http.StatusUnauthorized, "invalid_credentials")
		if r.sessionCookie() != nil {
			t.Errorf("%s: a cookie was issued", name)
		}
		if first == "" {
			first = r.Body.String()
		} else if r.Body.String() != first {
			t.Errorf("%s: body differs: %s vs %s", name, r.Body, first)
		}
	}
	if e.count(`SELECT COUNT(*) FROM sessions`) != 0 {
		t.Error("a failed login created a session")
	}
}

func TestLoginValidatesTheRequest(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	c := e.client()
	c.do(http.MethodPost, "/api/v1/auth/login", map[string]string{"username": "alice"}).expect(http.StatusUnprocessableEntity, "validation")
	c.do(http.MethodPost, "/api/v1/auth/login", map[string]string{"password": "x"}).expect(http.StatusUnprocessableEntity, "validation")
	c.do(http.MethodPost, "/api/v1/auth/login", map[string]string{"username": "alice", "password": "x", "extra": "y"}).expect(http.StatusUnprocessableEntity, "validation")
	c.do(http.MethodPost, "/api/v1/auth/login", nil).expect(http.StatusUnsupportedMediaType, "unsupported_media_type")
}

func TestUsernameIsCaseInsensitive(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("Alice", goodPassword, "member")
	c := e.client()
	c.mustLogin("  aLiCe ", goodPassword)
	if c.whoami().Code != 200 {
		t.Error("not signed in")
	}
}

func TestLogoutEndsTheSessionAndClearsTheCookie(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	c := e.client()
	c.mustLogin("alice", goodPassword)
	token := c.cookie

	r := c.do(http.MethodPost, "/api/v1/auth/logout", nil)
	if r.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", r.Code)
	}
	if cleared := r.sessionCookie(); cleared == nil || cleared.MaxAge >= 0 || cleared.Value != "" || !cleared.HttpOnly {
		t.Errorf("cleared cookie = %+v", cleared)
	}
	if tokenHashCount(e, token) != 0 {
		t.Error("the session row survived logout")
	}
	// A stolen copy of the old cookie is useless.
	replay := e.client()
	replay.cookie = token
	replay.whoami().expect(http.StatusUnauthorized, "unauthorized")

	// Logging out when already signed out is not an error.
	if r := e.client().do(http.MethodPost, "/api/v1/auth/logout", nil); r.Code != http.StatusNoContent {
		t.Errorf("anonymous logout: %d", r.Code)
	}
}

func TestLoginReplacesTheSessionThePresentedCookieHeld(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	c := e.client()
	c.mustLogin("alice", goodPassword)
	old := c.cookie
	c.mustLogin("alice", goodPassword)
	if c.cookie == old || tokenHashCount(e, old) != 0 {
		t.Error("the previous token survived a new login")
	}
	if e.count(`SELECT COUNT(*) FROM sessions`) != 1 {
		t.Errorf("sessions = %d, want 1", e.count(`SELECT COUNT(*) FROM sessions`))
	}
}

func TestStartSessionSignsTheBrowserIn(t *testing.T) {
	e := newEnv(t, config.Config{})
	id := e.addUser("alice", "", "admin")
	mux := http.NewServeMux()
	mux.HandleFunc("POST /start", func(w http.ResponseWriter, r *http.Request) {
		if err := e.svc.StartSession(w, r, id, "oidc"); err != nil {
			t.Error(err)
		}
	})
	mux.Handle("/", e.h)
	e.h = mux
	c := e.client()
	if r := c.do(http.MethodPost, "/start", nil); r.sessionCookie() == nil {
		t.Fatal("StartSession set no cookie")
	}
	if got := e.count(`SELECT COUNT(*) FROM sessions WHERE user_id = ? AND stage = 'active' AND auth_method = 'oidc'`, id); got != 1 {
		t.Errorf("sessions = %d", got)
	}
	if c.whoami().Code != 200 {
		t.Error("the new session does not authenticate")
	}
}

func TestSessionExpiresThirtyDaysAfterLastUse(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	c := e.client()
	c.mustLogin("alice", goodPassword)

	e.now = e.now.Add(20 * 24 * time.Hour)
	if c.whoami().Code != 200 {
		t.Fatal("session should be valid on day 20")
	}
	// The use on day 20 pushed the expiry out, so day 45 is still inside 30 days of last use.
	e.now = e.now.Add(25 * 24 * time.Hour)
	if c.whoami().Code != 200 {
		t.Fatal("a used session should have slid its expiry forward")
	}
	e.now = e.now.Add(30*24*time.Hour + time.Minute)
	c.whoami().expect(http.StatusUnauthorized, "unauthorized")
}

func TestUnusedSessionExpires(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	c := e.client()
	c.mustLogin("alice", goodPassword)
	e.now = e.now.Add(30*24*time.Hour + time.Second)
	c.whoami().expect(http.StatusUnauthorized, "unauthorized")
}

func TestLastSeenIsWrittenAtMostOncePerHour(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	c := e.client()
	c.mustLogin("alice", goodPassword)
	loginSeen := e.now.Format("2006-01-02T15:04:05Z")

	lastSeen := func() string {
		var s string
		if err := e.db.R.QueryRowContext(t.Context(), `SELECT last_seen_at FROM sessions`).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	e.now = e.now.Add(59 * time.Minute)
	r := c.whoami()
	if lastSeen() != loginSeen || r.sessionCookie() != nil {
		t.Errorf("a request within the hour wrote last_seen_at=%s or re-issued the cookie", lastSeen())
	}
	e.now = e.now.Add(2 * time.Minute)
	r = c.whoami()
	if want := e.now.Format("2006-01-02T15:04:05Z"); lastSeen() != want {
		t.Errorf("last_seen_at = %s, want %s", lastSeen(), want)
	}
	if got := r.sessionCookie(); got == nil || got.Value != c.cookie || got.MaxAge != int((30*24*time.Hour).Seconds()) || !got.HttpOnly {
		t.Errorf("re-issued cookie = %+v", got)
	}
	var expires string
	if err := e.db.R.QueryRowContext(t.Context(), `SELECT expires_at FROM sessions`).Scan(&expires); err != nil {
		t.Fatal(err)
	}
	if want := e.now.Add(30 * 24 * time.Hour).Format("2006-01-02T15:04:05Z"); expires != want {
		t.Errorf("expires_at = %s, want %s", expires, want)
	}
}

func TestSessionsDieWithTheirUserOrWhenDisabled(t *testing.T) {
	e := newEnv(t, config.Config{})
	id := e.addUser("alice", goodPassword, "member")
	c := e.client()
	c.mustLogin("alice", goodPassword)
	e.exec(`UPDATE users SET disabled = 1 WHERE id = ?`, id)
	c.whoami().expect(http.StatusUnauthorized, "unauthorized")

	e.exec(`UPDATE users SET disabled = 0 WHERE id = ?`, id)
	if c.whoami().Code != 200 {
		t.Error("re-enabled user should have their session back")
	}
	if err := e.store.SetSetupCompleted(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	e.exec(`DELETE FROM users WHERE id = ?`, id)
	c.whoami().expect(http.StatusUnauthorized, "unauthorized")
	if e.count(`SELECT COUNT(*) FROM sessions`) != 0 {
		t.Error("sessions outlived their user")
	}
}

func TestGarbageCookiesAreAnonymous(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	for _, v := range []string{"x", strings.Repeat("A", 43), "%%%", strings.Repeat("z", 5000)} {
		c := e.client()
		c.cookie = v
		c.whoami().expect(http.StatusUnauthorized, "unauthorized")
	}
}

func TestExpiredSessionsAreCleanedUp(t *testing.T) {
	e := newEnv(t, config.Config{})
	id := e.addUser("alice", goodPassword, "member")
	old := e.now.Add(-time.Hour).Format("2006-01-02T15:04:05Z")
	future := e.now.Add(time.Hour).Format("2006-01-02T15:04:05Z")
	for i, exp := range []string{old, old, future} {
		sum := sha256.Sum256([]byte{byte(i)})
		e.exec(`INSERT INTO sessions (token_hash, user_id, stage, auth_method, expires_at) VALUES (?, ?, 'active', 'password', ?)`, sum[:], id, exp)
	}
	if err := e.svc.CleanupExpired(t.Context()); err != nil {
		t.Fatal(err)
	}
	if n := e.count(`SELECT COUNT(*) FROM sessions`); n != 1 {
		t.Errorf("sessions after cleanup = %d, want only the unexpired one", n)
	}

	// Creating a session also sweeps what has expired meanwhile.
	e.now = e.now.Add(2 * time.Hour)
	e.client().mustLogin("alice", goodPassword)
	if n := e.count(`SELECT COUNT(*) FROM sessions`); n != 1 {
		t.Errorf("sessions after a login = %d, want just the new one", n)
	}
}

func TestRequireAdmin(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("root", goodPassword, "admin")
	e.addUser("mia", goodPassword, "member")

	anon := e.client()
	anon.do(http.MethodGet, "/api/v1/admin-only", nil).expect(http.StatusUnauthorized, "unauthorized")

	member := e.client()
	member.mustLogin("mia", goodPassword)
	member.do(http.MethodGet, "/api/v1/admin-only", nil).expect(http.StatusForbidden, "forbidden")

	admin := e.client()
	admin.mustLogin("root", goodPassword)
	if r := admin.do(http.MethodGet, "/api/v1/admin-only", nil); r.Code != 200 {
		t.Errorf("admin: %d", r.Code)
	}
}

func TestSessionIDIsAbsentWithoutASession(t *testing.T) {
	if _, ok := auth.SessionID(t.Context()); ok {
		t.Error("SessionID found a session in an empty context")
	}
}

func TestOldPasswordHashIsUpgradedOnLogin(t *testing.T) {
	e := newEnv(t, config.Config{})
	id := e.addUser("alice", "", "member")
	weak := password.NewHasher(password.Params{Time: 1, MemoryKiB: 32, Threads: 1})
	hash, err := weak.Hash(t.Context(), goodPassword)
	if err != nil {
		t.Fatal(err)
	}
	e.exec(`UPDATE users SET password_hash = ? WHERE id = ?`, hash, id)

	stored := func() string {
		var h string
		if err := e.db.R.QueryRowContext(t.Context(), `SELECT password_hash FROM users WHERE id = ?`, id).Scan(&h); err != nil {
			t.Fatal(err)
		}
		return h
	}
	// A wrong password must not touch the stored hash.
	e.client().login("alice", "not the password").expect(http.StatusUnauthorized, "invalid_credentials")
	if stored() != hash {
		t.Fatal("hash changed after a failed login")
	}
	e.client().mustLogin("alice", goodPassword)
	if got := stored(); got == hash || !strings.Contains(got, "$m=64,t=1,p=1$") {
		t.Errorf("hash after login = %q, want one with the current parameters", got)
	}
	e.client().mustLogin("alice", goodPassword)
}

func TestUnreadableStoredHashIsAnOrdinaryLoginFailure(t *testing.T) {
	e := newEnv(t, config.Config{})
	id := e.addUser("alice", "", "member")
	e.exec(`UPDATE users SET password_hash = '$2a$10$notargon' WHERE id = ?`, id)
	e.client().login("alice", goodPassword).expect(http.StatusUnauthorized, "invalid_credentials")
}

func TestUnknownUserLoginCostsAsMuchAsAWrongPassword(t *testing.T) {
	// Parameters heavy enough that skipping the hash is easy to see in the timings.
	e := newEnvParams(t, config.Config{}, password.Params{Time: 3, MemoryKiB: 32 * 1024, Threads: 1})
	e.addUser("alice", goodPassword, "member")
	e.addUser("nopass", "", "member")

	attempt := func(i int, username string) time.Duration {
		c := e.client()
		c.ip = fmt.Sprintf("198.51.100.%d", i)
		start := time.Now()
		c.login(username, "wrong password here").expect(http.StatusUnauthorized, "invalid_credentials")
		return time.Since(start)
	}
	attempt(1, "ghost") // the first unknown-user login also prepares the stand-in hash
	wrong := attempt(2, "alice")
	for i, name := range []string{"ghost", "nopass"} {
		if took := attempt(10+i, name); took < wrong/3 {
			t.Errorf("login as %q took %v against %v for a wrong password: the response time gives away that there is nothing to check", name, took, wrong)
		}
	}
}
