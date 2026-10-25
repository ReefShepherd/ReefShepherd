package auth_test

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

func signedInWithTOTP(t *testing.T) (*env, *client, string, []string) {
	t.Helper()
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "admin")
	c := e.client()
	c.mustLogin("alice", goodPassword)
	secret, recovery := c.enableTOTP()
	return e, c, secret, recovery
}

// loginStep1 passes the password step and returns the browser holding the pending session.
func loginStep1(t *testing.T, e *env) *client {
	t.Helper()
	c := e.client()
	r := c.login("alice", goodPassword)
	var out struct {
		MFARequired bool `json:"mfa_required"`
	}
	r.decode(&out)
	if r.Code != 200 || !out.MFARequired || r.sessionCookie() == nil {
		t.Fatalf("expected a pending session: %d %s", r.Code, r.Body)
	}
	return c
}

func TestEnrolmentReturnsSecretURLAndQRCode(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "admin")
	if err := e.store.SetInstanceName(t.Context(), "Reef: HQ"); err != nil {
		t.Fatal(err)
	}
	c := e.client()
	c.mustLogin("alice", goodPassword)

	r := c.do(http.MethodPost, "/api/v1/me/totp/enroll", nil)
	var out struct {
		Secret string `json:"secret"`
		URL    string `json:"otpauth_url"`
		QR     string `json:"qr_code"`
	}
	r.decode(&out)
	if r.Code != 200 || len(out.Secret) < 20 {
		t.Fatalf("enroll: %d %s", r.Code, r.Body)
	}
	u, err := url.Parse(out.URL)
	if err != nil || u.Scheme != "otpauth" || u.Host != "totp" {
		t.Fatalf("otpauth url = %q (%v)", out.URL, err)
	}
	q := u.Query()
	if q.Get("secret") != out.Secret || q.Get("digits") != "6" || q.Get("period") != "30" || q.Get("algorithm") != "SHA1" || q.Get("issuer") != "Reef  HQ" {
		t.Errorf("query = %v", q)
	}
	if !strings.HasSuffix(u.Path, ":alice") {
		t.Errorf("label = %q", u.Path)
	}
	raw, ok := strings.CutPrefix(out.QR, "data:image/png;base64,")
	if !ok {
		t.Fatalf("qr_code = %.40s", out.QR)
	}
	png, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(png) < 100 || string(png[1:4]) != "PNG" {
		t.Errorf("qr_code does not hold a PNG: %v", err)
	}

	// Until confirmed the account still logs in with a password alone.
	fresh := e.client()
	fresh.mustLogin("alice", goodPassword)
	if fresh.whoami().Code != 200 {
		t.Error("an unconfirmed enrolment must not require a code")
	}
}

func TestConfirmRejectsAWrongCodeAndNeedsEnrolment(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "admin")
	c := e.client()
	c.mustLogin("alice", goodPassword)

	c.do(http.MethodPost, "/api/v1/me/totp/confirm", map[string]string{"code": "123456"}).expect(http.StatusConflict, "conflict")

	r := c.do(http.MethodPost, "/api/v1/me/totp/enroll", nil)
	var out struct {
		Secret string `json:"secret"`
	}
	r.decode(&out)
	wrong := codeAt(t, out.Secret, e.now.Add(5*time.Minute))
	c.do(http.MethodPost, "/api/v1/me/totp/confirm", map[string]string{"code": wrong}).expect(http.StatusUnprocessableEntity, "validation")
	c.do(http.MethodPost, "/api/v1/me/totp/confirm", map[string]string{"code": "abc"}).expect(http.StatusUnprocessableEntity, "validation")
	if testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM user_totp WHERE confirmed_at IS NOT NULL`) != 0 || testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM user_recovery_codes`) != 0 {
		t.Error("a wrong code activated two-step login or made recovery codes")
	}

	// A new enrolment replaces the unconfirmed secret.
	r = c.do(http.MethodPost, "/api/v1/me/totp/enroll", nil)
	var again struct {
		Secret string `json:"secret"`
	}
	r.decode(&again)
	if again.Secret == out.Secret {
		t.Error("re-enrolling kept the old secret")
	}
}

