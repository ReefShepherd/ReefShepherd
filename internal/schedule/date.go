// Package schedule interprets the shared schedule columns and answers when an item is next due.
package schedule

import (
	"fmt"
	"time"
)

// Date is a calendar date with no time zone.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// TimeOfDay is a local wall-clock time.
type TimeOfDay struct {
	Hour, Minute int
}

// DefaultDueTime is the time of day used when a schedule does not name one.
func DefaultDueTime() TimeOfDay { return TimeOfDay{Hour: 9} }

// ParseDate parses YYYY-MM-DD.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return Date{}, fmt.Errorf("parse date %q: %w", s, ErrInvalid)
	}
	return Date{t.Year(), t.Month(), t.Day()}, nil
}

// DateOf returns the calendar date of t in loc.
func DateOf(t time.Time, loc *time.Location) Date {
	y, m, d := t.In(loc).Date()
	return Date{y, m, d}
}

// ParseTimeOfDay parses HH:MM on a 24-hour clock.
func ParseTimeOfDay(s string) (TimeOfDay, error) {
	t, err := time.Parse("15:04", s)
	if err != nil || len(s) != len("15:04") {
		return TimeOfDay{}, fmt.Errorf("parse time of day %q: %w", s, ErrInvalid)
	}
	return TimeOfDay{t.Hour(), t.Minute()}, nil
}

// String formats the time as HH:MM.
func (t TimeOfDay) String() string { return fmt.Sprintf("%02d:%02d", t.Hour, t.Minute) }

// String formats the date as YYYY-MM-DD.
func (d Date) String() string { return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day) }

// IsZero reports whether d is the unset zero value.
func (d Date) IsZero() bool { return d == Date{} }

// Valid reports whether d names a real calendar day.
func (d Date) Valid() bool { return d.utc().Format(time.DateOnly) == d.String() }

// Weekday returns the day of the week.
func (d Date) Weekday() time.Weekday { return d.utc().Weekday() }

// AddDays returns the date n calendar days later (earlier when n is negative).
func (d Date) AddDays(n int) Date { return DateOf(d.utc().AddDate(0, 0, n), time.UTC) }

// DaysSince returns the number of calendar days from o to d.
func (d Date) DaysSince(o Date) int { return d.dayNumber() - o.dayNumber() }

// dayNumber returns the days since 1970-01-01 in the proleptic Gregorian calendar, with no overflow at any year.
func (d Date) dayNumber() int {
	y, m := d.Year, int(d.Month)
	if m <= 2 {
		y--
	}
	era := floorDiv(y, 400)
	yoe := y - era*400
	mp := (m + 9) % 12
	doy := (153*mp+2)/5 + d.Day - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return era*146097 + doe - 719468
}

// Before reports whether d is earlier than o.
func (d Date) Before(o Date) bool { return d.DaysSince(o) < 0 }

func (d Date) firstOfNextMonth() Date {
	return DateOf(time.Date(d.Year, d.Month+1, 1, 0, 0, 0, 0, time.UTC), time.UTC)
}

func (d Date) utc() time.Time { return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC) }

// At returns the instant of wall-clock time tod on d in loc, shifting a skipped time forward by the gap and taking the first of a repeated time.
func (d Date) At(tod TimeOfDay, loc *time.Location) time.Time {
	wall := time.Date(d.Year, d.Month, d.Day, tod.Hour, tod.Minute, 0, 0, time.UTC)
	_, before := wall.Add(-24 * time.Hour).In(loc).Zone()
	_, after := wall.Add(24 * time.Hour).In(loc).Zone()
	first := wall.Add(-time.Duration(before) * time.Second)
	second := wall.Add(-time.Duration(after) * time.Second)
	switch {
	case showsWall(first, loc, wall) && showsWall(second, loc, wall):
		if second.Before(first) {
			return second
		}
		return first
	case showsWall(second, loc, wall):
		return second
	default:
		return first
	}
}

// showsWall reports whether instant t reads as the wall clock time given in wall (expressed in UTC fields).
func showsWall(t time.Time, loc *time.Location, wall time.Time) bool {
	l := t.In(loc)
	return l.Year() == wall.Year() && l.Month() == wall.Month() && l.Day() == wall.Day() &&
		l.Hour() == wall.Hour() && l.Minute() == wall.Minute()
}
