package scheduler

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Trigger decides whether a job should start on a tick.
type Trigger interface {
	// Due reports whether the job should start at now, given when it last started (zero if never).
	Due(ctx context.Context, now, last time.Time) (bool, error)
}

type triggerFunc func(ctx context.Context, now, last time.Time) (bool, error)

func (f triggerFunc) Due(ctx context.Context, now, last time.Time) (bool, error) {
	return f(ctx, now, last)
}

// EveryTick fires on every tick.
func EveryTick() Trigger {
	return triggerFunc(func(context.Context, time.Time, time.Time) (bool, error) { return true, nil })
}

// Every fires on the first tick, then once at least d has passed since the last start.
func Every(d time.Duration) Trigger {
	return triggerFunc(func(_ context.Context, now, last time.Time) (bool, error) {
		return last.IsZero() || now.Sub(last) >= d, nil
	})
}

// DailyAt fires once per local day at the first tick at or after the "HH:MM" time, so after downtime it catches up once.
func DailyAt(at func(ctx context.Context) (string, error), loc func(ctx context.Context) (*time.Location, error)) Trigger {
	return triggerFunc(func(ctx context.Context, now, last time.Time) (bool, error) {
		clock, err := at(ctx)
		if err != nil {
			return false, fmt.Errorf("read daily time: %w", err)
		}
		hour, minute, err := parseClock(clock)
		if err != nil {
			return false, err
		}
		zone, err := loc(ctx)
		if err != nil {
			return false, fmt.Errorf("read time zone: %w", err)
		}
		local := now.In(zone)
		target := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, zone)
		return !now.Before(target) && last.Before(target), nil
	})
}

// parseClock reads a 24-hour "HH:MM" time of day.
func parseClock(s string) (hour, minute int, err error) {
	h, m, ok := strings.Cut(s, ":")
	if !ok || len(h) != 2 || len(m) != 2 {
		return 0, 0, fmt.Errorf("time of day %q is not HH:MM", s)
	}
	hour, errH := strconv.Atoi(h)
	minute, errM := strconv.Atoi(m)
	if errH != nil || errM != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("time of day %q is not HH:MM", s)
	}
	return hour, minute, nil
}