func TestConfirmGivesTenRecoveryCodesStoredHashed(t *testing.T) {
	e, _, _, recovery := signedInWithTOTP(t)
	if len(recovery) != 10 {
		t.Fatalf("%d recovery codes", len(recovery))
	}
	seen := map[string]bool{}
	for _, code := range recovery {
		if len(code) != 11 || code[5] != '-' || seen[code] {
			t.Errorf("odd or repeated code %q", code)
		}
		seen[code] = true
		plain := strings.ReplaceAll(code, "-", "")
		if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM user_recovery_codes WHERE code_hash = ? OR code_hash = ?`, code, plain); n != 0 {
			t.Errorf("recovery code %q stored in the clear", code)
		}
	}
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM user_recovery_codes WHERE used_at IS NULL AND length(code_hash) = 64`); n != 10 {
		t.Errorf("hashed unused codes = %d", n)
	}
}

func TestEnrollingAgainOnceEnabledIsRefused(t *testing.T) {
	_, c, _, _ := signedInWithTOTP(t)
	c.do(http.MethodPost, "/api/v1/me/totp/enroll", nil).expect(http.StatusConflict, "conflict")
	// Confirming twice would mint a second set of codes.
	c.do(http.MethodPost, "/api/v1/me/totp/confirm", map[string]string{"code": "123456"}).expect(http.StatusConflict, "conflict")
}

