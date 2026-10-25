// Package password hashes and checks passwords with Argon2id and enforces the password length rules.
package password

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Length limits in characters.
const (
	MinLength = 12
	MaxLength = 256
)

const (
	saltLen = 16
	keyLen  = 32
	// maxConcurrent keeps the peak memory of simultaneous hashes near twice the memory cost.
	maxConcurrent = 2

	// Upper bounds for parameters read from a stored hash, so a corrupted value cannot make a login allocate gigabytes.
	maxStoredMemoryKiB = 1 << 21
	maxStoredTime      = 64
	maxStoredThreads   = 64
)

// Errors returned by Validate.
var (
	ErrTooShort    = fmt.Errorf("must be at least %d characters", MinLength)
	ErrTooLong     = fmt.Errorf("must be at most %d characters", MaxLength)
	ErrInvalidText = errors.New("must be valid text")
)

// ErrUnsupportedHash is returned when a stored hash is not an Argon2id PHC string this package can read.
var ErrUnsupportedHash = errors.New("unsupported password hash")

// Params are the Argon2id cost parameters.
type Params struct {
	Time      uint32
	MemoryKiB uint32
	Threads   uint8
}

// DefaultParams is the RFC 9106 memory-constrained profile: t=3, m=64 MiB, p=4.
func DefaultParams() Params {
	return Params{Time: 3, MemoryKiB: 64 * 1024, Threads: 4}
}

// Validate checks the length rules; there are no composition rules.
func Validate(plain string) error {
	if !utf8.ValidString(plain) {
		return ErrInvalidText
	}
	switch n := utf8.RuneCountInString(plain); {
	case n < MinLength:
		return ErrTooShort
	case n > MaxLength:
		return ErrTooLong
	}
	return nil
}

// Hasher computes and verifies hashes, running at most two at once.
type Hasher struct {
	params Params
	sem    chan struct{}

	dummyOnce sync.Once
	dummy     string
}

// NewHasher returns a hasher that creates hashes with params.
func NewHasher(params Params) *Hasher {
	return &Hasher{params: params, sem: make(chan struct{}, maxConcurrent)}
}

func (h *Hasher) acquire(ctx context.Context) error {
	select {
	case h.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for hashing slot: %w", ctx.Err())
	}
}

func (h *Hasher) release() { <-h.sem }

// Hash returns the PHC string of plain with a fresh random salt.
func (h *Hasher) Hash(ctx context.Context, plain string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	if err := h.acquire(ctx); err != nil {
		return "", err
	}
	defer h.release()
	p := h.params
	key := argon2.IDKey([]byte(plain), salt, p.Time, p.MemoryKiB, p.Threads, keyLen)
	return encode(p, salt, key), nil
}

// Verify reports whether plain matches the stored hash; needsRehash is true when the hash was made with other parameters than the current ones.
func (h *Hasher) Verify(ctx context.Context, plain, encoded string) (ok, needsRehash bool, err error) {
	p, salt, want, err := decode(encoded)
	if err != nil {
		return false, false, err
	}
	if err := h.acquire(ctx); err != nil {
		return false, false, err
	}
	defer h.release()
	got := argon2.IDKey([]byte(plain), salt, p.Time, p.MemoryKiB, p.Threads, uint32(len(want))) //nolint:gosec // decode bounds the key to 64 bytes
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return false, false, nil
	}
	return true, p != h.params || len(want) != keyLen, nil
}

// SpendVerifyTime does the work of a failed Verify, so a login for an unknown user takes as long as one with a wrong password.
func (h *Hasher) SpendVerifyTime(ctx context.Context, plain string) error {
	var err error
	h.dummyOnce.Do(func() {
		h.dummy, err = h.Hash(context.WithoutCancel(ctx), "reefshepherd-dummy-password")
	})
	if err != nil {
		return err
	}
	_, _, err = h.Verify(ctx, plain, h.dummy)
	return err
}

func encode(p Params, salt, key []byte) string {
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.MemoryKiB, p.Time, p.Threads, b64.EncodeToString(salt), b64.EncodeToString(key))
}

func decode(encoded string) (p Params, salt, key []byte, err error) {
	fail := func(why string) (Params, []byte, []byte, error) {
		return Params{}, nil, nil, fmt.Errorf("%w: %s", ErrUnsupportedHash, why)
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return fail("not an argon2id PHC string")
	}
	if parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return fail("unknown argon2 version")
	}
	var m, t, par uint64
	fields := strings.Split(parts[3], ",")
	if len(fields) != 3 {
		return fail("bad parameters")
	}
	for i, spec := range []struct {
		prefix string
		dst    *uint64
		bits   int
	}{{"m=", &m, 32}, {"t=", &t, 32}, {"p=", &par, 8}} {
		v, ok := strings.CutPrefix(fields[i], spec.prefix)
		if !ok {
			return fail("bad parameters")
		}
		n, perr := strconv.ParseUint(v, 10, spec.bits)
		if perr != nil {
			return fail("bad parameters")
		}
		*spec.dst = n
	}
	if t < 1 || t > maxStoredTime || par < 1 || par > maxStoredThreads || m < 8*par || m > maxStoredMemoryKiB {
		return fail("parameters out of range")
	}
	b64 := base64.RawStdEncoding
	salt, err = b64.DecodeString(parts[4])
	if err != nil || len(salt) < 8 {
		return fail("bad salt")
	}
	key, err = b64.DecodeString(parts[5])
	if err != nil || len(key) < 16 || len(key) > 64 {
		return fail("bad key")
	}
	return Params{Time: uint32(t), MemoryKiB: uint32(m), Threads: uint8(par)}, salt, key, nil
}
