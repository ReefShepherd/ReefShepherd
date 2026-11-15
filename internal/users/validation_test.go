package users

import (
	"strings"
	"testing"
)

func TestCleanEmail(t *testing.T) {
	str := func(s string) *string { return &s }
	good := map[string]string{"a@b.co": "a@b.co", "  a@b.co ": "a@b.co", "first.last+tag@example.org": "first.last+tag@example.org"}
	for in, want := range good {
		got, err := cleanEmail(str(in))
		if err != nil || got == nil || *got != want {
			t.Errorf("cleanEmail(%q) = %v, %v", in, got, err)
		}
	}
	for _, blank := range []*string{nil, str(""), str("   ")} {
		if got, err := cleanEmail(blank); got != nil || err != nil {
			t.Errorf("blank email = %v, %v", got, err)
		}
	}
	for _, bad := range []string{"nope", "a@", "@b.co", "Jay <a@b.co>", "a@b.co, c@d.co", strings.Repeat("x", 250) + "@example.org"} {
		if got, err := cleanEmail(str(bad)); err == nil {
			t.Errorf("cleanEmail(%q) = %v, want an error", bad, *got)
		}
	}
}

func TestCleanDisplayName(t *testing.T) {
	if got, err := cleanDisplayName("  Jay  "); err != nil || got != "Jay" {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := cleanDisplayName(strings.Repeat("é", 100)); err != nil {
		t.Errorf("100 characters rejected: %v", err)
	}
	if _, err := cleanDisplayName(strings.Repeat("é", 101)); err == nil {
		t.Error("101 characters accepted")
	}
	if _, err := cleanDisplayName("bad\xff"); err == nil {
		t.Error("invalid text accepted")
	}
}

func TestUsernameAndLocalePatterns(t *testing.T) {
	for _, ok := range []string{"al", "jay.reef", "A_B-c@d", strings.Repeat("a", 64)} {
		if !usernamePattern.MatchString(ok) {
			t.Errorf("username %q rejected", ok)
		}
	}
	for _, bad := range []string{"", "a", "a b", "jürgen", "x/y", "a\nb", strings.Repeat("a", 65), "name\x00"} {
		if usernamePattern.MatchString(bad) {
			t.Errorf("username %q accepted", bad)
		}
	}
	for _, ok := range []string{"en", "de", "pt-BR", "zh-Hant"} {
		if !localePattern.MatchString(ok) {
			t.Errorf("locale %q rejected", ok)
		}
	}
	for _, bad := range []string{"", "English", "e", "en_US", "EN", "en-", "../etc"} {
		if localePattern.MatchString(bad) {
			t.Errorf("locale %q accepted", bad)
		}
	}
}