func TestLoginWithTOTPNeedsTheSecondStep(t *testing.T) {
	e, _, secret, _ := signedInWithTOTP(t)

	c := loginStep1(t, e)
	// A password-only session is not a session.
	c.whoami().expect(http.StatusUnauthorized, "unauthorized")
	if c.do(http.MethodGet, "/api/v1/me/sessions", nil).Code != http.StatusUnauthorized {
		t.Error("pending session listed sessions")
	}
	pending := c.cookie

	c.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"code": "000000"}).expect(http.StatusUnauthorized, "invalid_code")
	c.whoami().expect(http.StatusUnauthorized, "unauthorized")

	// The code of the next window is still inside the allowed drift; use the current one that confirm did not consume.
	e.now = e.now.Add(30 * time.Second)
	r := c.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"code": codeAt(t, secret, e.now)})
	if r.Code != http.StatusNoContent {
		t.Fatalf("mfa: %d %s", r.Code, r.Body)
	}
	if c.cookie == pending || c.cookie == "" {
		t.Error("the token was not replaced when the session was upgraded")
	}
	if tokenHashCount(e, pending) != 0 {
		t.Error("the pending token still resolves")
	}
	cookie := r.sessionCookie()
	if cookie == nil || !cookie.HttpOnly || cookie.MaxAge != int((30*24*time.Hour).Seconds()) {
		t.Errorf("cookie after mfa = %+v", cookie)
	}
	if c.whoami().Code != 200 {
		t.Error("not signed in after the second step")
	}
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM sessions WHERE stage = 'mfa_pending'`); n != 0 {
		t.Errorf("%d pending sessions left", n)
	}
	// The old pending cookie cannot be used to get in.
	old := e.client()
	old.cookie = pending
	old.whoami().expect(http.StatusUnauthorized, "unauthorized")
}

func TestPendingSessionLastsFiveMinutes(t *testing.T) {
	e, _, secret, _ := signedInWithTOTP(t)
	c := loginStep1(t, e)
	var expires string
	if err := e.db.R.QueryRowContext(t.Context(), `SELECT expires_at FROM sessions WHERE stage = 'mfa_pending'`).Scan(&expires); err != nil {
		t.Fatal(err)
	}
	if want := e.now.Add(5 * time.Minute).Format("2006-01-02T15:04:05Z"); expires != want {
		t.Errorf("pending expires_at = %s, want %s", expires, want)
	}
	if cookie := c.do(http.MethodGet, "/api/v1/whoami", nil).sessionCookie(); cookie != nil {
		t.Errorf("a pending session got its cookie refreshed: %+v", cookie)
	}

	e.now = e.now.Add(5*time.Minute + time.Second)
	c.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"code": codeAt(t, secret, e.now)}).expect(http.StatusUnauthorized, "unauthorized")
}

func TestReplayedTOTPCodeIsRejected(t *testing.T) {
	e, _, secret, _ := signedInWithTOTP(t)
	e.now = e.now.Add(60 * time.Second)
	code := codeAt(t, secret, e.now)

	first := loginStep1(t, e)
	if r := first.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"code": code}); r.Code != http.StatusNoContent {
		t.Fatalf("first use: %d %s", r.Code, r.Body)
	}
	// Same code, same time window, new login.
	second := loginStep1(t, e)
	second.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"code": code}).expect(http.StatusUnauthorized, "invalid_code")
	if second.whoami().Code != http.StatusUnauthorized {
		t.Error("replayed code produced a session")
	}
	// An older window's code, now that a newer one has been used, is also dead.
	older := codeAt(t, secret, e.now.Add(-30*time.Second))
	second.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"code": older}).expect(http.StatusUnauthorized, "invalid_code")

	// The next window's code is fresh.
	e.now = e.now.Add(30 * time.Second)
	if r := second.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"code": codeAt(t, secret, e.now)}); r.Code != http.StatusNoContent {
		t.Errorf("next window: %d %s", r.Code, r.Body)
	}
}

func TestTOTPAllowsOneStepOfDriftOnly(t *testing.T) {
	e, _, secret, _ := signedInWithTOTP(t)
	e.now = e.now.Add(10 * time.Minute)

	c := loginStep1(t, e)
	c.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"code": codeAt(t, secret, e.now.Add(-90*time.Second))}).expect(http.StatusUnauthorized, "invalid_code")
	c.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"code": codeAt(t, secret, e.now.Add(90*time.Second))}).expect(http.StatusUnauthorized, "invalid_code")
	if r := c.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"code": codeAt(t, secret, e.now.Add(-30*time.Second))}); r.Code != http.StatusNoContent {
		t.Errorf("one step behind: %d %s", r.Code, r.Body)
	}
}

func TestRecoveryCodeWorksOnce(t *testing.T) {
	e, _, _, recovery := signedInWithTOTP(t)

	c := loginStep1(t, e)
	// Lower case and spaces instead of the dash are fine.
	typed := strings.ToLower(strings.ReplaceAll(recovery[3], "-", " "))
	if r := c.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"recovery_code": typed}); r.Code != http.StatusNoContent {
		t.Fatalf("recovery code: %d %s", r.Code, r.Body)
	}
	if c.whoami().Code != 200 {
		t.Error("not signed in after a recovery code")
	}
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM user_recovery_codes WHERE used_at IS NOT NULL`); n != 1 {
		t.Errorf("used codes = %d", n)
	}

	again := loginStep1(t, e)
	again.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"recovery_code": recovery[3]}).expect(http.StatusUnauthorized, "invalid_code")
	if r := again.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"recovery_code": recovery[4]}); r.Code != http.StatusNoContent {
		t.Errorf("another code: %d %s", r.Code, r.Body)
	}
	other := loginStep1(t, e)
	other.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"recovery_code": "AAAAA-AAAAA"}).expect(http.StatusUnauthorized, "invalid_code")
}

func TestRecoveryCodeOfAnotherUserDoesNotWork(t *testing.T) {
	e, _, _, recovery := signedInWithTOTP(t)
	e.addUser("bob", goodPassword, "member")
	bob := e.client()
	bob.mustLogin("bob", goodPassword)
	bob.enableTOTP()

	b := e.client()
	b.login("bob", goodPassword)
	b.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"recovery_code": recovery[0]}).expect(http.StatusUnauthorized, "invalid_code")
}

