package schedule

import (
	"errors"
	"testing"
	"time"
)

func TestParseDate(t *testing.T) {
	tests := []struct {
		in   string
		want Date
		ok   bool
	}{
		{"2026-10-24", Date{2026, time.October, 24}, true},
		{"2028-02-29", Date{2028, time.February, 29}, true},
		{"2027-02-29", Date{}, false},
		{"2026-13-01", Date{}, false},
		{"2026-04-31", Date{}, false},
		{"2026-4-1", Date{}, false},
		{"", Date{}, false},
		{"yesterday", Date{}, false},
	}
	for _, tc := range tests {
		got, err := ParseDate(tc.in)
		if (err == nil) != tc.ok {
			t.Errorf("ParseDate(%q) err = %v, want ok=%v", tc.in, err, tc.ok)
			continue
		}
		if !tc.ok && !errors.Is(err, ErrInvalid) {
			t.Errorf("ParseDate(%q) err = %v, want ErrInvalid", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("ParseDate(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseTimeOfDay(t *testing.T) {
	tests := []struct {
		in   string
		want TimeOfDay
		ok   bool
	}{
		{"09:00", tod(9, 0), true}, {"00:00", tod(0, 0), true}, {"23:59", tod(23, 59), true},
		{"24:00", TimeOfDay{}, false}, {"12:60", TimeOfDay{}, false}, {"9:00", TimeOfDay{}, false},
		{"09:00:00", TimeOfDay{}, false}, {"", TimeOfDay{}, false}, {"noon", TimeOfDay{}, false},
	}
	for _, tc := range tests {
		got, err := ParseTimeOfDay(tc.in)
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("ParseTimeOfDay(%q) = %v, %v; want %v ok=%v", tc.in, got, err, tc.want, tc.ok)
		}
		if tc.ok && got.String() != tc.in {
			t.Errorf("String() = %q, want %q", got.String(), tc.in)
		}
	}
}

func TestDateArithmetic(t *testing.T) {
	tests := []struct {
		from string
		n    int
		want string
	}{
		{"2026-01-31", 1, "2026-02-01"}, {"2026-02-28", 1, "2026-03-01"}, {"2028-02-28", 1, "2028-02-29"},
		{"2028-02-29", 1, "2028-03-01"}, {"2026-12-31", 1, "2027-01-01"}, {"2027-01-01", -1, "2026-12-31"},
		{"2026-03-01", -1, "2026-02-28"}, {"2028-03-01", -1, "2028-02-29"}, {"2026-10-24", 0, "2026-10-24"},
		{"2026-01-01", 365, "2027-01-01"}, {"2028-01-01", 366, "2029-01-01"}, {"2026-10-24", -400, "2025-09-19"},
	}
	for _, tc := range tests {
		from := date(t, tc.from)
		got := from.AddDays(tc.n)
		if got.String() != tc.want {
			t.Errorf("%s + %d = %s, want %s", tc.from, tc.n, got, tc.want)
		}
		if back := got.DaysSince(from); back != tc.n {
			t.Errorf("%s.DaysSince(%s) = %d, want %d", got, tc.from, back, tc.n)
		}
		if !got.Valid() {
			t.Errorf("%s not valid", got)
		}
	}
	if !date(t, "2026-01-01").Before(date(t, "2026-01-02")) || date(t, "2026-01-02").Before(date(t, "2026-01-02")) {
		t.Error("Before is wrong")
	}
	if (Date{2026, 2, 30}).Valid() || (Date{2026, 13, 1}).Valid() {
		t.Error("Valid accepted an impossible date")
	}
	if !(Date{}).IsZero() || date(t, "2026-01-01").IsZero() {
		t.Error("IsZero is wrong")
	}
}

func TestWeekday(t *testing.T) {
	// 2026-10-24 is a Saturday.
	d := date(t, "2026-10-24")
	for i, want := range []time.Weekday{time.Saturday, time.Sunday, time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday} {
		if got := d.AddDays(i).Weekday(); got != want {
			t.Errorf("+%d: %v, want %v", i, got, want)
		}
	}
	bits := map[time.Weekday]int{
		time.Monday: Mon, time.Tuesday: Tue, time.Wednesday: Wed, time.Thursday: Thu,
		time.Friday: Fri, time.Saturday: Sat, time.Sunday: Sun,
	}
	want := map[time.Weekday]int{
		time.Monday: 1, time.Tuesday: 2, time.Wednesday: 4, time.Thursday: 8,
		time.Friday: 16, time.Saturday: 32, time.Sunday: 64,
	}
	for w, b := range bits {
		if b != want[w] || weekdayBit(w) != want[w] {
			t.Errorf("%v: const %d, weekdayBit %d, want %d", w, b, weekdayBit(w), want[w])
		}
	}
	if EveryDay != 127 {
		t.Errorf("EveryDay = %d", EveryDay)
	}
}

// TestAtAcrossClockChanges pins how wall-clock times that a DST change skips or repeats resolve.
func TestAtAcrossClockChanges(t *testing.T) {
	ny, lon, syd := loadLoc(t, "America/New_York"), loadLoc(t, "Europe/London"), loadLoc(t, "Australia/Sydney")
	tests := []struct {
		name string
		loc  *time.Location
		day  string
		at   TimeOfDay
		want string
	}{
		{"ny normal winter", ny, "2026-01-15", tod(9, 0), "2026-01-15T09:00:00-05:00"},
		{"ny normal summer", ny, "2026-07-15", tod(9, 0), "2026-07-15T09:00:00-04:00"},
		{"ny gap shifts forward", ny, "2026-03-08", tod(2, 30), "2026-03-08T03:30:00-04:00"},
		{"ny gap start edge", ny, "2026-03-08", tod(2, 0), "2026-03-08T03:00:00-04:00"},
		{"ny just before gap", ny, "2026-03-08", tod(1, 59), "2026-03-08T01:59:00-05:00"},
		{"ny just after gap", ny, "2026-03-08", tod(3, 0), "2026-03-08T03:00:00-04:00"},
		{"ny overlap first", ny, "2026-11-01", tod(1, 30), "2026-11-01T01:30:00-04:00"},
		{"ny after overlap", ny, "2026-11-01", tod(2, 0), "2026-11-01T02:00:00-05:00"},
		{"london gap", lon, "2026-03-29", tod(1, 30), "2026-03-29T02:30:00+01:00"},
		{"london overlap first", lon, "2026-10-25", tod(1, 30), "2026-10-25T01:30:00+01:00"},
		{"london noon after change", lon, "2026-10-25", tod(12, 0), "2026-10-25T12:00:00+00:00"},
		{"sydney gap", syd, "2026-10-04", tod(2, 30), "2026-10-04T03:30:00+11:00"},
		{"sydney overlap first", syd, "2027-04-04", tod(2, 30), "2027-04-04T02:30:00+11:00"},
		{"sydney after overlap", syd, "2027-04-04", tod(3, 0), "2027-04-04T03:00:00+10:00"},
		{"utc", time.UTC, "2026-03-08", tod(2, 30), "2026-03-08T02:30:00+00:00"},
		{"midnight", ny, "2026-06-01", tod(0, 0), "2026-06-01T00:00:00-04:00"},
	}
	for _, tc := range tests {
		got := date(t, tc.day).At(tc.at, tc.loc)
		if want := ts(t, tc.want); !got.Equal(want) {
			t.Errorf("%s: At = %s, want %s", tc.name, got.Format(time.RFC3339), tc.want)
		}
	}
}

func TestDaysSinceBeyondDurationRange(t *testing.T) {
	// time.Duration saturates near 292 years, so these spans must not go through it.
	tests := []struct {
		from, to string
		want     int
	}{
		{"1700-01-01", "2026-10-24", 119365},
		{"2026-10-24", "1700-01-01", -119365},
		{"0001-01-01", "9999-12-31", 3652058},
		{"1900-01-01", "2200-12-31", 109937},
		{"2000-02-29", "2000-02-29", 0},
	}
	for _, tc := range tests {
		if got := date(t, tc.to).DaysSince(date(t, tc.from)); got != tc.want {
			t.Errorf("%s.DaysSince(%s) = %d, want %d", tc.to, tc.from, got, tc.want)
		}
	}
}

func TestDaysSinceMatchesCalendar(t *testing.T) {
	d := Date{1895, time.January, 1}
	for i := 0; i < 400*366; i++ {
		if got := d.DaysSince(Date{1895, time.January, 1}); got != i {
			t.Fatalf("%s.DaysSince(1895-01-01) = %d, want %d", d, got, i)
		}
		d = d.AddDays(1)
	}
}
