package auth

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

func TestLockoutSchedule(t *testing.T) {
	want := map[int]time.Duration{
		0: 0, 1: 0, 4: 0,
		5: 30 * time.Second, 6: time.Minute, 7: 2 * time.Minute, 8: 4 * time.Minute, 9: 8 * time.Minute,
		10: 15 * time.Minute, 11: 15 * time.Minute, 500: 15 * time.Minute,
	}
	for failures, d := range want {
		if got := lockout(failures); got != d {
			t.Errorf("lockout(%d) = %v, want %v", failures, got, d)
		}
	}
}

func TestIPKeyGroupsIPv6ByPrefix(t *testing.T) {
	a := ipKey(netip.MustParseAddr("2001:db8:1:2::1"))
	b := ipKey(netip.MustParseAddr("2001:db8:1:2:ffff:ffff:ffff:ffff"))
	c := ipKey(netip.MustParseAddr("2001:db8:1:3::1"))
	if a != b || a == c {
		t.Errorf("keys %q %q %q", a, b, c)
	}
	if got := ipKey(netip.MustParseAddr("192.0.2.5")); got != "192.0.2.5" {
		t.Errorf("v4 key = %q", got)
	}
}

func TestMatchStep(t *testing.T) {
	const secret = "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"
	at := time.Unix(1_700_000_000, 0)
	current := at.Unix() / 30
	code := func(step int64) string {
		c, err := totp.GenerateCodeCustom(secret, time.Unix(step*30, 0), totpOpts())
		if err != nil {
			t.Fatal(err)
		}
		return c
	}

	for _, step := range []int64{current - 1, current, current + 1} {
		if got, ok := matchStep(secret, code(step), at, 0); !ok || got != step {
			t.Errorf("step %d: got %d, %v", step, got, ok)
		}
	}
	for _, step := range []int64{current - 2, current + 2, current - 50} {
		if _, ok := matchStep(secret, code(step), at, 0); ok {
			t.Errorf("step %d must not match", step)
		}
	}
	// A step already used, or older than one used, is refused.
	if _, ok := matchStep(secret, code(current), at, current); ok {
		t.Error("the used step matched again")
	}
	if _, ok := matchStep(secret, code(current-1), at, current); ok {
		t.Error("a step older than the last used one matched")
	}
	if got, ok := matchStep(secret, code(current+1), at, current); !ok || got != current+1 {
		t.Errorf("newer step: %d, %v", got, ok)
	}
	for _, bad := range []string{"", "12345", "1234567", "abcdef", " 123456"} {
		if _, ok := matchStep(secret, bad, at, 0); ok {
			t.Errorf("%q matched", bad)
		}
	}
	if _, ok := matchStep("not base32!!", "123456", at, 0); ok {
		t.Error("a broken secret matched")
	}
}

func TestRecoveryCodeNormalisation(t *testing.T) {
	want := hashRecoveryCode("ABCDE-FGHJK")
	for _, in := range []string{"abcde-fghjk", "ABCDE FGHJK", "abcdefghjk", " AbCdE-fGhJk "} {
		if got := hashRecoveryCode(in); got != want {
			t.Errorf("%q hashes differently", in)
		}
	}
	if hashRecoveryCode("ABCDE-FGHJL") == want {
		t.Error("different codes share a hash")
	}
}

// recordFailure counts a try against both keys without the lock check, so a test can fill the tables from one address.
func recordFailure(l *limiter, ip netip.Addr, name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.ips.record(ipKey(ip), now)
	l.names.record(clampKey(name), now)
}

func TestLimiterMemoryIsBounded(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newLimiter(func() time.Time { return now }, testutil.Logger(t))
	ip := netip.MustParseAddr("192.0.2.1")
	for i := range maxTracked + 500 {
		now = now.Add(time.Millisecond)
		recordFailure(l, ip, "user"+strconv.Itoa(i))
	}
	if len(l.names.own) > maxTracked {
		t.Errorf("tracking %d usernames exactly, limit %d", len(l.names.own), maxTracked)
	}
	if _, kept := l.names.own["user"+strconv.Itoa(maxTracked+499)]; !kept {
		t.Error("the newest username was not tracked")
	}
	if _, kept := l.names.own["user0"]; kept {
		t.Error("the oldest username should have been evicted")
	}
	// Eviction loses only the exact entry; the sketch still knows every failure.
	if got := l.names.sketch.count("user0", now); got < 1 {
		t.Errorf("the sketch forgot user0: count %d", got)
	}
}

