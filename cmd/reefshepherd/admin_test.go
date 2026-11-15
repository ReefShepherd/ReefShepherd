package main

import (
	"bytes"
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/app"
	"github.com/reefshepherd/reefshepherd/internal/auth/password"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

var testHasher = password.NewHasher(password.Params{Time: 1, MemoryKiB: 64, Threads: 1})

// adminFixture is a migrated data directory, held open like a running server would hold it.
type adminFixture struct {
	t   *testing.T
	dir string
	db  *db.DB
}

func newAdminFixture(t *testing.T) *adminFixture {
	t.Helper()
	dir := t.TempDir()
	return &adminFixture{t: t, dir: dir, db: testutil.OpenDBAt(t, filepath.Join(dir, app.DBFileName))}
}

func (f *adminFixture) addUser(id int64, username, role, pw string) {
	f.t.Helper()
	var hash any
	if pw != "" {
		h, err := testHasher.Hash(f.t.Context(), pw)
		if err != nil {
			f.t.Fatal(err)
		}
		hash = h
	}
	testutil.Exec(f.t, f.db, `INSERT INTO users (id, username, role, password_hash, display_name) VALUES (?, ?, ?, ?, ?)`, id, username, role, hash, strings.ToUpper(username[:1])+username[1:])
}

func (f *adminFixture) addSession(userID int64, tokenByte byte) {
	f.t.Helper()
	testutil.Exec(f.t, f.db, `INSERT INTO sessions (token_hash, user_id, stage, auth_method, expires_at) VALUES (?, ?, 'active', 'password', '2999-01-01T00:00:00Z')`, []byte{tokenByte}, userID)
}

func (f *adminFixture) setting(key string) string {
	f.t.Helper()
	var v string
	_ = f.db.R.QueryRowContext(f.t.Context(), `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	return v
}

func (f *adminFixture) admin(args ...string) (code int, stdout, stderr string) {
	f.t.Helper()
	return f.adminEnv(noEnv, args...)
}

func (f *adminFixture) adminEnv(env func(string) (string, bool), args ...string) (code int, stdout, stderr string) {
	f.t.Helper()
	var out, errBuf bytes.Buffer
	full := append([]string{"admin"}, args...)
	full = append(full, "--data-dir", f.dir)
	code = run(context.Background(), full, env, &out, &errBuf)
	return code, out.String(), errBuf.String()
}

var newPasswordLine = regexp.MustCompile(`New password for (\S+): (\S+)\n`)

func TestAdminSetAuthMode(t *testing.T) {
	f := newAdminFixture(t)
	f.addUser(1, "alice", "admin", goodPassword)
	f.addUser(2, "bob", "admin", "")

	code, out, errOut := f.admin("set-auth-mode", "disabled")
	if code != 0 || !strings.Contains(out, "Login mode set to disabled") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
	if f.setting("auth_mode") != "disabled" || f.setting("auth_disabled_user_id") != "1" {
		t.Errorf("auth_mode=%q acting user=%q", f.setting("auth_mode"), f.setting("auth_disabled_user_id"))
	}

	code, out, _ = f.admin("set-auth-mode", "password")
	if code != 0 || !strings.Contains(out, "Login mode set to password") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	if f.setting("auth_mode") != "password" || f.setting("auth_disabled_user_id") != "" {
		t.Errorf("auth_mode=%q acting user=%q", f.setting("auth_mode"), f.setting("auth_disabled_user_id"))
	}
}

func TestAdminSetAuthModeRefusesToLockEveryoneOut(t *testing.T) {
	f := newAdminFixture(t)
	f.addUser(1, "nopass", "admin", "")
	code, _, errOut := f.admin("set-auth-mode", "password")
	if code != 1 || !strings.Contains(errOut, "password") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
	if f.setting("auth_mode") != "" {
		t.Error("the mode changed")
	}
	if code, _, errOut := f.admin("set-auth-mode", "oidc"); code != 1 || errOut == "" {
		t.Errorf("unknown mode: code=%d err=%q", code, errOut)
	}
	testutil.Exec(f.t, f.db, `DELETE FROM users`)
	if code, _, _ := f.admin("set-auth-mode", "disabled"); code != 1 {
		t.Errorf("disabling with no users: code=%d", code)
	}
}

func TestAdminSetAuthModeMentionsAnEnvironmentOverride(t *testing.T) {
	f := newAdminFixture(t)
	f.addUser(1, "alice", "admin", goodPassword)
	env := func(k string) (string, bool) {
		if k == "REEFSHEPHERD_AUTH_MODE" {
			return "disabled", true
		}
		return "", false
	}
	code, _, errOut := f.adminEnv(env, "set-auth-mode", "password")
	if code != 0 || !strings.Contains(errOut, "REEFSHEPHERD_AUTH_MODE") || !strings.Contains(errOut, "disabled") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
	if f.setting("auth_mode") != "password" {
		t.Error("the stored mode should still be updated")
	}
	_, _, errOut = f.adminEnv(env, "set-auth-mode", "disabled")
	if strings.Contains(errOut, "takes precedence") {
		t.Errorf("no warning is needed when the override already matches: %q", errOut)
	}
}

const goodPassword = "correct horse battery"

func TestAdminResetPassword(t *testing.T) {
	f := newAdminFixture(t)
	f.addUser(1, "alice", "admin", goodPassword)
	f.addUser(2, "bob", "member", goodPassword)
	f.addSession(1, 1)
	f.addSession(1, 2)
	f.addSession(2, 3)

	code, out, errOut := f.admin("reset-password", "ALICE")
	if code != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
	m := newPasswordLine.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no password in output %q", out)
	}
	name, pw := m[1], m[2]
	if name != "alice" {
		t.Errorf("name = %q", name)
	}
	if len(pw) != 20 || strings.Trim(pw, generatedPasswordChars) != "" {
		t.Errorf("generated password %q", pw)
	}
	if err := password.Validate(pw); err != nil {
		t.Errorf("generated password breaks the rules: %v", err)
	}

	var hash string
	if err := f.db.R.QueryRowContext(t.Context(), `SELECT password_hash FROM users WHERE id = 1`).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if ok, _, _ := testHasher.Verify(t.Context(), pw, hash); !ok {
		t.Error("the printed password does not match the stored hash")
	}
	if ok, _, _ := testHasher.Verify(t.Context(), goodPassword, hash); ok {
		t.Error("the old password still works")
	}
	if strings.Contains(hash, pw) {
		t.Error("password stored in the clear")
	}
	if testutil.Count(f.t, f.db, `SELECT COUNT(*) FROM users WHERE id = 1 AND password_changed_at IS NOT NULL`) != 1 {
		t.Error("password_changed_at not set")
	}
	if testutil.Count(f.t, f.db, `SELECT COUNT(*) FROM sessions WHERE user_id = 1`) != 0 {
		t.Error("the user's sessions survived")
	}
	if testutil.Count(f.t, f.db, `SELECT COUNT(*) FROM sessions WHERE user_id = 2`) != 1 {
		t.Error("another user's session was ended")
	}
	if testutil.Count(f.t, f.db, `SELECT COUNT(*) FROM users WHERE id = 2 AND password_hash LIKE '$argon2id%'`) != 1 {
		t.Error("another user's password changed")
	}

	_, out2, _ := f.admin("reset-password", "alice")
	if m2 := newPasswordLine.FindStringSubmatch(out2); m2 == nil || m2[2] == pw {
		t.Errorf("second reset gave the same password: %q", out2)
	}
}

func TestAdminResetPasswordWorksForDisabledAndPasswordlessAccounts(t *testing.T) {
	f := newAdminFixture(t)
	f.addUser(1, "nopass", "admin", "")
	f.addUser(2, "off", "member", goodPassword)
	testutil.Exec(f.t, f.db, `UPDATE users SET disabled = 1 WHERE id = 2`)
	for _, name := range []string{"nopass", "off"} {
		if code, out, errOut := f.admin("reset-password", name); code != 0 || newPasswordLine.FindStringSubmatch(out) == nil {
			t.Errorf("%s: code=%d out=%q err=%q", name, code, out, errOut)
		}
	}
	if testutil.Count(f.t, f.db, `SELECT COUNT(*) FROM users WHERE password_hash IS NULL`) != 0 {
		t.Error("a password was not stored")
	}
	if testutil.Count(f.t, f.db, `SELECT COUNT(*) FROM users WHERE id = 2 AND disabled = 1`) != 1 {
		t.Error("resetting a password must not re-enable the account")
	}
}

func TestAdminResetPasswordUnknownUser(t *testing.T) {
	f := newAdminFixture(t)
	f.addUser(1, "alice", "admin", goodPassword)
	code, out, errOut := f.admin("reset-password", "ghost")
	if code != 1 || !strings.Contains(errOut, `no user named "ghost"`) || out != "" {
		t.Errorf("code=%d out=%q err=%q", code, out, errOut)
	}
}

func TestAdminResetMFA(t *testing.T) {
	f := newAdminFixture(t)
	f.addUser(1, "alice", "admin", goodPassword)
	f.addUser(2, "bob", "member", goodPassword)
	for _, id := range []int{1, 2} {
		testutil.Exec(f.t, f.db, `INSERT INTO user_totp (user_id, secret, confirmed_at) VALUES (?, 'JBSWY3DPEHPK3PXP', '2026-01-01T00:00:00Z')`, id)
		testutil.Exec(f.t, f.db, `INSERT INTO user_recovery_codes (user_id, code_hash) VALUES (?, 'abc')`, id)
		f.addSession(int64(id), byte(id))
	}
	code, out, errOut := f.admin("reset-mfa", "alice")
	if code != 0 || !strings.Contains(out, "alice") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
	if testutil.Count(f.t, f.db, `SELECT COUNT(*) FROM user_totp WHERE user_id = 1`) != 0 || testutil.Count(f.t, f.db, `SELECT COUNT(*) FROM user_recovery_codes WHERE user_id = 1`) != 0 {
		t.Error("alice's two-step login survived")
	}
	if testutil.Count(f.t, f.db, `SELECT COUNT(*) FROM sessions WHERE user_id = 1`) != 0 {
		t.Error("alice's sessions survived")
	}
	if testutil.Count(f.t, f.db, `SELECT COUNT(*) FROM user_totp WHERE user_id = 2`) != 1 || testutil.Count(f.t, f.db, `SELECT COUNT(*) FROM user_recovery_codes WHERE user_id = 2`) != 1 || testutil.Count(f.t, f.db, `SELECT COUNT(*) FROM sessions WHERE user_id = 2`) != 1 {
		t.Error("bob was affected")
	}
	// Running it again for an account without two-step login is fine.
	if code, _, errOut := f.admin("reset-mfa", "alice"); code != 0 {
		t.Errorf("second run: code=%d err=%q", code, errOut)
	}
	if code, _, errOut := f.admin("reset-mfa", "ghost"); code != 1 || !strings.Contains(errOut, `no user named "ghost"`) {
		t.Errorf("unknown user: code=%d err=%q", code, errOut)
	}
}

func TestAdminListUsers(t *testing.T) {
	f := newAdminFixture(t)
	f.addUser(1, "alice", "admin", goodPassword)
	f.addUser(2, "bob", "member", "")
	testutil.Exec(f.t, f.db, `UPDATE users SET disabled = 1 WHERE id = 2`)
	testutil.Exec(f.t, f.db, `INSERT INTO user_totp (user_id, secret, confirmed_at) VALUES (1, 'JBSWY3DPEHPK3PXP', '2026-01-01T00:00:00Z')`)

	code, out, errOut := f.admin("list-users")
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("output:\n%s", out)
	}
	header := strings.Fields(lines[0])
	if strings.Join(header, " ") != "ID USERNAME ROLE DISABLED PASSWORD 2FA NAME" {
		t.Errorf("header = %q", lines[0])
	}
	if got := strings.Fields(lines[1]); strings.Join(got, " ") != "1 alice admin no yes yes Alice" {
		t.Errorf("alice row = %q", lines[1])
	}
	if got := strings.Fields(lines[2]); strings.Join(got, " ") != "2 bob member yes no no Bob" {
		t.Errorf("bob row = %q", lines[2])
	}
	if strings.Contains(out, "argon2") || strings.Contains(out, "JBSWY3") {
		t.Error("list-users printed a secret")
	}
}

func TestAdminWorksWhileTheServerHoldsTheDatabase(t *testing.T) {
	f := newAdminFixture(t)
	f.addUser(1, "alice", "admin", goodPassword)
	// The fixture's own pools stay open and keep serving reads, as the running service would.
	if code, _, errOut := f.admin("reset-password", "alice"); code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	if testutil.Count(f.t, f.db, `SELECT COUNT(*) FROM users`) != 1 {
		t.Error("the server's connection no longer sees the data")
	}
}

func TestAdminUsageErrors(t *testing.T) {
	f := newAdminFixture(t)
	if code, _, errOut := runCmd("admin"); code != 2 || !strings.Contains(errOut, "Usage: reefshepherd admin") {
		t.Errorf("no subcommand: code=%d err=%q", code, errOut)
	}
	if code, out, _ := runCmd("admin", "help"); code != 0 || !strings.Contains(out, "reset-password") {
		t.Errorf("help: code=%d out=%q", code, out)
	}
	if code, _, errOut := runCmd("admin", "frobnicate"); code != 2 || !strings.Contains(errOut, `unknown command "frobnicate"`) {
		t.Errorf("unknown: code=%d err=%q", code, errOut)
	}
	for _, sub := range []string{"set-auth-mode", "reset-password", "reset-mfa"} {
		if code, _, errOut := runCmd("admin", sub); code != 2 || !strings.Contains(errOut, "missing argument") {
			t.Errorf("%s with no argument: code=%d err=%q", sub, code, errOut)
		}
		if code, _, _ := runCmd("admin", sub, "--data-dir", f.dir); code != 2 {
			t.Errorf("%s with only flags: code=%d", sub, code)
		}
	}
	if code, _, errOut := f.admin("list-users", "--listen", "nonsense"); code != 2 || !strings.Contains(errOut, "REEFSHEPHERD_LISTEN") {
		t.Errorf("bad flag: code=%d err=%q", code, errOut)
	}
	if code, _, errOut := f.admin("list-users", "extra"); code != 2 || !strings.Contains(errOut, "unexpected argument") {
		t.Errorf("stray argument: code=%d err=%q", code, errOut)
	}
}

func TestAdminNeedsAnInitialisedDatabase(t *testing.T) {
	dir := t.TempDir()
	var out, errBuf bytes.Buffer
	if code := run(context.Background(), []string{"admin", "list-users", "--data-dir", dir}, noEnv, &out, &errBuf); code != 1 || !strings.Contains(errBuf.String(), "no database") {
		t.Errorf("missing database: code=%d err=%q", code, errBuf.String())
	}
	if _, err := db.Open(filepath.Join(dir, app.DBFileName)); err != nil {
		t.Fatal(err)
	}
	errBuf.Reset()
	if code := run(context.Background(), []string{"admin", "list-users", "--data-dir", dir}, noEnv, &out, &errBuf); code != 1 || !strings.Contains(errBuf.String(), "start the server once first") {
		t.Errorf("empty database: code=%d err=%q", code, errBuf.String())
	}
}

func TestRandomPassword(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		pw, err := randomPassword()
		if err != nil {
			t.Fatal(err)
		}
		if len(pw) != generatedPasswordLen || strings.Trim(pw, generatedPasswordChars) != "" {
			t.Fatalf("password %q", pw)
		}
		if seen[pw] {
			t.Fatalf("repeated password %q", pw)
		}
		seen[pw] = true
	}
	for _, c := range "0O1lI" {
		if strings.ContainsRune(generatedPasswordChars, c) {
			t.Errorf("look-alike character %q in the alphabet", c)
		}
	}
}

func TestAdminSetAuthModeIsAllOrNothing(t *testing.T) {
	f := newAdminFixture(t)
	f.addUser(1, "alice", "admin", goodPassword)
	// The second write of the switch fails; the acting-user write before it must not stay behind.
	testutil.Exec(f.t, f.db, `CREATE TRIGGER fail_mode BEFORE INSERT ON settings WHEN NEW.key = 'auth_mode' BEGIN SELECT RAISE(ABORT, 'disk full'); END`)
	code, _, errOut := f.admin("set-auth-mode", "disabled")
	if code != 1 || !strings.Contains(errOut, "disk full") {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	if f.setting("auth_disabled_user_id") != "" || f.setting("auth_mode") != "" {
		t.Errorf("partial switch left behind: user=%q mode=%q", f.setting("auth_disabled_user_id"), f.setting("auth_mode"))
	}
}

func TestAdminOpensTheFileTheServerUses(t *testing.T) {
	if app.DBFileName != "reefshepherd.db" {
		t.Fatalf("DBFileName = %q", app.DBFileName)
	}
	f := newAdminFixture(t)
	f.addUser(1, "alice", "admin", goodPassword)
	// The fixture creates the database under the server's file name, so finding the user proves the CLI looks there.
	if code, out, _ := f.admin("list-users"); code != 0 || !strings.Contains(out, "alice") {
		t.Errorf("code=%d out=%q", code, out)
	}
}
