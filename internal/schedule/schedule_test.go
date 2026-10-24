package schedule

import (
	"errors"
	"reflect"
	"testing"
)

func ip(v int) *int       { return &v }
func sp(v string) *string { return &v }

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		in   Columns
		want Schedule
		err  bool
	}{
		{"none defaults", Columns{Kind: "none", IntervalWeeks: 1}, Schedule{Kind: None, IntervalWeeks: 1, DueTime: tod(9, 0)}, false},
		{"none ignores stray fields", Columns{Kind: "none", IntervalDays: ip(3), Weekdays: ip(5), IntervalWeeks: 4, MonthDay: ip(2), AnchorDate: sp("2026-01-01"), DueTime: "07:15"},
			Schedule{Kind: None, IntervalWeeks: 1, DueTime: tod(7, 15)}, false},
		{"after_last", Columns{Kind: "after_last", IntervalDays: ip(7), IntervalWeeks: 1, DueTime: "18:30"},
			Schedule{Kind: AfterLast, IntervalDays: 7, IntervalWeeks: 1, DueTime: tod(18, 30)}, false},
		{"after_last drops anchor", Columns{Kind: "after_last", IntervalDays: ip(7), AnchorDate: sp("2026-01-01")},
			Schedule{Kind: AfterLast, IntervalDays: 7, IntervalWeeks: 1, DueTime: tod(9, 0)}, false},
		{"every_n_days", Columns{Kind: "every_n_days", IntervalDays: ip(3), AnchorDate: sp("2026-11-01"), IntervalWeeks: 1},
			Schedule{Kind: EveryNDays, IntervalDays: 3, IntervalWeeks: 1, AnchorDate: Date{2026, 11, 1}, DueTime: tod(9, 0)}, false},
		{"weekly single week drops anchor", Columns{Kind: "weekly", Weekdays: ip(64), IntervalWeeks: 1, AnchorDate: sp("2026-11-01")},
			Schedule{Kind: Weekly, Weekdays: 64, IntervalWeeks: 1, DueTime: tod(9, 0)}, false},
		{"weekly multi week keeps anchor", Columns{Kind: "weekly", Weekdays: ip(32), IntervalWeeks: 2, AnchorDate: sp("2026-10-24")},
			Schedule{Kind: Weekly, Weekdays: 32, IntervalWeeks: 2, AnchorDate: Date{2026, 10, 24}, DueTime: tod(9, 0)}, false},
		{"weekly zero interval_weeks means 1", Columns{Kind: "weekly", Weekdays: ip(127)},
			Schedule{Kind: Weekly, Weekdays: 127, IntervalWeeks: 1, DueTime: tod(9, 0)}, false},
		{"monthly", Columns{Kind: "monthly", MonthDay: ip(31), IntervalWeeks: 1},
			Schedule{Kind: Monthly, MonthDay: 31, IntervalWeeks: 1, DueTime: tod(9, 0)}, false},
		{"unknown kind", Columns{Kind: "yearly"}, Schedule{}, true},
		{"empty kind", Columns{}, Schedule{}, true},
		{"after_last without interval", Columns{Kind: "after_last"}, Schedule{}, true},
		{"after_last zero interval", Columns{Kind: "after_last", IntervalDays: ip(0)}, Schedule{}, true},
		{"after_last negative interval", Columns{Kind: "after_last", IntervalDays: ip(-2)}, Schedule{}, true},
		{"after_last huge interval", Columns{Kind: "after_last", IntervalDays: ip(1 << 40)}, Schedule{}, true},
		{"every_n_days without anchor", Columns{Kind: "every_n_days", IntervalDays: ip(2)}, Schedule{}, true},
		{"every_n_days empty anchor", Columns{Kind: "every_n_days", IntervalDays: ip(2), AnchorDate: sp("")}, Schedule{}, true},
		{"every_n_days bad anchor", Columns{Kind: "every_n_days", IntervalDays: ip(2), AnchorDate: sp("2026-02-30")}, Schedule{}, true},
		{"every_n_days no interval", Columns{Kind: "every_n_days", AnchorDate: sp("2026-01-01")}, Schedule{}, true},
		{"weekly no weekdays", Columns{Kind: "weekly", IntervalWeeks: 1}, Schedule{}, true},
		{"weekly mask too big", Columns{Kind: "weekly", Weekdays: ip(128)}, Schedule{}, true},
		{"weekly multi week without anchor", Columns{Kind: "weekly", Weekdays: ip(1), IntervalWeeks: 2}, Schedule{}, true},
		{"weekly negative weeks", Columns{Kind: "weekly", Weekdays: ip(1), IntervalWeeks: -1}, Schedule{}, true},
		{"monthly day 0", Columns{Kind: "monthly", MonthDay: ip(0)}, Schedule{}, true},
		{"monthly day 32", Columns{Kind: "monthly", MonthDay: ip(32)}, Schedule{}, true},
		{"monthly missing day", Columns{Kind: "monthly"}, Schedule{}, true},
		{"bad due time", Columns{Kind: "none", DueTime: "25:00"}, Schedule{}, true},
		{"bad due time format", Columns{Kind: "none", DueTime: "9am"}, Schedule{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.in)
			if tc.err {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("err = %v, want ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got  %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func TestColumnsRoundTrip(t *testing.T) {
	in := []Columns{
		{Kind: "none", IntervalWeeks: 1, DueTime: "09:00"},
		{Kind: "after_last", IntervalDays: ip(7), IntervalWeeks: 1, DueTime: "08:05"},
		{Kind: "every_n_days", IntervalDays: ip(3), IntervalWeeks: 1, AnchorDate: sp("2026-11-01"), DueTime: "21:00"},
		{Kind: "weekly", Weekdays: ip(21), IntervalWeeks: 1, DueTime: "07:00"},
		{Kind: "weekly", Weekdays: ip(32), IntervalWeeks: 3, AnchorDate: sp("2026-10-24"), DueTime: "09:00"},
		{Kind: "monthly", MonthDay: ip(31), IntervalWeeks: 1, DueTime: "23:59"},
	}
	for _, c := range in {
		s, err := Parse(c)
		if err != nil {
			t.Fatalf("%+v: %v", c, err)
		}
		if got := s.Columns(); !reflect.DeepEqual(got, c) {
			t.Errorf("Columns() = %+v\nwant       %+v", got, c)
		}
	}
	// A hand-built schedule with zero interval_weeks still stores the column default.
	if got := (Schedule{Kind: Monthly, MonthDay: 1}).Columns().IntervalWeeks; got != 1 {
		t.Errorf("IntervalWeeks = %d, want 1", got)
	}
}

func TestValidateDirect(t *testing.T) {
	if err := (Schedule{}).Validate(); !errors.Is(err, ErrInvalid) {
		t.Errorf("zero Schedule: %v, want ErrInvalid", err)
	}
	if err := (Schedule{Kind: None, DueTime: tod(24, 0)}).Validate(); !errors.Is(err, ErrInvalid) {
		t.Errorf("hour 24: %v", err)
	}
	if err := (Schedule{Kind: None, DueTime: tod(-1, 0)}).Validate(); !errors.Is(err, ErrInvalid) {
		t.Errorf("hour -1: %v", err)
	}
	if err := (Schedule{Kind: None, AnchorDate: Date{2026, 2, 30}}).Validate(); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad anchor: %v", err)
	}
	for _, y := range []int{1700, 1899, 2201, 9999} {
		s := Schedule{Kind: EveryNDays, IntervalDays: 7, AnchorDate: Date{y, 1, 1}}
		if err := s.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("anchor year %d: %v, want ErrInvalid", y, err)
		}
	}
	for _, y := range []int{1900, 2026, 2200} {
		s := Schedule{Kind: EveryNDays, IntervalDays: 7, AnchorDate: Date{y, 12, 31}}
		if err := s.Validate(); err != nil {
			t.Errorf("anchor year %d: %v", y, err)
		}
	}
	for _, s := range []Schedule{
		{Kind: None},
		{Kind: AfterLast, IntervalDays: 1},
		{Kind: Weekly, Weekdays: EveryDay, IntervalWeeks: 520, AnchorDate: Date{2026, 1, 1}},
		{Kind: Monthly, MonthDay: 1},
	} {
		if err := s.Validate(); err != nil {
			t.Errorf("%+v: %v", s, err)
		}
	}
	if !(Schedule{Kind: Weekly}).Fixed() || (Schedule{Kind: AfterLast}).Fixed() || (Schedule{Kind: None}).Fixed() {
		t.Error("Fixed is wrong")
	}
}