func TestLimiterEvictsStaleEntriesBeforeLiveOnes(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newLimiter(func() time.Time { return now }, testutil.Logger(t))
	ip := netip.MustParseAddr("192.0.2.1")
	// The veteran's last failure is the oldest of all, but its 15 minute lock keeps it live for 29 minutes.
	for range 10 {
		recordFailure(l, ip, "veteran")
	}
	now = now.Add(13 * time.Minute)
	recordFailure(l, ip, "old")
	now = now.Add(16 * time.Minute)
	for i := range maxTracked - 2 {
		recordFailure(l, netip.MustParseAddr("198.51.100.1"), "live"+strconv.Itoa(i))
	}
	recordFailure(l, ip, "newcomer")
	if _, ok := l.names.own["old"]; ok {
		t.Error("the stale entry was kept")
	}
	for _, k := range []string{"veteran", "live0"} {
		if _, ok := l.names.own[k]; !ok {
			t.Errorf("live entry %q was evicted while a stale one existed", k)
		}
	}
}

func TestLimiterTruncatesHugeKeys(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newLimiter(func() time.Time { return now }, testutil.Logger(t))
	ip := netip.MustParseAddr("192.0.2.1")
	recordFailure(l, ip, strings.Repeat("a", 100000))
	for k := range l.names.own {
		if len(k) > maxKeyLen {
			t.Errorf("key of %d bytes stored", len(k))
		}
	}
}

func TestBeginCountsTheTryAtOnceAndRefusesWhileLocked(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newLimiter(func() time.Time { return now }, testutil.Logger(t))
	ip := netip.MustParseAddr("192.0.2.1")

	for i := 1; i <= freeFailures; i++ {
		if _, wait := l.begin(ip, "alice"); wait != 0 {
			t.Fatalf("try %d refused for %v", i, wait)
		}
	}
	// Five tries were admitted without any of them being settled; the sixth is refused.
	_, wait := l.begin(ip, "alice")
	if wait != firstLockout {
		t.Fatalf("sixth try wait = %v, want %v", wait, firstLockout)
	}
	if l.names.own["alice"].failures != freeFailures || l.ips.own[ipKey(ip)].failures != freeFailures {
		t.Errorf("a refused try was counted: names=%d ips=%d", l.names.own["alice"].failures, l.ips.own[ipKey(ip)].failures)
	}
	if got := l.names.sketch.count("alice", now); got != freeFailures {
		t.Errorf("sketch count = %d, want %d", got, freeFailures)
	}
	// A different name from the same locked address is refused too, and vice versa.
	if _, wait := l.begin(ip, "bob"); wait == 0 {
		t.Error("a locked address admitted another username")
	}
	if _, wait := l.begin(netip.MustParseAddr("192.0.2.2"), "alice"); wait == 0 {
		t.Error("a locked username admitted another address")
	}
	if len(l.names.own) != 1 || len(l.ips.own) != 1 {
		t.Errorf("a refused try created entries: names=%d ips=%d", len(l.names.own), len(l.ips.own))
	}
	if got := l.names.sketch.count("bob", now); got != 0 {
		t.Errorf("a refused try counted in the sketch: bob=%d", got)
	}
}

