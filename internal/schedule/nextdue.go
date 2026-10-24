package schedule

import (
	"time"
)

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func ceilDiv(a, b int) int { return -floorDiv(-a, b) }

func daysIn(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func weekdayBit(w time.Weekday) int { return Mon << ((int(w) + 6) % 7) }

// monthlyDate returns the occurrence in the given month, clamped to its last day.
func (s Schedule) monthlyDate(year int, month time.Month) Date {
	return Date{year, month, min(s.MonthDay, daysIn(year, month))}
}

// weekOn reports whether the week containing d is one of the weeks the schedule repeats in.
func (s Schedule) weekOn(d Date) bool {
	if s.IntervalWeeks <= 1 {
		return true
	}
	// 1970-01-05 is a Monday, which fixes week numbering when there is no anchor date.
	ref := Date{1970, time.January, 5}
	if !s.AnchorDate.IsZero() {
		ref = s.AnchorDate.AddDays(-((int(s.AnchorDate.Weekday()) + 6) % 7))
	}
	return floorDiv(d.DaysSince(ref), 7)%s.IntervalWeeks == 0
}

// onOrAfter returns the first fixed-grid date on or after d, with the grid extending backwards past the anchor date.
func (s Schedule) onOrAfter(d Date) Date {
	switch s.Kind {
	case EveryNDays:
		k := ceilDiv(d.DaysSince(s.AnchorDate), s.IntervalDays)
		return s.AnchorDate.AddDays(k * s.IntervalDays)
	case Weekly:
		for i := 0; ; i++ {
			c := d.AddDays(i)
			if s.Weekdays&weekdayBit(c.Weekday()) != 0 && s.weekOn(c) {
				return c
			}
		}
	default:
		c := s.monthlyDate(d.Year, d.Month)
		if c.Before(d) {
			next := d.firstOfNextMonth()
			c = s.monthlyDate(next.Year, next.Month)
		}
		return c
	}
}

// onOrBefore returns the last fixed-grid date on or before d.
func (s Schedule) onOrBefore(d Date) Date {
	switch s.Kind {
	case EveryNDays:
		k := floorDiv(d.DaysSince(s.AnchorDate), s.IntervalDays)
		return s.AnchorDate.AddDays(k * s.IntervalDays)
	case Weekly:
		for i := 0; ; i++ {
			c := d.AddDays(-i)
			if s.Weekdays&weekdayBit(c.Weekday()) != 0 && s.weekOn(c) {
				return c
			}
		}
	default:
		c := s.monthlyDate(d.Year, d.Month)
		if d.Before(c) {
			prev := Date{d.Year, d.Month, 1}.AddDays(-1)
			c = s.monthlyDate(prev.Year, prev.Month)
		}
		return c
	}
}

// starts returns the first date on which a fixed schedule can occur, or the zero Date when it has no start.
func (s Schedule) starts() Date {
	if s.Kind == EveryNDays || (s.Kind == Weekly && s.IntervalWeeks > 1) {
		return s.AnchorDate
	}
	return Date{}
}

// OccurrencesOn returns when a fixed schedule is due on a calendar day in loc, and nothing for the other kinds.
func OccurrencesOn(s Schedule, day Date, loc *time.Location) []time.Time {
	if loc == nil {
		loc = time.UTC
	}
	if !s.Fixed() || s.Validate() != nil {
		return nil
	}
	if start := s.starts(); !start.IsZero() && day.Before(start) {
		return nil
	}
	if s.onOrAfter(day) != day {
		return nil
	}
	return []time.Time{day.At(s.DueTime, loc)}
}

// NextDue returns when the item is next due, counting a completion on a fixed schedule for the occurrence nearest its local date (ties go to the earlier one) and reporting ok false when the schedule has no due time.
func NextDue(s Schedule, lastDone *time.Time, created time.Time, loc *time.Location) (due time.Time, ok bool) {
	if loc == nil {
		loc = time.UTC
	}
	if s.Validate() != nil {
		return time.Time{}, false
	}
	switch {
	case s.Kind == AfterLast:
		if lastDone == nil {
			return created.In(loc), true
		}
		return DateOf(*lastDone, loc).AddDays(s.IntervalDays).At(s.DueTime, loc), true
	case s.Fixed():
		if lastDone == nil {
			return s.firstAtOrAfter(created, loc), true
		}
		return s.afterCompletion(*lastDone, loc), true
	}
	return time.Time{}, false
}

func (s Schedule) firstAtOrAfter(t time.Time, loc *time.Location) time.Time {
	d := DateOf(t, loc)
	if start := s.starts(); !start.IsZero() && d.Before(start) {
		d = start
	}
	occ := s.onOrAfter(d)
	if at := occ.At(s.DueTime, loc); !at.Before(t) {
		return at
	}
	return s.onOrAfter(occ.AddDays(1)).At(s.DueTime, loc)
}

// afterCompletion finds the occurrence nearest the completion's local date (ties go to the earlier one) and returns the one after it, never before the schedule starts.
func (s Schedule) afterCompletion(last time.Time, loc *time.Location) time.Time {
	day := DateOf(last, loc)
	prev, next := s.onOrBefore(day), s.onOrAfter(day)
	satisfied := next
	if day.DaysSince(prev) <= next.DaysSince(day) {
		satisfied = prev
	}
	due := s.onOrAfter(satisfied.AddDays(1))
	if start := s.starts(); !start.IsZero() && due.Before(start) {
		due = s.onOrAfter(start)
	}
	return due.At(s.DueTime, loc)
}
