package db

import (
	"testing"
	"time"
)

func TestFormatTimeIsUTCWholeSeconds(t *testing.T) {
	zone := time.FixedZone("plus2", 2*3600)
	got := FormatTime(time.Date(2026, 10, 24, 14, 30, 5, 987_000_000, zone))
	if want := "2026-10-24T12:30:05Z"; got != want {
		t.Errorf("FormatTime = %q, want %q", got, want)
	}
}

func TestParseTime(t *testing.T) {
	for in, want := range map[string]string{
		"2026-10-24T12:30:05Z":      "2026-10-24T12:30:05Z",
		"2026-10-24T14:30:05+02:00": "2026-10-24T12:30:05Z",
	} {
		got, err := ParseTime(in)
		if err != nil || got.Location() != time.UTC || FormatTime(got) != want {
			t.Errorf("ParseTime(%q) = %v, %v; want %s in UTC", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "2026-10-24", "yesterday", "2026-10-24 12:30:05"} {
		if _, err := ParseTime(bad); err == nil {
			t.Errorf("ParseTime(%q) succeeded", bad)
		}
	}
}

func TestFlag(t *testing.T) {
	if Flag(true) != 1 || Flag(false) != 0 {
		t.Errorf("Flag(true) = %d, Flag(false) = %d", Flag(true), Flag(false))
	}
}