func TestSucceedAndPartialSettleATry(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newLimiter(func() time.Time { return now }, testutil.Logger(t))
	ip := netip.MustParseAddr("192.0.2.1")

	for range 3 {
		recordFailure(l, ip, "alice")
	}
	a, _ := l.begin(ip, "alice")
	l.succeed(a)
	if _, ok := l.names.own["alice"]; ok {
		t.Error("success left the username's failures")
	}
	if got := l.names.sketch.count("alice", now); got != 0 {
		t.Errorf("success left %d failures in the sketch", got)
	}
	if got := l.ips.own[ipKey(ip)].failures; got != 3 {
		t.Errorf("address failures = %d, want 3: success must only take back its own try", got)
	}
	if got := l.ips.sketch.count(ipKey(ip), now); got != 3 {
		t.Errorf("address sketch count = %d, want 3", got)
	}

	for range 2 {
		recordFailure(l, ip, "bob")
	}
	a, _ = l.begin(ip, "bob")
	l.partial(a)
	if got := l.names.own["bob"].failures; got != 2 {
		t.Errorf("username failures after a partial = %d, want 2", got)
	}
	if got := l.names.sketch.count("bob", now); got != 2 {
		t.Errorf("username sketch count after a partial = %d, want 2", got)
	}
	if got := l.ips.own[ipKey(ip)].failures; got != 5 {
		t.Errorf("address failures = %d, want 3+2", got)
	}
}

func TestSettlingTheTryThatSetTheLockLiftsIt(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newLimiter(func() time.Time { return now }, testutil.Logger(t))
	ip := netip.MustParseAddr("192.0.2.1")
	for range freeFailures - 1 {
		recordFailure(l, ip, "alice")
	}
	a, wait := l.begin(ip, "alice") // the fifth try sets the lock
	if wait != 0 {
		t.Fatal("fifth try refused")
	}
	if _, wait := l.begin(ip, "carol"); wait == 0 {
		t.Fatal("the lock set by the fifth try is missing")
	}
	l.partial(a)
	if _, wait := l.begin(ip, "alice"); wait != 0 {
		t.Errorf("a try that turned out fine left a lock of %v", wait)
	}
}

// junkAddr is the nth distinct IPv6 /64.
func junkAddr(n int) netip.Addr {
	return netip.MustParseAddr(fmt.Sprintf("2001:db8:%x:%x::1", n>>16, n&0xffff))
}

// shares reports whether two keys hash to a common cell, so that one's failures would count for the other.
func shares(s *sketch, a, b string) bool {
	ia, ib := s.index(a), s.index(b)
	for i := range ia {
		if ia[i] == ib[i] {
			return true
		}
	}
	return false
}

// flood tries n distinct junk usernames tries times each, every one from its own /64, skipping junk whose cells overlap the keys under test so that their counts stay exact; junk refused by collisions among the flood itself does not count towards n.
func flood(l *limiter, n, tries int, name string, ips ...netip.Addr) {
	sent := 0
	for i := 1; sent < n; i++ {
		junk, addr := "junk"+strconv.Itoa(i), junkAddr(i)
		skip := shares(l.names.sketch, junk, name)
		for _, ip := range ips {
			skip = skip || shares(l.ips.sketch, ipKey(addr), ipKey(ip))
		}
		if skip {
			continue
		}
		if _, wait := l.begin(addr, junk); wait > 0 {
			continue
		}
		for range tries - 1 {
			l.begin(addr, junk)
		}
		sent++
	}
}

// addrs returns n distinct IPv4 addresses for one target, so the address table never locks before the username does.
func addrs(block, n int) []netip.Addr {
	out := make([]netip.Addr, n)
	for i := range out {
		out[i] = netip.MustParseAddr(fmt.Sprintf("198.51.%d.%d", block, i+1))
	}
	return out
}

func TestLockedUsernameSurvivesAFullCapFlood(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newLimiter(func() time.Time { return now }, testutil.Logger(t))
	ips := addrs(100, freeFailures+2)
	for i := range freeFailures {
		l.begin(ips[i], "alice")
	}
	if _, wait := l.begin(ips[5], "alice"); wait == 0 {
		t.Fatal("alice should be locked")
	}
	now = now.Add(time.Second)
	flood(l, maxTracked+1, 1, "alice", ips...)
	if _, wait := l.begin(ips[6], "alice"); wait == 0 {
		t.Error("a flood of junk usernames lifted the lock")
	}
	if len(l.names.own) > maxTracked || len(l.ips.own) > maxTracked {
		t.Errorf("tables grew past the cap: names=%d ips=%d", len(l.names.own), len(l.ips.own))
	}
}

