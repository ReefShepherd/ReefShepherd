package auth_test

import (
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/config"
)

// failN makes n wrong-password attempts and returns the last response.
func failN(c *client, username string, n int) *resp {
	var r *resp
	for range n {
		r = c.login(username, "wrong password here")
	}
	return r
}

func TestLoginLocksAfterFiveFailuresAndBacksOff(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	c := e.client()

	for i := 1; i <= 4; i++ {
		c.login("alice", "wrong password here").expect(http.StatusUnauthorized, "invalid_credentials")
	}
	// The fifth failure is still answered normally and starts the first lock.
	c.login("alice", "wrong password here").expect(http.StatusUnauthorized, "invalid_credentials")

	r := c.login("alice", "wrong password here")
	r.expect(http.StatusTooManyRequests, "too_many_requests")
	if got := r.Header().Get("Retry-After"); got != "30" {
		t.Errorf("Retry-After = %q, want 30", got)
	}
	// While locked even the right password is refused, so guessing cannot succeed during the lock.
	c.login("alice", goodPassword).expect(http.StatusTooManyRequests, "too_many_requests")
	if c.cookie != "" {
		t.Error("a locked login set a cookie")
	}

	e.now = e.now.Add(31 * time.Second)
	c.login("alice", "wrong password here").expect(http.StatusUnauthorized, "invalid_credentials")
	// The sixth failure doubles the lock.
	if got := c.login("alice", goodPassword).Header().Get("Retry-After"); got != "60" {
		t.Errorf("second lock Retry-After = %q, want 60", got)
	}
	e.now = e.now.Add(61 * time.Second)
	c.mustLogin("alice", goodPassword)
}

func TestLockoutDoublesUpToFifteenMinutes(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	c := e.client()
	failN(c, "alice", 5)

	var waits []string
	for range 8 {
		r := c.login("alice", goodPassword)
		r.expect(http.StatusTooManyRequests, "too_many_requests")
		wait := r.Header().Get("Retry-After")
		waits = append(waits, wait)
		secs, _ := strconv.Atoi(wait)
		e.now = e.now.Add(time.Duration(secs+1) * time.Second)
		c.login("alice", "wrong password here").expect(http.StatusUnauthorized, "invalid_credentials")
	}
	want := []string{"30", "60", "120", "240", "480", "900", "900", "900"}
	if len(waits) != len(want) {
		t.Fatalf("waits = %v", waits)
	}
	for i := range want {
		if waits[i] != want[i] {
			t.Errorf("lock %d = %ss, want %ss (all: %v)", i+1, waits[i], want[i], waits)
		}
	}
}

func TestFailuresAreRememberedWhileTheyKeepComing(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	c := e.client()
	failN(c, "alice", 5)
	// Ten minutes later the 30 second lock is over but the account is still on probation: the next mistake locks for a minute.
	e.now = e.now.Add(10 * time.Minute)
	c.login("alice", "wrong password here").expect(http.StatusUnauthorized, "invalid_credentials")
	if got := c.login("alice", goodPassword).Header().Get("Retry-After"); got != "60" {
		t.Errorf("Retry-After = %q, want 60", got)
	}
}

func TestUsernameLockSpansAddresses(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	// One failure from each of five different addresses.
	for i := range 5 {
		c := e.client()
		c.ip = "198.51.100." + strconv.Itoa(10+i)
		c.login("alice", "wrong password here").expect(http.StatusUnauthorized, "invalid_credentials")
	}
	late := e.client()
	late.ip = "203.0.113.99"
	late.login("alice", goodPassword).expect(http.StatusTooManyRequests, "too_many_requests")
	// The lock is on the name, so other users are unaffected.
	e.addUser("bob", goodPassword, "member")
	late.login("bob", goodPassword)
}

func TestUsernameLockIsCaseInsensitive(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	c := e.client()
	for _, name := range []string{"alice", "ALICE", "Alice", " alice", "aLiCe"} {
		c.login(name, "wrong password here")
	}
	c.login("alice", goodPassword).expect(http.StatusTooManyRequests, "too_many_requests")
}

