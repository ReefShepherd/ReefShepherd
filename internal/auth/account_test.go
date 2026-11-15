package auth_test

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/config"
)

const newPassword = "an entirely new passphrase"

func changePassword(c *client, current, next string) *resp {
	return c.do(http.MethodPost, "/api/v1/me/password", map[string]string{"current_password": current, "new_password": next})
}

func TestPasswordChangeRevokesOtherSessions(t *testing.T) {
	e := newEnv(t, config.Config{})
	id := e.addUser("alice", goodPassword, "member")
	e.addUser("bob", goodPassword, "member")
	phone, laptop, bob := e.client(), e.client(), e.client()
	phone.mustLogin("alice", goodPassword)
	laptop.mustLogin("alice", goodPassword)
	bob.mustLogin("bob", goodPassword)

	r := changePassword(phone, goodPassword, newPassword)
	if r.Code != http.StatusNoContent {
		t.Fatalf("change: %d %s", r.Code, r.Body)
	}
	if phone.whoami().Code != 200 {
		t.Error("the session that changed the password was ended")
	}
	laptop.whoami().expect(http.StatusUnauthorized, "unauthorized")
	if bob.whoami().Code != 200 {
		t.Error("another user's session was ended")
	}
	if e.count(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, id) != 1 {
		t.Error("alice should have exactly the session that made the change")
	}

	e.client().login("alice", goodPassword).expect(http.StatusUnauthorized, "invalid_credentials")
	e.client().mustLogin("alice", newPassword)
	if e.count(`SELECT COUNT(*) FROM users WHERE id = ? AND password_changed_at IS NOT NULL`, id) != 1 {
		t.Error("password_changed_at not set")
	}
}

func TestPasswordChangeEndsPendingSecondStepSessionsToo(t *testing.T) {
	e, signedIn, _, _ := signedInWithTOTP(t)
	loginStep1(t, e)
	if e.count(`SELECT COUNT(*) FROM sessions WHERE stage = 'mfa_pending'`) != 1 {
		t.Fatal("no pending session to end")
	}
	if r := changePassword(signedIn, goodPassword, newPassword); r.Code != http.StatusNoContent {
		t.Fatal(r.Body)
	}
	if e.count(`SELECT COUNT(*) FROM sessions WHERE stage = 'mfa_pending'`) != 0 {
		t.Error("a pending session outlived a password change")
	}
}

func TestPasswordChangeChecksTheCurrentPasswordWithoutLoggingOut(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	c := e.client()
	c.mustLogin("alice", goodPassword)

	r := changePassword(c, "not my password!!", newPassword)
	// Not a 401: the web client treats 401 as "signed out" and would bounce to the login page.
	r.expect(http.StatusUnprocessableEntity, "validation")
	if got := r.Body.String(); !containsField(got, "current_password") {
		t.Errorf("body = %s", got)
	}
	changePassword(c, "", newPassword).expect(http.StatusUnprocessableEntity, "validation")
	if c.whoami().Code != 200 {
		t.Error("a failed change ended the session")
	}
	e.client().mustLogin("alice", goodPassword)
}

func containsField(body, field string) bool {
	return strings.Contains(body, `"fields":{"`+field+`"`)
}

func TestPasswordChangeEnforcesLengthRules(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	c := e.client()
	c.mustLogin("alice", goodPassword)

	short := changePassword(c, goodPassword, "sevench")
	short.expect(http.StatusUnprocessableEntity, "validation")
	if !containsField(short.Body.String(), "new_password") {
		t.Errorf("body = %s", short.Body)
	}
	long := make([]byte, 257)
	for i := range long {
		long[i] = 'x'
	}
	changePassword(c, goodPassword, string(long)).expect(http.StatusUnprocessableEntity, "validation")
	if r := changePassword(c, goodPassword, "exactly8"); r.Code != http.StatusNoContent {
		t.Errorf("8 characters: %d %s", r.Code, r.Body)
	}
	e.client().mustLogin("alice", "exactly8")
}

func TestWrongCurrentPasswordIsRateLimited(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	c := e.client()
	c.mustLogin("alice", goodPassword)
	for range 5 {
		changePassword(c, "not my password!!", newPassword).expect(http.StatusUnprocessableEntity, "validation")
	}
	r := changePassword(c, goodPassword, newPassword)
	r.expect(http.StatusTooManyRequests, "too_many_requests")
	if r.Header().Get("Retry-After") == "" {
		t.Error("no Retry-After")
	}
	// The password did not change: once the lock is over only the old one works.
	e.now = e.now.Add(31 * time.Second)
	e.client().login("alice", newPassword).expect(http.StatusUnauthorized, "invalid_credentials")
	e.now = e.now.Add(2 * time.Minute)
	e.client().mustLogin("alice", goodPassword)
}

func TestAccountWithoutPasswordCanSetOneWithoutCurrent(t *testing.T) {
	e := newEnv(t, config.Config{AuthMode: config.AuthModeDisabled})
	e.addUser("root", "", "admin")
	c := e.client()
	c.whoami()
	if r := changePassword(c, "", newPassword); r.Code != http.StatusNoContent {
		t.Fatalf("set first password: %d %s", r.Code, r.Body)
	}
	// Once set it is required.
	changePassword(c, "", "yet another passphrase").expect(http.StatusUnprocessableEntity, "validation")
}