func TestUsernameOnProbationSurvivesAFullCapFlood(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newLimiter(func() time.Time { return now }, testutil.Logger(t))
	ips := addrs(100, freeFailures+2)
	for i := range freeFailures {
		l.begin(ips[i], "alice")
	}
	// The 30 second lock is over, but the failures still count until a quiet period passes.
	now = now.Add(firstLockout + time.Second)
	flood(l, maxTracked+1, 1, "alice", ips...)
	if got := l.names.sketch.count("alice", now); got != freeFailures {
		t.Fatalf("alice's failures were forgotten: sketch count %d, want %d", got, freeFailures)
	}
	if _, wait := l.begin(ips[5], "alice"); wait != 0 {
		t.Fatalf("the lock should be over: %v", wait)
	}
	// The history carried on, so the sixth failure locks for a minute, not thirty seconds.
	if _, wait := l.begin(ips[6], "alice"); wait != 2*firstLockout {
		t.Errorf("wait after the sixth failure = %v, want %v", wait, 2*firstLockout)
	}
}

func TestLockedAddressSurvivesAFullCapFlood(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newLimiter(func() time.Time { return now }, testutil.Logger(t))
	home := netip.MustParseAddr("192.0.2.50")
	for i := range freeFailures {
		l.begin(home, "user"+strconv.Itoa(i))
	}
	if _, wait := l.begin(home, "another"); wait == 0 {
		t.Fatal("the address should be locked")
	}
	now = now.Add(time.Second)
	flood(l, maxTracked+1, 1, "fresh-name", home)
	if _, wait := l.begin(home, "fresh-name"); wait == 0 {
		t.Error("a flood of junk usernames lifted the address lock")
	}
}

// TestFailuresSurviveRepeatedFloods replays the attack that beat the capped map: a few guesses, a flood that fills the exact table, and again.
func TestFailuresSurviveRepeatedFloods(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newLimiter(func() time.Time { return now }, testutil.Logger(t))
	ips := addrs(100, 13)
	var admitted []int
	for cycle := range 3 {
		n := 0
		for i := range 4 {
			if _, wait := l.begin(ips[cycle*4+i], "alice"); wait == 0 {
				n++
			}
		}
		admitted = append(admitted, n)
		now = now.Add(time.Second)
		flood(l, maxTracked+1, 1, "alice", ips...)
	}
	// Four guesses in the first cycle, the fifth in the second cycle locks, nothing after that.
	if want := []int{4, 1, 0}; fmt.Sprint(admitted) != fmt.Sprint(want) {
		t.Errorf("guesses admitted per cycle = %v, want %v", admitted, want)
	}
	if _, wait := l.begin(ips[12], "alice"); wait == 0 {
		t.Error("alice is not locked after five failures spread across floods")
	}
	if len(l.names.own) > maxTracked {
		t.Errorf("exact table grew past the cap: %d", len(l.names.own))
	}
}