func TestMFAEndpointRequiresAPendingSessionAndOneKindOfCode(t *testing.T) {
	e, signedIn, secret, recovery := signedInWithTOTP(t)
	code := map[string]string{"code": codeAt(t, secret, e.now.Add(30*time.Second))}

	e.client().do(http.MethodPost, "/api/v1/auth/mfa", code).expect(http.StatusUnauthorized, "unauthorized")
	// A full session cannot be "upgraded" again.
	signedIn.do(http.MethodPost, "/api/v1/auth/mfa", code).expect(http.StatusUnauthorized, "unauthorized")

	c := loginStep1(t, e)
	c.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{}).expect(http.StatusUnprocessableEntity, "validation")
	c.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"code": code["code"], "recovery_code": recovery[0]}).expect(http.StatusUnprocessableEntity, "validation")
	if testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM user_recovery_codes WHERE used_at IS NOT NULL`) != 0 {
		t.Error("a rejected request spent a recovery code")
	}
}

func disableTOTP(c *client, body any) *resp {
	return c.do(http.MethodDelete, "/api/v1/me/totp", body)
}

func TestDisablingTOTPRemovesSecretAndCodes(t *testing.T) {
	e, c, _, _ := signedInWithTOTP(t)
	if r := disableTOTP(c, map[string]string{"password": goodPassword}); r.Code != http.StatusNoContent {
		t.Fatalf("disable: %d %s", r.Code, r.Body)
	}
	if testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM user_totp`) != 0 || testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM user_recovery_codes`) != 0 {
		t.Error("secret or recovery codes survived")
	}
	disableTOTP(c, map[string]string{"password": goodPassword}).expect(http.StatusNotFound, "not_found")
	fresh := e.client()
	fresh.mustLogin("alice", goodPassword)
}

func TestDisablingTOTPNeedsThePassword(t *testing.T) {
	e, c, _, _ := signedInWithTOTP(t)

	for name, body := range map[string]any{
		"no body":        nil,
		"empty object":   map[string]string{},
		"blank password": map[string]string{"password": ""},
		"only a code":    map[string]string{"code": "123456"},
	} {
		r := disableTOTP(c, body)
		r.expect(http.StatusUnprocessableEntity, "validation")
		if !containsField(r.Body.String(), "password") {
			t.Errorf("%s: body = %s", name, r.Body)
		}
	}
	r := disableTOTP(c, map[string]string{"password": "not my password!!"})
	r.expect(http.StatusUnprocessableEntity, "validation")
	if !containsField(r.Body.String(), "password") {
		t.Errorf("body = %s", r.Body)
	}
	if testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM user_totp WHERE confirmed_at IS NOT NULL`) != 1 || testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM user_recovery_codes`) != 10 {
		t.Fatal("two-step login was removed without the right password")
	}
	if c.whoami().Code != 200 {
		t.Error("a failed attempt ended the session")
	}
	disableTOTP(c, map[string]string{"password": goodPassword, "extra": "x"}).expect(http.StatusUnprocessableEntity, "validation")
}

func TestDisablingTOTPWithAWrongPasswordIsRateLimited(t *testing.T) {
	e, c, _, _ := signedInWithTOTP(t)
	for range 5 {
		disableTOTP(c, map[string]string{"password": "not my password!!"}).expect(http.StatusUnprocessableEntity, "validation")
	}
	r := disableTOTP(c, map[string]string{"password": goodPassword})
	r.expect(http.StatusTooManyRequests, "too_many_requests")
	if r.Header().Get("Retry-After") == "" {
		t.Error("no Retry-After")
	}
	if testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM user_totp`) != 1 {
		t.Error("the secret was removed while locked")
	}
}

