package auth_test

import (
	"net/http"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

type who struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Session  int64  `json:"session_id"`
}

func TestDisabledModeActsAsTheConfiguredUser(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("first", goodPassword, "admin")
	second := e.addUser("second", goodPassword, "admin")
	if err := e.store.SetAuthDisabledUserID(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	if err := e.store.SetAuthMode(t.Context(), config.AuthModeDisabled); err != nil {
		t.Fatal(err)
	}

	var w who
	r := e.client().whoami()
	r.decode(&w)
	if r.Code != 200 || w.ID != second || w.Username != "second" {
		t.Fatalf("whoami: %d %s", r.Code, r.Body)
	}
	if w.Session != 0 {
		t.Error("disabled mode must not invent a session id")
	}
	if r.sessionCookie() != nil {
		t.Error("disabled mode set a cookie")
	}
	if testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM sessions`) != 0 {
		t.Error("disabled mode created sessions")
	}
	// Any session cookie is ignored in this mode.
	c := e.client()
	c.cookie = "whatever"
	if c.whoami().Code != 200 {
		t.Error("a stray cookie broke disabled mode")
	}
	e.client().login("second", goodPassword).expect(http.StatusConflict, "conflict")
}

func TestDisabledModeFallsBackToTheFirstEnabledAdmin(t *testing.T) {
	e := newEnv(t, config.Config{})
	member := e.addUser("member", goodPassword, "member")
	off := e.addUser("off", goodPassword, "admin")
	want := e.addUser("real", goodPassword, "admin")
	testutil.Exec(e.t, e.db, `UPDATE users SET disabled = 1 WHERE id = ?`, off)
	if err := e.store.SetAuthMode(t.Context(), config.AuthModeDisabled); err != nil {
		t.Fatal(err)
	}

	var w who
	e.client().whoami().decode(&w)
	if w.ID != want {
		t.Errorf("acting as %d, want the first enabled admin %d", w.ID, want)
	}

	// A stored user that is a disabled account is not used either.
	if err := e.store.SetAuthDisabledUserID(t.Context(), off); err != nil {
		t.Fatal(err)
	}
	e.client().whoami().decode(&w)
	if w.ID != want {
		t.Errorf("acting as disabled user %d", w.ID)
	}
	// A stored member never becomes the acting user: login being off must not leave every visitor without admin rights.
	if err := e.store.SetAuthDisabledUserID(t.Context(), member); err != nil {
		t.Fatal(err)
	}
	e.client().whoami().decode(&w)
	if w.ID != want {
		t.Errorf("acting as %d, want the first enabled admin %d instead of the stored member", w.ID, want)
	}
}

func TestDisabledModeWithNobodyToActAsStaysClosed(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("member", goodPassword, "member")
	if err := e.store.SetAuthMode(t.Context(), config.AuthModeDisabled); err != nil {
		t.Fatal(err)
	}
	e.client().whoami().expect(http.StatusUnauthorized, "unauthorized")
}

func TestEnvironmentOverrideBeatsTheStoredMode(t *testing.T) {
	// Stored mode says password, the environment says disabled: no login needed.
	e := newEnv(t, config.Config{AuthMode: config.AuthModeDisabled})
	id := e.addUser("root", goodPassword, "admin")
	var w who
	r := e.client().whoami()
	r.decode(&w)
	if r.Code != 200 || w.ID != id {
		t.Errorf("env disabled: %d %s", r.Code, r.Body)
	}

	// Stored mode says disabled, the environment says password: login is required again.
	e2 := newEnv(t, config.Config{AuthMode: config.AuthModePassword})
	e2.addUser("root", goodPassword, "admin")
	if err := e2.store.SetAuthMode(t.Context(), config.AuthModeDisabled); err != nil {
		t.Fatal(err)
	}
	c := e2.client()
	c.whoami().expect(http.StatusUnauthorized, "unauthorized")
	c.mustLogin("root", goodPassword)
	if c.whoami().Code != 200 {
		t.Error("not signed in")
	}
}

func TestPasswordModeIsTheDefault(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("root", goodPassword, "admin")
	e.client().whoami().expect(http.StatusUnauthorized, "unauthorized")
}
