package users

import (
	"strings"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

// codeOf runs check against a fresh collector and returns the single code it recorded for field, "" when none.
func codeOf(t *testing.T, field string, check func(f *httpx.FieldErrors)) string {
	t.Helper()
	f := httpx.FieldErrors{}
	check(&f)
	got := httpx.Validation(f).Fields
	if len(got) > 1 || (len(got) == 1 && got[field] == "") {
		t.Fatalf("fields = %v, want at most %s", got, field)
	}
	return got[field]
}

func TestCleanEmail(t *testing.T) {
	str := func(s string) *string { return &s }
	good := map[string]string{"a@b.co": "a@b.co", "  a@b.co ": "a@b.co", "first.last+tag@example.org": "first.last+tag@example.org"}
	for in, want := range good {
		var got *string
		if code := codeOf(t, "email", func(f *httpx.FieldErrors) { got = cleanEmail(str(in), f) }); code != "" || got == nil || *got != want {
			t.Errorf("cleanEmail(%q) = %v, code %q", in, got, code)
		}
	}
	for _, blank := range []*string{nil, str(""), str("   ")} {
		var got *string
		if code := codeOf(t, "email", func(f *httpx.FieldErrors) { got = cleanEmail(blank, f) }); got != nil || code != "" {
			t.Errorf("blank email = %v, code %q", got, code)
		}
	}
	for _, bad := range []string{"nope", "a@", "@b.co", "Jay <a@b.co>", "a@b.co, c@d.co", strings.Repeat("x", 250) + "@example.org"} {
		var got *string
		if code := codeOf(t, "email", func(f *httpx.FieldErrors) { got = cleanEmail(str(bad), f) }); code != "invalid" || got != nil {
			t.Errorf("cleanEmail(%q) = %v, code %q, want invalid", bad, got, code)
		}
	}
}

func TestCleanDisplayName(t *testing.T) {
	clean := func(s string) (string, string) {
		var got string
		code := codeOf(t, "display_name", func(f *httpx.FieldErrors) { got = cleanDisplayName(s, f) })
		return got, code
	}
	if got, code := clean("  Jay  "); code != "" || got != "Jay" {
		t.Errorf("got %q, code %q", got, code)
	}
	if _, code := clean(strings.Repeat("é", 100)); code != "" {
		t.Errorf("100 characters rejected: %q", code)
	}
	if _, code := clean(strings.Repeat("é", 101)); code != "too_long" {
		t.Errorf("101 characters: code %q, want too_long", code)
	}
	if _, code := clean("bad\xff"); code != "invalid" {
		t.Errorf("invalid text: code %q, want invalid", code)
	}
}

func TestValidateNewAccountCodes(t *testing.T) {
	cases := []struct {
		name string
		in   CreateInput
		want map[string]string
	}{
		{"fine", CreateInput{Username: "jay", Role: httpx.RoleMember}, map[string]string{}},
		{"empty username", CreateInput{Role: httpx.RoleMember}, map[string]string{"username": "too_short"}},
		{"one letter", CreateInput{Username: "a", Role: httpx.RoleMember}, map[string]string{"username": "too_short"}},
		{"65 letters", CreateInput{Username: strings.Repeat("a", 65), Role: httpx.RoleMember}, map[string]string{"username": "too_long"}},
		{"bad characters", CreateInput{Username: "a b", Role: httpx.RoleMember}, map[string]string{"username": "invalid"}},
		{"bad role", CreateInput{Username: "jay", Role: "owner"}, map[string]string{"role": "invalid"}},
	}
	for _, c := range cases {
		f := httpx.FieldErrors{}
		validateNewAccount(&c.in, &f)
		got := httpx.Validation(f).Fields
		if len(got) != len(c.want) {
			t.Errorf("%s: fields = %v, want %v", c.name, got, c.want)
			continue
		}
		for field, code := range c.want {
			if got[field] != code {
				t.Errorf("%s: fields = %v, want %v", c.name, got, c.want)
			}
		}
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