func TestAccountWithoutAPasswordNeedsACurrentCodeToDisableTOTP(t *testing.T) {
	e := newEnv(t, config.Config{AuthMode: config.AuthModeDisabled})
	e.addUser("root", "", "admin")
	c := e.client()
	secret, _ := c.enableTOTP()

	disableTOTP(c, nil).expect(http.StatusUnprocessableEntity, "validation")
	r := disableTOTP(c, map[string]string{"code": "000000"})
	r.expect(http.StatusUnprocessableEntity, "validation")
	if !containsField(r.Body.String(), "code") {
		t.Errorf("body = %s", r.Body)
	}
	// A password field means nothing for this account.
	disableTOTP(c, map[string]string{"password": goodPassword}).expect(http.StatusUnprocessableEntity, "validation")
	if testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM user_totp`) != 1 {
		t.Fatal("removed without a valid code")
	}

	// The code that confirmed enrolment is spent; the next window's code works.
	disableTOTP(c, map[string]string{"code": codeAt(t, secret, e.now)}).expect(http.StatusUnprocessableEntity, "validation")
	e.now = e.now.Add(30 * time.Second)
	if r := disableTOTP(c, map[string]string{"code": codeAt(t, secret, e.now)}); r.Code != http.StatusNoContent {
		t.Fatalf("disable with a code: %d %s", r.Code, r.Body)
	}
	if testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM user_totp`) != 0 {
		t.Error("secret survived")
	}
}

func TestWrongCodesWhenDisablingTOTPAreRateLimited(t *testing.T) {
	e := newEnv(t, config.Config{AuthMode: config.AuthModeDisabled})
	e.addUser("root", "", "admin")
	c := e.client()
	secret, _ := c.enableTOTP()
	// Move to a fresh window so the right code would otherwise be accepted.
	e.now = e.now.Add(30 * time.Second)
	for range 5 {
		disableTOTP(c, map[string]string{"code": "000000"}).expect(http.StatusUnprocessableEntity, "validation")
	}
	disableTOTP(c, map[string]string{"code": codeAt(t, secret, e.now)}).expect(http.StatusTooManyRequests, "too_many_requests")
}

func TestUnconfirmedEnrolmentCanBeCancelledWithoutProof(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "admin")
	c := e.client()
	c.mustLogin("alice", goodPassword)
	c.do(http.MethodPost, "/api/v1/me/totp/enroll", nil)
	if r := disableTOTP(c, nil); r.Code != http.StatusNoContent {
		t.Fatalf("cancel: %d %s", r.Code, r.Body)
	}
	if testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM user_totp`) != 0 {
		t.Error("pending enrolment survived")
	}
}

func TestResetMFARemovesTwoStepLoginAndOtherSessions(t *testing.T) {
	e := newEnv(t, config.Config{})
	id := e.addUser("alice", goodPassword, "admin")
	c, other := e.client(), e.client()
	c.mustLogin("alice", goodPassword)
	other.mustLogin("alice", goodPassword)
	c.enableTOTP()
	var who struct {
		Session int64 `json:"session_id"`
	}
	c.whoami().decode(&who)

	if err := e.svc.ResetMFA(t.Context(), id, who.Session); err != nil {
		t.Fatal(err)
	}
	if testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM user_totp`) != 0 || testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM user_recovery_codes`) != 0 {
		t.Error("two-step login survived the reset")
	}
	if c.whoami().Code != 200 {
		t.Error("the kept session was ended")
	}
	other.whoami().expect(http.StatusUnauthorized, "unauthorized")
	// Resetting an account that has none is fine.
	if err := e.svc.ResetMFA(t.Context(), id, 0); err != nil {
		t.Errorf("second reset: %v", err)
	}
	if testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM sessions`) != 0 {
		t.Error("keep=0 should end every session")
	}
}

func TestTheCodeThatConfirmedEnrolmentCannotBeReusedToLogIn(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "admin")
	c := e.client()
	c.mustLogin("alice", goodPassword)
	r := c.do(http.MethodPost, "/api/v1/me/totp/enroll", nil)
	var enroll struct {
		Secret string `json:"secret"`
	}
	r.decode(&enroll)
	code := codeAt(t, enroll.Secret, e.now)
	if r := c.do(http.MethodPost, "/api/v1/me/totp/confirm", map[string]string{"code": code}); r.Code != 200 {
		t.Fatal(r.Body)
	}

	login := loginStep1(t, e)
	login.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"code": code}).expect(http.StatusUnauthorized, "invalid_code")
}
