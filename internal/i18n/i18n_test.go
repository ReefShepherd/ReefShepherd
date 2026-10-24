package i18n

import "testing"

func TestT(t *testing.T) {
	c := New(map[string]map[string]string{
		"en": {"greeting": "Hello", "due": "%s is due in %d days", "only_en": "English only"},
		"de": {"greeting": "Hallo"},
	})
	tests := []struct {
		name   string
		locale string
		key    string
		args   []any
		want   string
	}{
		{"exact locale", "de", "greeting", nil, "Hallo"},
		{"default locale", "en", "greeting", nil, "Hello"},
		{"unknown locale falls back", "fr", "greeting", nil, "Hello"},
		{"missing key in locale falls back", "de", "only_en", nil, "English only"},
		{"formatting", "en", "due", []any{"Water change", 3}, "Water change is due in 3 days"},
		{"missing key returns the key", "en", "nope", nil, "nope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.T(tt.locale, tt.key, tt.args...); got != tt.want {
				t.Errorf("T(%q, %q) = %q, want %q", tt.locale, tt.key, got, tt.want)
			}
		})
	}
}
