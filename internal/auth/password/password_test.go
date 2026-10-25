package password

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

// testParams keeps the tests fast; the production profile is checked separately.
func testParams() Params { return Params{Time: 1, MemoryKiB: 64, Threads: 1} }

func TestHashAndVerify(t *testing.T) {
	h := NewHasher(testParams())
	enc, err := h.Hash(t.Context(), "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enc, "$argon2id$v=19$m=64,t=1,p=1$") {
		t.Errorf("encoded = %q", enc)
	}
	ok, rehash, err := h.Verify(t.Context(), "correct horse battery", enc)
	if err != nil || !ok || rehash {
		t.Errorf("right password: ok=%v rehash=%v err=%v", ok, rehash, err)
	}
	ok, _, err = h.Verify(t.Context(), "correct horse batterY", enc)
	if err != nil || ok {
		t.Errorf("wrong password: ok=%v err=%v", ok, err)
	}
}

func TestHashUsesFreshSalt(t *testing.T) {
	h := NewHasher(testParams())
	a, _ := h.Hash(t.Context(), "same password here")
	b, _ := h.Hash(t.Context(), "same password here")
	if a == b {
		t.Error("two hashes of the same password are identical")
	}
}

func TestDefaultParamsAreTheMemoryConstrainedProfile(t *testing.T) {
	if got, want := DefaultParams(), (Params{Time: 3, MemoryKiB: 65536, Threads: 4}); got != want {
		t.Fatalf("DefaultParams = %+v, want %+v", got, want)
	}
	h := NewHasher(DefaultParams())
	enc, err := h.Hash(t.Context(), "production parameters")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(enc, "$m=65536,t=3,p=4$") {
		t.Errorf("encoded = %q", enc)
	}
}

func TestVerifyFlagsOldParametersForRehash(t *testing.T) {
	old := NewHasher(Params{Time: 1, MemoryKiB: 32, Threads: 1})
	enc, _ := old.Hash(t.Context(), "an old password!!")
	current := NewHasher(testParams())
	ok, rehash, err := current.Verify(t.Context(), "an old password!!", enc)
	if err != nil || !ok || !rehash {
		t.Errorf("ok=%v rehash=%v err=%v; want a valid password flagged for rehash", ok, rehash, err)
	}
}

func TestVerifyRejectsMalformedHashes(t *testing.T) {
	h := NewHasher(testParams())
	good, _ := h.Hash(t.Context(), "some password 123")
	parts := strings.Split(good, "$")
	with := func(i int, v string) string {
		p := append([]string(nil), parts...)
		p[i] = v
		return strings.Join(p, "$")
	}
	bad := map[string]string{
		"empty":            "",
		"bcrypt":           "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ012345",
		"argon2i":          with(1, "argon2i"),
		"old version":      with(2, "v=16"),
		"missing part":     strings.Join(parts[:5], "$"),
		"extra part":       good + "$x",
		"param order":      with(3, "t=1,m=64,p=1"),
		"non-numeric":      with(3, "m=x,t=1,p=1"),
		"zero time":        with(3, "m=64,t=0,p=1"),
		"zero threads":     with(3, "m=64,t=1,p=0"),
		"memory too low":   with(3, "m=4,t=1,p=1"),
		"memory huge":      with(3, "m=4294967295,t=1,p=1"),
		"time huge":        with(3, "m=64,t=4000000,p=1"),
		"threads overflow": with(3, "m=64,t=1,p=300"),
		"bad salt":         with(4, "!!!"),
		"short salt":       with(4, "AAAA"),
		"bad key":          with(5, "!!!"),
		"short key":        with(5, "AAAA"),
	}
	for name, enc := range bad {
		t.Run(name, func(t *testing.T) {
			ok, _, err := h.Verify(t.Context(), "some password 123", enc)
			if ok || !errors.Is(err, ErrUnsupportedHash) {
				t.Errorf("ok=%v err=%v; want ErrUnsupportedHash", ok, err)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want error
	}{
		{"eleven characters", strings.Repeat("a", 11), ErrTooShort},
		{"twelve characters", strings.Repeat("a", 12), nil},
		{"256 characters", strings.Repeat("a", 256), nil},
		{"257 characters", strings.Repeat("a", 257), ErrTooLong},
		{"characters not bytes, short", strings.Repeat("é", 11), ErrTooShort},
		{"characters not bytes, long enough", strings.Repeat("é", 12), nil},
		{"characters not bytes, upper bound", strings.Repeat("é", 256), nil},
		{"only spaces still counts", strings.Repeat(" ", 12), nil},
		{"invalid utf-8", strings.Repeat("a", 12) + "\xff", ErrInvalidText},
		{"empty", "", ErrTooShort},
	}
	for _, tc := range tests {
		if got := Validate(tc.in); !errors.Is(got, tc.want) {
			t.Errorf("%s: Validate = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestAtMostTwoHashesRunAtOnce(t *testing.T) {
	h := NewHasher(testParams())
	// Occupy both slots as two running hashes would.
	h.sem <- struct{}{}
	h.sem <- struct{}{}

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := h.Hash(ctx, "a third hash waits"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Hash with both slots busy = %v, want a deadline error", err)
	}
	if _, _, err := h.Verify(ctx, "x", "$argon2id$v=19$m=64,t=1,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Verify with both slots busy = %v, want a deadline error", err)
	}

	<-h.sem
	if _, err := h.Hash(t.Context(), "a slot is free again"); err != nil {
		t.Errorf("Hash after a slot was freed: %v", err)
	}
}

func TestSpendVerifyTimeRunsAHash(t *testing.T) {
	h := NewHasher(testParams())
	if err := h.SpendVerifyTime(t.Context(), "anything"); err != nil {
		t.Fatal(err)
	}
	// With both slots busy it must wait like a real verification, not return early.
	h.sem <- struct{}{}
	h.sem <- struct{}{}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if err := h.SpendVerifyTime(ctx, "anything"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("SpendVerifyTime with busy slots = %v, want a deadline error", err)
	}
}

func TestHashReturnsItsWorkingMemoryToTheSystem(t *testing.T) {
	const memoryKiB = 32 * 1024
	h := NewHasher(Params{Time: 1, MemoryKiB: memoryKiB, Threads: 1})
	if _, err := h.Hash(t.Context(), "a password that is long enough"); err != nil {
		t.Fatal(err)
	}
	var m runtime.MemStats
	// A collection first, so the block counts as free heap whether or not the hasher returned it.
	runtime.GC()
	runtime.ReadMemStats(&m)
	// Free heap pages the runtime still holds resident; the 32 MiB block must not be among them.
	if retained := (m.HeapIdle - m.HeapReleased) / 1024; retained > memoryKiB/2 {
		t.Errorf("%d KiB of free heap is still resident after hashing", retained)
	}
}