func TestAddressLockSpansUsernames(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	c := e.client()
	for _, name := range []string{"u1", "u2", "u3", "u4", "u5"} {
		c.login(name, "wrong password here").expect(http.StatusUnauthorized, "invalid_credentials")
	}
	c.login("alice", goodPassword).expect(http.StatusTooManyRequests, "too_many_requests")
	other := e.client()
	other.ip = "203.0.113.50"
	other.mustLogin("alice", goodPassword)
}

func TestUnknownUsernamesAreRateLimitedToo(t *testing.T) {
	e := newEnv(t, config.Config{})
	c := e.client()
	failN(c, "ghost", 5)
	c.login("ghost", "anything at all").expect(http.StatusTooManyRequests, "too_many_requests")
}

func TestIPv6AddressesShareTheirSlash64(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	for i, ip := range []string{"2001:db8:1:2::1", "2001:db8:1:2::2", "2001:db8:1:2:aaaa::3", "2001:db8:1:2::4", "2001:db8:1:2::5"} {
		c := e.client()
		c.ip = "[" + ip + "]"
		c.login("user"+strconv.Itoa(i), "wrong password here")
	}
	c := e.client()
	c.ip = "[2001:db8:1:2:ffff::9]"
	c.login("alice", goodPassword).expect(http.StatusTooManyRequests, "too_many_requests")
	d := e.client()
	d.ip = "[2001:db8:1:3::1]"
	d.mustLogin("alice", goodPassword)
}

func TestSuccessClearsTheUsernameCounter(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	// Different addresses so only the username counter is in play.
	attempt := func(i int, pw string) *resp {
		c := e.client()
		c.ip = "198.51.100." + strconv.Itoa(i)
		return c.login("alice", pw)
	}
	for i := 1; i <= 4; i++ {
		attempt(i, "wrong password here")
	}
	if r := attempt(5, goodPassword); r.Code != 200 {
		t.Fatalf("login: %d", r.Code)
	}
	for i := 6; i <= 9; i++ {
		attempt(i, "wrong password here").expect(http.StatusUnauthorized, "invalid_credentials")
	}
	if r := attempt(10, goodPassword); r.Code != 200 {
		t.Errorf("four failures after a success locked the account: %d", r.Code)
	}
}

func TestSuccessDoesNotClearTheAddressCounter(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	e.addUser("mallory", goodPassword, "member")
	c := e.client()
	// Mallory has a real account and signs in between guesses at other accounts.
	for i := range 4 {
		c.login("victim"+strconv.Itoa(i), "guess number one")
		c.mustLogin("mallory", goodPassword)
	}
	c.login("victim4", "guess number one").expect(http.StatusUnauthorized, "invalid_credentials")
	c.login("victim5", "guess number one").expect(http.StatusTooManyRequests, "too_many_requests")
}

func TestForgottenAfterQuietPeriod(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	c := e.client()
	failN(c, "alice", 4)
	e.now = e.now.Add(16 * time.Minute)
	failN(c, "alice", 4)
	c.mustLogin("alice", goodPassword)
}

func TestForwardedAddressIsUsedOnlyFromTrustedProxies(t *testing.T) {
	trusted := config.Config{TrustedProxies: []netip.Prefix{prefix("10.0.0.0/8")}}
	e := newEnv(t, trusted)
	e.addUser("alice", goodPassword, "member")

	// Behind the proxy each real client has its own budget, and the proxy itself is not locked.
	for i := range 5 {
		c := e.client()
		c.ip = "10.0.0.2"
		c.header.Set("X-Forwarded-For", "203.0.113.7")
		c.login("nobody"+strconv.Itoa(i), "wrong password here")
	}
	locked := e.client()
	locked.ip = "10.0.0.2"
	locked.header.Set("X-Forwarded-For", "203.0.113.7")
	locked.login("alice", goodPassword).expect(http.StatusTooManyRequests, "too_many_requests")
	fine := e.client()
	fine.ip = "10.0.0.2"
	fine.header.Set("X-Forwarded-For", "203.0.113.8")
	fine.mustLogin("alice", goodPassword)

	// A client that is not a trusted proxy cannot shake off its limit by lying in the header.
	e2 := newEnv(t, trusted)
	e2.addUser("alice", goodPassword, "member")
	for i := range 5 {
		c := e2.client()
		c.ip = "192.0.2.66"
		c.header.Set("X-Forwarded-For", "203.0.113."+strconv.Itoa(100+i))
		c.login("nobody"+strconv.Itoa(i), "wrong password here")
	}
	liar := e2.client()
	liar.ip = "192.0.2.66"
	liar.header.Set("X-Forwarded-For", "203.0.113.200")
	liar.login("alice", goodPassword).expect(http.StatusTooManyRequests, "too_many_requests")
}