func TestSessionListShowsOnlyActiveOwnSessions(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	e.addUser("bob", goodPassword, "member")
	a1, a2, b := e.client(), e.client(), e.client()
	a1.mustLogin("alice", goodPassword)
	a2.ip = "192.0.2.20"
	a2.mustLogin("alice", goodPassword)
	b.mustLogin("bob", goodPassword)

	type item struct {
		ID         int64  `json:"id"`
		AuthMethod string `json:"auth_method"`
		UserAgent  string `json:"user_agent"`
		IPAddress  string `json:"ip_address"`
		LastSeenAt string `json:"last_seen_at"`
		ExpiresAt  string `json:"expires_at"`
		Current    bool   `json:"current"`
	}
	var list []item
	r := a1.do(http.MethodGet, "/api/v1/me/sessions", nil)
	r.decode(&list)
	if r.Code != 200 || len(list) != 2 {
		t.Fatalf("list: %d %s", r.Code, r.Body)
	}
	currents := 0
	for _, s := range list {
		if s.Current {
			currents++
			if s.IPAddress != "192.0.2.10" {
				t.Errorf("current session ip = %s", s.IPAddress)
			}
		}
		if s.AuthMethod != "password" || s.UserAgent != "test-browser" || s.LastSeenAt == "" || s.ExpiresAt == "" {
			t.Errorf("session = %+v", s)
		}
	}
	if currents != 1 {
		t.Errorf("%d sessions marked current", currents)
	}

}

func TestExpiredSessionsAreNotListed(t *testing.T) {
	e := newEnv(t, config.Config{})
	id := e.addUser("alice", goodPassword, "member")
	c := e.client()
	c.mustLogin("alice", goodPassword)
	// A second row that has run out while its user is still around.
	e.exec(`INSERT INTO sessions (token_hash, user_id, stage, auth_method, expires_at) VALUES (x'01', ?, 'active', 'password', ?)`,
		id, e.now.Add(-time.Minute).Format("2006-01-02T15:04:05Z"))
	var list []struct {
		ID int64 `json:"id"`
	}
	c.do(http.MethodGet, "/api/v1/me/sessions", nil).decode(&list)
	if len(list) != 1 {
		t.Errorf("listed %d sessions, want 1", len(list))
	}
}

func TestPendingSessionsAreNotListed(t *testing.T) {
	e, c, _, _ := signedInWithTOTP(t)
	loginStep1(t, e)
	var list []struct {
		ID int64 `json:"id"`
	}
	c.do(http.MethodGet, "/api/v1/me/sessions", nil).decode(&list)
	if len(list) != 1 {
		t.Errorf("listed %d sessions, want only the active one", len(list))
	}
}

func TestRevokeOtherSession(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	phone, laptop := e.client(), e.client()
	phone.mustLogin("alice", goodPassword)
	laptop.mustLogin("alice", goodPassword)

	var who struct {
		Session int64 `json:"session_id"`
	}
	laptop.whoami().decode(&who)
	if r := phone.do(http.MethodDelete, "/api/v1/me/sessions/"+strconv.FormatInt(who.Session, 10), nil); r.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d %s", r.Code, r.Body)
	}
	laptop.whoami().expect(http.StatusUnauthorized, "unauthorized")
	if phone.whoami().Code != 200 {
		t.Error("revoking another session ended this one")
	}
	phone.do(http.MethodDelete, "/api/v1/me/sessions/"+strconv.FormatInt(who.Session, 10), nil).expect(http.StatusNotFound, "not_found")
}

func TestRevokeCurrentSessionClearsTheCookie(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	c := e.client()
	c.mustLogin("alice", goodPassword)
	var who struct {
		Session int64 `json:"session_id"`
	}
	c.whoami().decode(&who)
	r := c.do(http.MethodDelete, "/api/v1/me/sessions/"+strconv.FormatInt(who.Session, 10), nil)
	if r.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d", r.Code)
	}
	if cookie := r.sessionCookie(); cookie == nil || cookie.MaxAge >= 0 {
		t.Errorf("cookie not cleared: %+v", cookie)
	}
	c.whoami().expect(http.StatusUnauthorized, "unauthorized")
}

func TestCannotRevokeSomeoneElsesSession(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	e.addUser("bob", goodPassword, "member")
	alice, bob := e.client(), e.client()
	alice.mustLogin("alice", goodPassword)
	bob.mustLogin("bob", goodPassword)
	var who struct {
		Session int64 `json:"session_id"`
	}
	bob.whoami().decode(&who)

	alice.do(http.MethodDelete, "/api/v1/me/sessions/"+strconv.FormatInt(who.Session, 10), nil).expect(http.StatusNotFound, "not_found")
	if bob.whoami().Code != 200 {
		t.Error("bob's session was ended by alice")
	}
	for _, bad := range []string{"abc", "0", "-3", "99999999999999999999"} {
		alice.do(http.MethodDelete, "/api/v1/me/sessions/"+bad, nil).expect(http.StatusNotFound, "not_found")
	}
}
