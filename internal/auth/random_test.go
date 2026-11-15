package auth

import (
	"bytes"
	"strings"
	"testing"
)

func TestRandomStringSkipsBytesThatWouldFavourTheFirstCharacters(t *testing.T) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789" // 57 characters, so bytes 228 and up are skipped
	if len(alphabet) != 57 {
		t.Fatalf("alphabet has %d characters", len(alphabet))
	}
	// 255, 228 and 240 sit above the limit; 0, 1 and 227 are the first and last bytes that count.
	src := bytes.NewReader([]byte{255, 0, 228, 1, 240, 227})
	got, err := randomString(src, alphabet, 3)
	if err != nil {
		t.Fatal(err)
	}
	if want := string([]byte{alphabet[0], alphabet[1], alphabet[227%57]}); got != want {
		t.Errorf("randomString = %q, want %q", got, want)
	}
}

func TestRandomStringReadsMoreWhenBytesAreSkipped(t *testing.T) {
	const alphabet = "abc" // limit 255, so only 255 is skipped
	src := bytes.NewReader([]byte{255, 255, 255, 255, 0, 1, 2, 3})
	got, err := randomString(src, alphabet, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ab" {
		t.Errorf("randomString = %q, want ab", got)
	}
}

func TestRandomStringReportsAShortSource(t *testing.T) {
	if _, err := randomString(bytes.NewReader([]byte{1, 2}), "abc", 4); err == nil {
		t.Error("a source that ran dry was not reported")
	}
}

func TestRandomStringUsesOnlyTheAlphabetAtTheRequestedLength(t *testing.T) {
	for range 50 {
		got, err := RandomString(recoveryAbc, recoveryLen)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != recoveryLen || strings.Trim(got, recoveryAbc) != "" {
			t.Fatalf("RandomString = %q", got)
		}
	}
}