func TestSecondStepGuessingIsRateLimited(t *testing.T) {
	e, _, secret, _ := signedInWithTOTP(t)
	c := loginStep1(t, e)
	for range 5 {
		c.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"code": "000000"}).expect(http.StatusUnauthorized, "invalid_code")
	}
	// Even the right code is refused during the lock.
	good := map[string]string{"code": codeAt(t, secret, e.now.Add(30*time.Second))}
	e.now = e.now.Add(0)
	c.do(http.MethodPost, "/api/v1/auth/mfa", good).expect(http.StatusTooManyRequests, "too_many_requests")
	// Logging in again does not reset the count, because the lock is checked before the password.
	c2 := e.client()
	c2.login("alice", goodPassword).expect(http.StatusTooManyRequests, "too_many_requests")
	// Another address does not help with the name locked.
	c3 := e.client()
	c3.ip = "203.0.113.4"
	c3.login("alice", goodPassword).expect(http.StatusTooManyRequests, "too_many_requests")

	e.now = e.now.Add(31 * time.Second)
	if r := c.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"code": codeAt(t, secret, e.now)}); r.Code != http.StatusNoContent {
		t.Errorf("after the lock: %d %s", r.Code, r.Body)
	}
}

func TestPasswordStepAloneDoesNotClearFailuresForTOTPUsers(t *testing.T) {
	e, _, _, _ := signedInWithTOTP(t)
	// Four wrong codes from one address.
	c := loginStep1(t, e)
	for range 4 {
		c.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"code": "000000"}).expect(http.StatusUnauthorized, "invalid_code")
	}
	// A fresh password login from another address must not wipe the username's count: its next wrong code is the fifth failure.
	other := e.client()
	other.ip = "203.0.113.55"
	other.login("alice", goodPassword)
	other.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"code": "000000"}).expect(http.StatusUnauthorized, "invalid_code")
	other.do(http.MethodPost, "/api/v1/auth/mfa", map[string]string{"code": "000000"}).expect(http.StatusTooManyRequests, "too_many_requests")
}

// junk6 is the nth distinct IPv6 /64 in bracketed form for RemoteAddr.
func junk6(n int) string {
	return fmt.Sprintf("[2001:db8:%x:%x::1]", n>>16, n&0xffff)
}

// TestGuessesAtOneUsernameStayBoundedAcrossFloods replays the attack through the HTTP stack: four guesses, a flood of distinct junk usernames from distinct /64s, and again; the flood may only make the limit stricter.
func TestGuessesAtOneUsernameStayBoundedAcrossFloods(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.addUser("alice", goodPassword, "member")
	evaluated := 0
	for cycle := range 3 {
		for i := range 4 {
			c := e.client()
			c.ip = fmt.Sprintf("198.51.%d.%d", cycle+1, i+1)
			switch r := c.login("alice", "wrong password here"); r.Code {
			case http.StatusUnauthorized:
				evaluated++
			default:
				r.expect(http.StatusTooManyRequests, "too_many_requests")
			}
		}
		e.now = e.now.Add(time.Second)
		for i := range 10_001 {
			c := e.client()
			c.ip = junk6(cycle*10_001 + i + 1)
			c.login("junk"+strconv.Itoa(cycle*10_001+i), "wrong password here")
		}
	}
	if evaluated > 5 {
		t.Errorf("%d guesses at alice were evaluated across the floods, want at most 5", evaluated)
	}
	late := e.client()
	late.ip = "203.0.113.77"
	late.login("alice", goodPassword).expect(http.StatusTooManyRequests, "too_many_requests")
}
