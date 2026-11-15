package auth

import (
	"crypto/rand"
	"fmt"
	"io"
)

// RandomString returns n characters drawn uniformly from alphabet, which must hold 1 to 256 bytes.
func RandomString(alphabet string, n int) (string, error) {
	return randomString(rand.Reader, alphabet, n)
}

// randomString skips random bytes beyond the largest multiple of the alphabet size, since 256 is not a multiple of it and a plain modulo would favour the first characters.
func randomString(src io.Reader, alphabet string, n int) (string, error) {
	limit := 256 - 256%len(alphabet)
	out := make([]byte, 0, n)
	buf := make([]byte, 2*n)
	for len(out) < n {
		if _, err := io.ReadFull(src, buf); err != nil {
			return "", fmt.Errorf("read random bytes: %w", err)
		}
		for _, b := range buf {
			if int(b) < limit && len(out) < n {
				out = append(out, alphabet[int(b)%len(alphabet)])
			}
		}
	}
	return string(out), nil
}
