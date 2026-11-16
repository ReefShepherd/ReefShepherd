package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"
)

func berlin(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func fixedAt(s string) func(context.Context) (string, error) {
	return func(context.Context) (string, error) { return s, nil }
}

func fixedLoc(loc *time.Location) func(context.Context) (*time.Location, error) {
	return func(context.Context) (*time.Location, error) { return loc, nil }
}

func TestEveryTickIsAlwaysDue(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	for _, last := range []time.Time{{}, now, now.Add(-time.Hour)} {
		due, err := EveryTick().Due(t.Context(), now, last)
		if err != nil || !due {
			t.Errorf("EveryTick due = %v, %v with last %v; want true", due, err, last)
		}
	}
}

func TestEveryFiresFirstThenAfterTheInterval(t *testing.T) {
	trigger := Every(time.Hour)
	start := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		now, last time.Time
		want      bool
	}{
		{"never ran", start, time.Time{}, true},
		{"just ran", start, start, false},
		{"one second short", start.Add(time.Hour - time.Second), start, false},
		{"exactly the interval", start.Add(time.Hour), start, true},
		{"long after", start.Add(5 * time.Hour), start, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := trigger.Due(t.Context(), tc.now, tc.last)
			if err != nil || got != tc.want {
				t.Errorf("Due = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}

// dailyFires steps a DailyAt trigger one minute at a time from from to to and returns the instants it fired.
func dailyFires(t *testing.T, trigger Trigger, from, to time.Time) []time.Time {
	t.Helper()
	var fires []time.Time
	var last time.Time
	for now := from; now.Before(to); now = now.Add(time.Minute) {
		due, err := trigger.Due(t.Context(), now, last)
		if err != nil {
			t.Fatal(err)
		}
		if due {
			fires = append(fires, now)
			last = now
		}
	}
	return fires
}

func TestDailyAtFiresOncePerLocalDayAcrossDST(t *testing.T) {
	loc := berlin(t)
	cases := []struct {
		name string
		at   string
		from time.Time
		days int
		// want is the local wall-clock time of each fire.
		want []string
	}{
		{
			name: "spring forward keeps 09:00 local", at: "09:00", days: 4,
			from: time.Date(2027, 3, 26, 0, 0, 0, 0, loc),
			want: []string{"2027-03-26 09:00", "2027-03-27 09:00", "2027-03-28 09:00", "2027-03-29 09:00"},
		},
		{
			name: "fall back keeps 09:00 local", at: "09:00", days: 4,
			from: time.Date(2026, 10, 24, 0, 0, 0, 0, loc),
			want: []string{"2026-10-24 09:00", "2026-10-25 09:00", "2026-10-26 09:00", "2026-10-27 09:00"},
		},
		{
			name: "a time skipped by spring forward still fires once", at: "02:30", days: 3,
			from: time.Date(2027, 3, 27, 0, 0, 0, 0, loc),
			want: []string{"2027-03-27 02:30", "2027-03-28 03:30", "2027-03-29 02:30"},
		},
		{
			name: "a time repeated by fall back fires once", at: "02:30", days: 3,
			from: time.Date(2026, 10, 24, 0, 0, 0, 0, loc),
			want: []string{"2026-10-24 02:30", "2026-10-25 02:30", "2026-10-26 02:30"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trigger := DailyAt(fixedAt(tc.at), fixedLoc(loc))
			fires := dailyFires(t, trigger, tc.from, tc.from.AddDate(0, 0, tc.days))
			var got []string
			for _, f := range fires {
				got = append(got, f.In(loc).Format("2006-01-02 15:04"))
			}
			if len(got) != len(tc.want) {
				t.Fatalf("fires = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("fires = %v, want %v", got, tc.want)
					break
				}
			}
		})
	}
}

func TestDailyAtCatchesUpOnceAfterDowntime(t *testing.T) {
	loc := berlin(t)
	trigger := DailyAt(fixedAt("09:00"), fixedLoc(loc))
	lastRun := time.Date(2026, 6, 1, 9, 0, 0, 0, loc)

	// Three days later, well past 09:00: due once.
	back := time.Date(2026, 6, 4, 14, 0, 0, 0, loc)
	due, err := trigger.Due(t.Context(), back, lastRun)
	if err != nil || !due {
		t.Fatalf("after downtime due = %v, %v; want true", due, err)
	}
	// Once it has run, later ticks that day stay quiet, and the next day fires at 09:00 again.
	for _, now := range []time.Time{back.Add(time.Minute), back.Add(5 * time.Hour), time.Date(2026, 6, 5, 8, 59, 0, 0, loc)} {
		if due, _ := trigger.Due(t.Context(), now, back); due {
			t.Errorf("due again at %v after the catch-up run", now)
		}
	}
	if due, _ := trigger.Due(t.Context(), time.Date(2026, 6, 5, 9, 0, 0, 0, loc), back); !due {
		t.Error("not due at 09:00 on the day after the catch-up")
	}
}

func TestDailyAtWaitsForTheTimeAndFiresAtStartupWhenPast(t *testing.T) {
	loc := berlin(t)
	trigger := DailyAt(fixedAt("09:00"), fixedLoc(loc))
	if due, _ := trigger.Due(t.Context(), time.Date(2026, 6, 1, 8, 59, 0, 0, loc), time.Time{}); due {
		t.Error("due before the time of day")
	}
	if due, _ := trigger.Due(t.Context(), time.Date(2026, 6, 1, 9, 0, 0, 0, loc), time.Time{}); !due {
		t.Error("not due at the time of day")
	}
	// A start-up after the time of day runs the job once, since nothing remembers whether it ran today.
	if due, _ := trigger.Due(t.Context(), time.Date(2026, 6, 1, 22, 0, 0, 0, loc), time.Time{}); !due {
		t.Error("not due on the first tick after the time of day")
	}
}

func TestDailyAtReportsBadInputs(t *testing.T) {
	loc := berlin(t)
	boom := errors.New("boom")
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, loc)
	cases := map[string]Trigger{
		"time source fails": DailyAt(func(context.Context) (string, error) { return "", boom }, fixedLoc(loc)),
		"zone source fails": DailyAt(fixedAt("09:00"), func(context.Context) (*time.Location, error) { return nil, boom }),
		"empty time":        DailyAt(fixedAt(""), fixedLoc(loc)),
		"no colon":          DailyAt(fixedAt("0900"), fixedLoc(loc)),
		"hour too large":    DailyAt(fixedAt("24:00"), fixedLoc(loc)),
		"minute too large":  DailyAt(fixedAt("09:60"), fixedLoc(loc)),
		"not numbers":       DailyAt(fixedAt("ab:cd"), fixedLoc(loc)),
		"single digits":     DailyAt(fixedAt("9:00"), fixedLoc(loc)),
	}
	for name, trigger := range cases {
		t.Run(name, func(t *testing.T) {
			if due, err := trigger.Due(t.Context(), now, time.Time{}); err == nil || due {
				t.Errorf("Due = %v, %v; want an error and false", due, err)
			}
		})
	}
}
