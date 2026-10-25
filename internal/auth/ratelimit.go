package auth

import (
	"hash/maphash"
	"log/slog"
	"math"
	"net/netip"
	"sync"
	"time"
)

const (
	// freeFailures is how many failed attempts a key may make before it is locked out.
	freeFailures = 5
	firstLockout = 30 * time.Second
	maxLockout   = 15 * time.Minute
	// maxTracked caps the exact entries kept per table; the sketch holds the floor for every key regardless.
	maxTracked = 10_000
	// maxKeyLen bounds a single key, since usernames come straight from the request.
	maxKeyLen = 128
	// sketchRows and sketchWidth size the fixed memory that counts failures for every key, 2 MiB per table.
	sketchRows  = 2
	sketchWidth = 1 << 16
	// sweepInterval bounds how often a full exact map is scanned for stale entries.
	sweepInterval = 10 * time.Second
	// warnInterval bounds how often a refusal caused by other keys' failures is logged.
	warnInterval = time.Minute
)

// entry is the exact failure history of one username or client address.
type entry struct {
	failures    int
	lockedUntil time.Time
	last        time.Time
}

// cell is one counter of the sketch: the failures hashed into it and the latest lock they set, as unix nanoseconds.
type cell struct {
	count       int
	lockedUntil int64
}

// sketch is a count-min sketch of failures: a key's count is the minimum over its cells, which other keys can raise but never lower, so no flood can make it forget.
type sketch struct {
	seed  maphash.Seed
	width uint64
	cells []cell
}

func newSketch(width uint64) *sketch {
	return &sketch{seed: maphash.MakeSeed(), width: width, cells: make([]cell, sketchRows*width)}
}

// index returns the key's cell in each row; the seed is random per process so nobody can aim collisions at a victim.
func (s *sketch) index(key string) [sketchRows]int {
	h := maphash.String(s.seed, key)
	pos, step := h&math.MaxUint32, (h>>32)|1
	var idx [sketchRows]int
	for i := range idx {
		idx[i] = int(uint64(i)*s.width + pos%s.width) //nolint:gosec // below sketchRows*width, the length of cells
		pos += step
	}
	return idx
}

func (s *sketch) stale(c cell, now int64) bool {
	return now-c.lockedUntil > int64(maxLockout)
}

// add counts one failure against the key and extends the lock in each of its cells.
func (s *sketch) add(key string, now time.Time) {
	t := now.UnixNano()
	for _, i := range s.index(key) {
		c := &s.cells[i]
		if s.stale(*c, t) {
			*c = cell{}
		}
		c.count++
		c.lockedUntil = max(c.lockedUntil, now.Add(lockout(c.count)).UnixNano())
	}
}

// release takes n of the key's own failures back and lifts the locks they set, which can only remove what this key itself added.
func (s *sketch) release(key string, n int, now time.Time) {
	t := now.UnixNano()
	for _, i := range s.index(key) {
		c := &s.cells[i]
		if s.stale(*c, t) {
			*c = cell{}
			continue
		}
		c.count = max(c.count-n, 0)
		c.lockedUntil = min(c.lockedUntil, t)
	}
}

// count is the key's failures as far as the sketch knows, never fewer than it really made.
func (s *sketch) count(key string, now time.Time) int {
	t := now.UnixNano()
	n := math.MaxInt
	for _, i := range s.index(key) {
		c := s.cells[i]
		if s.stale(c, t) {
			return 0
		}
		n = min(n, c.count)
	}
	return n
}

// lockedUntil is when the key's lock ends as far as the sketch knows, the zero time when it has none.
func (s *sketch) lockedUntil(key string, now time.Time) time.Time {
	t := now.UnixNano()
	until := int64(math.MaxInt64)
	for _, i := range s.index(key) {
		c := s.cells[i]
		if s.stale(c, t) {
			return time.Time{}
		}
		until = min(until, c.lockedUntil)
	}
	return time.Unix(0, until)
}

// table is the failure history of one kind of key: a sketch nothing can evict from, plus capped exact entries so a success can refund exactly its own failures.
type table struct {
	own       map[string]*entry
	sketch    *sketch
	lastSweep time.Time
}

func newTable(width uint64) *table {
	return &table{own: map[string]*entry{}, sketch: newSketch(width)}
}

// stale reports whether an entry has been quiet long enough to be forgotten: a full lockout period has passed since it last locked.
func stale(e *entry, now time.Time) bool {
	return now.Sub(e.lockedUntil) > maxLockout
}

// lockedUntil is when the key's lock ends, the later of its exact entry's lock and the sketch floor.
func (t *table) lockedUntil(key string, now time.Time) time.Time {
	until := t.sketch.lockedUntil(key, now)
	if e, ok := t.own[key]; ok && !stale(e, now) && e.lockedUntil.After(until) {
		until = e.lockedUntil
	}
	return until
}

// ownLocked reports whether the key's own exact entry accounts for its lock.
func (t *table) ownLocked(key string, now time.Time) bool {
	e, ok := t.own[key]
	return ok && !stale(e, now) && e.lockedUntil.After(now)
}

