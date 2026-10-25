package schedule

import (
	"testing"
	"time"
)

func loadLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return loc
}

// ts parses an RFC 3339 instant.
func ts(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

func date(t *testing.T, s string) Date {
	t.Helper()
	d, err := ParseDate(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func tod(h, m int) TimeOfDay { return TimeOfDay{h, m} }

func afterLast(days int, at TimeOfDay) Schedule {
	return Schedule{Kind: AfterLast, IntervalDays: days, IntervalWeeks: 1, DueTime: at}
}

func everyNDays(t *testing.T, days int, anchor string, at TimeOfDay) Schedule {
	return Schedule{Kind: EveryNDays, IntervalDays: days, IntervalWeeks: 1, AnchorDate: date(t, anchor), DueTime: at}
}

func weekly(t *testing.T, mask, weeks int, anchor string, at TimeOfDay) Schedule {
	s := Schedule{Kind: Weekly, Weekdays: mask, IntervalWeeks: weeks, DueTime: at}
	if anchor != "" {
		s.AnchorDate = date(t, anchor)
	}
	return s
}

func monthly(day int, at TimeOfDay) Schedule {
	return Schedule{Kind: Monthly, MonthDay: day, IntervalWeeks: 1, DueTime: at}
}
