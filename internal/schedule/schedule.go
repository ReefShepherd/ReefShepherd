package schedule

import (
	"errors"
	"fmt"
)

// ErrInvalid is wrapped by every validation failure.
var ErrInvalid = errors.New("invalid schedule")

// Kind selects how a schedule repeats.
type Kind string

// Schedule kinds, as stored in schedule_kind.
const (
	None       Kind = "none"
	AfterLast  Kind = "after_last"
	EveryNDays Kind = "every_n_days"
	Weekly     Kind = "weekly"
	Monthly    Kind = "monthly"
)

// Weekday bits for the weekdays bitmask.
const (
	Mon      = 1 << iota // 1
	Tue                  // 2
	Wed                  // 4
	Thu                  // 8
	Fri                  // 16
	Sat                  // 32
	Sun                  // 64
	EveryDay = Mon | Tue | Wed | Thu | Fri | Sat | Sun
)

// Upper bounds that keep the occurrence searches short.
const (
	maxIntervalDays  = 36500
	maxIntervalWeeks = 520
)

// Anchor dates outside these years are treated as typos.
const (
	minAnchorYear = 1900
	maxAnchorYear = 2200
)

// Schedule is the validated form of the shared schedule columns, with the fields a kind does not use left zero.
type Schedule struct {
	Kind          Kind
	IntervalDays  int
	Weekdays      int
	IntervalWeeks int
	MonthDay      int
	AnchorDate    Date
	DueTime       TimeOfDay
}

// Columns mirrors the stored schedule columns, with nil for SQL NULL.
type Columns struct {
	Kind          string
	IntervalDays  *int
	Weekdays      *int
	IntervalWeeks int
	MonthDay      *int
	AnchorDate    *string
	DueTime       string
}

func intOrZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func intPtr(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

// Parse validates stored columns (empty due time means the default, zero interval_weeks means 1) and clears the ones the kind does not use.
func Parse(c Columns) (Schedule, error) {
	s := Schedule{
		Kind:          Kind(c.Kind),
		IntervalDays:  intOrZero(c.IntervalDays),
		Weekdays:      intOrZero(c.Weekdays),
		IntervalWeeks: c.IntervalWeeks,
		MonthDay:      intOrZero(c.MonthDay),
		DueTime:       DefaultDueTime(),
	}
	if s.IntervalWeeks == 0 {
		s.IntervalWeeks = 1
	}
	if c.AnchorDate != nil && *c.AnchorDate != "" {
		d, err := ParseDate(*c.AnchorDate)
		if err != nil {
			return Schedule{}, err
		}
		s.AnchorDate = d
	}
	if c.DueTime != "" {
		t, err := ParseTimeOfDay(c.DueTime)
		if err != nil {
			return Schedule{}, err
		}
		s.DueTime = t
	}
	if err := s.Validate(); err != nil {
		return Schedule{}, err
	}
	return s.normalized(), nil
}

// Columns returns the stored form of s.
func (s Schedule) Columns() Columns {
	c := Columns{
		Kind:          string(s.Kind),
		IntervalDays:  intPtr(s.IntervalDays),
		Weekdays:      intPtr(s.Weekdays),
		IntervalWeeks: s.IntervalWeeks,
		MonthDay:      intPtr(s.MonthDay),
		DueTime:       s.DueTime.String(),
	}
	if c.IntervalWeeks == 0 {
		c.IntervalWeeks = 1
	}
	if !s.AnchorDate.IsZero() {
		a := s.AnchorDate.String()
		c.AnchorDate = &a
	}
	return c
}

// normalized clears the fields the kind ignores.
func (s Schedule) normalized() Schedule {
	n := Schedule{Kind: s.Kind, IntervalWeeks: 1, DueTime: s.DueTime}
	switch s.Kind {
	case AfterLast:
		n.IntervalDays = s.IntervalDays
	case EveryNDays:
		n.IntervalDays = s.IntervalDays
		n.AnchorDate = s.AnchorDate
	case Weekly:
		n.Weekdays = s.Weekdays
		n.IntervalWeeks = s.IntervalWeeks
		if s.IntervalWeeks > 1 {
			n.AnchorDate = s.AnchorDate
		}
	case Monthly:
		n.MonthDay = s.MonthDay
	}
	return n
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), ErrInvalid)
}

// Validate checks that the fields the kind needs are present and every field is in range.
func (s Schedule) Validate() error {
	if s.DueTime.Hour < 0 || s.DueTime.Hour > 23 || s.DueTime.Minute < 0 || s.DueTime.Minute > 59 {
		return invalid("due time %v out of range", s.DueTime)
	}
	if !s.AnchorDate.IsZero() && !s.AnchorDate.Valid() {
		return invalid("anchor date %v is not a calendar date", s.AnchorDate)
	}
	if !s.AnchorDate.IsZero() && (s.AnchorDate.Year < minAnchorYear || s.AnchorDate.Year > maxAnchorYear) {
		return invalid("anchor date %v not in years %d..%d", s.AnchorDate, minAnchorYear, maxAnchorYear)
	}
	switch s.Kind {
	case None:
		return nil
	case AfterLast:
		return checkRange("interval_days", s.IntervalDays, 1, maxIntervalDays)
	case EveryNDays:
		if err := checkRange("interval_days", s.IntervalDays, 1, maxIntervalDays); err != nil {
			return err
		}
		if s.AnchorDate.IsZero() {
			return invalid("every_n_days needs an anchor date")
		}
		return nil
	case Weekly:
		if err := checkRange("weekdays", s.Weekdays, 1, EveryDay); err != nil {
			return err
		}
		if err := checkRange("interval_weeks", s.IntervalWeeks, 1, maxIntervalWeeks); err != nil {
			return err
		}
		if s.IntervalWeeks > 1 && s.AnchorDate.IsZero() {
			return invalid("weekly every %d weeks needs an anchor date", s.IntervalWeeks)
		}
		return nil
	case Monthly:
		return checkRange("month_day", s.MonthDay, 1, 31)
	default:
		return invalid("unknown kind %q", s.Kind)
	}
}

func checkRange(name string, v, lo, hi int) error {
	if v < lo || v > hi {
		return invalid("%s %d not in %d..%d", name, v, lo, hi)
	}
	return nil
}

// Fixed reports whether the schedule repeats on a calendar grid rather than relative to the last completion.
func (s Schedule) Fixed() bool {
	return s.Kind == EveryNDays || s.Kind == Weekly || s.Kind == Monthly
}