// record counts one failure against the key in both parts.
func (t *table) record(key string, now time.Time) {
	e, ok := t.own[key]
	if ok && stale(e, now) {
		e, ok = nil, false
	}
	if !ok {
		t.makeRoom(now)
		e = &entry{}
		t.own[key] = e
	}
	e.failures++
	e.last = now
	e.lockedUntil = now.Add(lockout(e.failures))
	t.sketch.add(key, now)
}

// release takes one reserved try back off the key, so a try that turned out not to be a guess leaves no mark.
func (t *table) release(key string, now time.Time) {
	if e, ok := t.own[key]; ok {
		e.failures--
		if e.failures <= 0 {
			delete(t.own, key)
		} else {
			// The try was admitted, so the key was not locked before it; undoing it lifts the lock it set.
			e.lockedUntil = e.last
		}
	}
	t.sketch.release(key, 1, now)
}

// forget clears the key's history: every failure its exact entry knows of, or just this try when that entry was evicted.
func (t *table) forget(key string, now time.Time) {
	n := 1
	if e, ok := t.own[key]; ok {
		n = max(e.failures, 1)
		delete(t.own, key)
	}
	t.sketch.release(key, n, now)
}

// makeRoom keeps the exact map under its cap, dropping stale entries and then the oldest in a scan at most every sweepInterval and an arbitrary entry in between; losing an entry only costs the exactness of its refund.
func (t *table) makeRoom(now time.Time) {
	if len(t.own) < maxTracked {
		return
	}
	if now.Sub(t.lastSweep) < sweepInterval {
		for k := range t.own {
			delete(t.own, k)
			return
		}
	}
	t.lastSweep = now
	var oldestKey string
	var oldest time.Time
	for k, e := range t.own {
		if stale(e, now) {
			delete(t.own, k)
			continue
		}
		if oldestKey == "" || e.last.Before(oldest) {
			oldestKey, oldest = k, e.last
		}
	}
	if len(t.own) >= maxTracked {
		delete(t.own, oldestKey)
	}
}

// limiter slows down password guessing per username and per client address, in memory only.
type limiter struct {
	mu       sync.Mutex
	now      func() time.Time
	log      *slog.Logger
	ips      *table
	names    *table
	lastWarn time.Time
}

func newLimiter(now func() time.Time) *limiter {
	return &limiter{now: now, log: slog.Default(), ips: newTable(sketchWidth), names: newTable(sketchWidth)}
}

// lockout is how long a key stays locked after its nth failure, doubling from 30 seconds up to 15 minutes.
func lockout(failures int) time.Duration {
	if failures < freeFailures {
		return 0
	}
	d := firstLockout
	for i := freeFailures; i < failures && d < maxLockout; i++ {
		d *= 2
	}
	return min(d, maxLockout)
}

// ipKey groups IPv6 addresses by /64 so one host cannot dodge the limit by rotating addresses.
func ipKey(a netip.Addr) string {
	if a.Is6() {
		return netip.PrefixFrom(a, 64).Masked().Addr().String()
	}
	return a.String()
}

func clampKey(k string) string {
	if len(k) > maxKeyLen {
		return k[:maxKeyLen]
	}
	return k
}

// attempt is one reserved try, to be settled with succeed or partial once its outcome is known; a failure needs no call.
type attempt struct {
	ip, name string
}

// begin refuses the try while either key is locked, and otherwise counts it against both keys before any password or code work starts, so concurrent tries cannot all pass the check first.
func (l *limiter) begin(ip netip.Addr, name string) (a attempt, wait time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	a = attempt{ip: ipKey(ip), name: clampKey(name)}
	for _, c := range []struct {
		t     *table
		key   string
		label string
	}{{l.ips, a.ip, "address"}, {l.names, a.name, "username"}} {
		until := c.t.lockedUntil(c.key, now)
		if !until.After(now) {
			continue
		}
		wait = max(wait, until.Sub(now))
		if !c.t.ownLocked(c.key, now) {
			l.warnCollision(now, c.label)
		}
	}
	if wait > 0 {
		return attempt{}, wait
	}
	l.ips.record(a.ip, now)
	l.names.record(a.name, now)
	return a, 0
}

// warnCollision logs, at most once per warnInterval, that a key was refused on the strength of other keys' failures, which is what a flood looks like from inside.
func (l *limiter) warnCollision(now time.Time, table string) {
	if !l.lastWarn.IsZero() && now.Sub(l.lastWarn) < warnInterval {
		return
	}
	l.lastWarn = now
	l.log.Warn("rate limiter refused a key without a lock of its own, which points to a flood of failed sign-ins", slog.String("table", table))
}

// succeed settles a try that completed a sign-in: the username forgets its failures, and the address only gets this one try back, so one valid account cannot clear the address's history.
func (l *limiter) succeed(a attempt) {
	if a == (attempt{}) {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.ips.release(a.ip, now)
	l.names.forget(a.name, now)
}

// partial settles a try that proved the password but still owes a second step: it was no guess, and it leaves the username's failures where they were.
func (l *limiter) partial(a attempt) {
	if a == (attempt{}) {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.ips.release(a.ip, now)
	l.names.release(a.name, now)
}