func TestFullTableOfLocksStillAdmitsNewKeysAndKeepsEveryLock(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newLimiter(func() time.Time { return now }, testutil.Logger(t))
	newcomer := netip.MustParseAddr("203.0.113.1")
	flood(l, maxTracked, freeFailures, "newcomer", newcomer)
	if len(l.names.own) != maxTracked || len(l.ips.own) != maxTracked {
		t.Fatalf("setup: names=%d ips=%d", len(l.names.own), len(l.ips.own))
	}
	now = now.Add(time.Second)

	// Nothing has to be refused to make room: the sketch is not a table that fills up.
	if a, wait := l.begin(newcomer, "newcomer"); wait != 0 || a == (attempt{}) {
		t.Errorf("a new key was refused by a table of locks: wait=%v", wait)
	}
	if len(l.names.own) != maxTracked || len(l.ips.own) != maxTracked {
		t.Errorf("the cap was exceeded: names=%d ips=%d", len(l.names.own), len(l.ips.own))
	}
	// Every lock holds, including the one whose exact entry just made room.
	for k := range l.names.own {
		if k == "newcomer" {
			continue
		}
		if _, wait := l.begin(netip.MustParseAddr("203.0.113.2"), k); wait == 0 {
			t.Fatalf("locked key %q admitted a try", k)
		}
	}
	evicted := ""
	for i := 1; evicted == "" && i <= 2*maxTracked; i++ {
		if _, ok := l.names.own["junk"+strconv.Itoa(i)]; !ok {
			if got := l.names.sketch.count("junk"+strconv.Itoa(i), now); got >= freeFailures {
				evicted = "junk" + strconv.Itoa(i)
			}
		}
	}
	if evicted == "" {
		t.Fatal("no evicted key still has its failures in the sketch")
	}
	if _, wait := l.begin(netip.MustParseAddr("203.0.113.3"), evicted); wait == 0 {
		t.Errorf("%q lost its lock with its exact entry", evicted)
	}
}

// TestCollisionsOnlyTightenTheLimit uses a one-cell-wide sketch so every username shares cells, the worst case for a real user.
func TestCollisionsOnlyTightenTheLimit(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newLimiter(func() time.Time { return now }, testutil.Logger(t))
	l.names = newTable(1)
	ips := addrs(100, 8)
	for i := range 4 {
		l.begin(ips[i], "alice")
	}
	l.begin(ips[4], "bob")
	// Bob's failure lands in alice's cells, so the floor says five and alice is locked early, never late.
	if got := l.names.sketch.count("alice", now); got != 5 {
		t.Errorf("alice's floor = %d, want 5", got)
	}
	if got := l.names.own["alice"].failures; got != 4 {
		t.Errorf("alice's exact count = %d, want 4", got)
	}
	if _, wait := l.begin(ips[5], "alice"); wait != firstLockout {
		t.Errorf("alice wait = %v, want %v", wait, firstLockout)
	}
	// Alice signing in takes back only her own four, so bob's failure is still counted.
	l.mu.Lock()
	l.names.forget("alice", now)
	l.mu.Unlock()
	if got := l.names.sketch.count("bob", now); got != 1 {
		t.Errorf("bob's count after alice's refund = %d, want 1", got)
	}
	if _, wait := l.begin(ips[6], "carol"); wait != 0 {
		t.Errorf("the shared lock outlived the failures that set it: %v", wait)
	}
}

func TestRefusalByOtherKeysFailuresIsLoggedOncePerMinute(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var buf bytes.Buffer
	l := newLimiter(func() time.Time { return now }, slog.New(slog.NewTextHandler(&buf, nil)))
	ips := addrs(100, 8)

	// A refusal the key earned itself is routine and not logged.
	for i := range freeFailures {
		l.begin(ips[i], "bob")
	}
	if _, wait := l.begin(ips[5], "bob"); wait == 0 {
		t.Fatal("bob should be locked")
	}
	if buf.Len() != 0 {
		t.Fatalf("an ordinary lock was logged: %s", buf.String())
	}

	// With every username sharing cells, alice is refused on bob's failures alone.
	l.names = newTable(1)
	for range 7 {
		recordFailure(l, ips[6], "bob")
	}
	if _, wait := l.begin(ips[7], "alice"); wait == 0 {
		t.Fatal("alice should be refused on the shared cells")
	}
	l.begin(ips[7], "carol")
	if got := strings.Count(buf.String(), "flood"); got != 1 {
		t.Fatalf("warnings after two refusals = %d, want 1: %s", got, buf.String())
	}
	if !strings.Contains(buf.String(), "table=username") {
		t.Errorf("the warning does not name the table: %s", buf.String())
	}
	now = now.Add(warnInterval + time.Second)
	l.begin(ips[7], "dave")
	if got := strings.Count(buf.String(), "flood"); got != 2 {
		t.Errorf("warnings after a minute = %d, want 2: %s", got, buf.String())
	}
}
