package reminders

import (
	"errors"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/notify"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

func TestTestsSummaryListsParametersInDisplayOrderAtTheReminderTime(t *testing.T) {
	e := newEnv(t)
	e.setTimezone("America/New_York")
	e.setReminderTime("09:00")
	tank := e.addTank("Display")
	// Display order is calcium, alkalinity, magnesium, which is neither alphabetical nor the order they were added in.
	e.enableTest(tank, "alkalinity", "2026-10-24T12:30:00Z", 2)
	e.enableTest(tank, "magnesium", "2026-10-23T12:00:00Z", 3)
	e.enableTest(tank, "calcium", "2026-10-23T12:00:00Z", 1)

	// 09:00 in New York is 13:00 UTC.
	e.tickTests("2026-10-24T12:59:00Z")
	if got := e.notes(); len(got) != 0 {
		t.Fatalf("before the reminder time: %+v", got)
	}
	e.tickTests("2026-10-24T13:00:00Z")

	got := e.notes()
	want := note{
		EventType: "tests_due", Severity: "info", Title: "3 tests due on Display", Body: "Calcium, Alkalinity, Magnesium",
		Link: "/tanks/1/tests", SourceKind: "tank_tests", SourceID: tank, Occurrence: "2026-10-24",
	}
	if len(got) != 1 || got[0].TankID == nil || *got[0].TankID != tank {
		t.Fatalf("notifications = %+v, want one for the tank", got)
	}
	if g := got[0]; withoutTank(g) != want {
		t.Errorf("summary = %+v, want %+v", g, want)
	}
}

func TestTestsSummaryNamesASingleTest(t *testing.T) {
	e := newEnv(t)
	tank := e.addTank("Frag tank")
	e.enableTest(tank, "nitrate", "2026-10-23T00:00:00Z", 1)
	e.tickTests("2026-10-24T09:00:00Z")
	got := e.notes()
	if len(got) != 1 || got[0].Title != "1 test due on Frag tank" || got[0].Body != "Nitrate" {
		t.Errorf("notifications = %+v", got)
	}
}

func TestTestsSummaryOnePerTankPerDayAndALaterDueParameterWaitsForTheNextDay(t *testing.T) {
	e := newEnv(t)
	e.setTimezone("America/New_York")
	e.setReminderTime("09:00")
	display, frag, late, old := e.addTank("Display"), e.addTank("Frag"), e.addTank("Late"), e.addTank("Old")
	e.enableTest(display, "calcium", "2026-10-23T12:00:00Z", 1)
	e.enableTest(display, "alkalinity", "2026-10-24T12:30:00Z", 2)
	// Falls due at 15:00 local time on the 24th, after the day's summary.
	e.enableTest(display, "magnesium", "2026-10-24T19:00:00Z", 3)
	e.enableTest(frag, "nitrate", "2026-10-24T01:00:00Z", 1)
	// Nothing is due on this tank at 09:00, and its only test falls due at 14:00 local time, so it waits for the next day.
	e.enableTest(late, "ph", "2026-10-24T18:00:00Z", 1)
	e.enableTest(old, "nitrate", "2026-10-20T00:00:00Z", 1)
	e.archiveTank(old)

	// A tick every ten minutes through the whole local day, 24 October 00:00 to 23:50.
	for m := 4 * 60; m < 28*60; m += 10 {
		e.tickTests(at("2026-10-24T00:00:00Z").Add(time.Duration(m) * time.Minute).Format(time.RFC3339))
	}
	got := e.notes()
	if len(got) != 2 {
		t.Fatalf("notifications on the first day = %+v, want one per tank", got)
	}
	if got[0].Title != "2 tests due on Display" || got[0].Body != "Calcium, Alkalinity" {
		t.Errorf("Display summary = %+v", got[0])
	}
	if got[1].Title != "1 test due on Frag" || got[1].Body != "Nitrate" {
		t.Errorf("Frag summary = %+v", got[1])
	}

	// The next day the parameter that fell due after the first summary is the only new one; the untested ones are not repeated.
	e.tickTests("2026-10-25T13:00:00Z")
	got = e.notes()
	if len(got) != 4 {
		t.Fatalf("notifications on the second day = %+v, want two more", got)
	}
	if g := got[2]; g.Title != "1 test due on Display" || g.Body != "Magnesium" || g.Occurrence != "2026-10-25" {
		t.Errorf("second day summary of Display = %+v", g)
	}
	if g := got[3]; g.Title != "1 test due on Late" || g.Body != "pH" || g.Occurrence != "2026-10-25" {
		t.Errorf("second day summary of Late = %+v", g)
	}

	// With nothing new, the third day is quiet.
	e.tickTests("2026-10-26T13:00:00Z")
	e.tickTests("2026-10-26T20:00:00Z")
	if n := len(e.notes()); n != 4 {
		t.Errorf("notifications after a day with nothing new = %d, want 4", n)
	}
}

func TestTestsSummaryIsSkippedWhenNothingIsDue(t *testing.T) {
	e := newEnv(t)
	tank := e.addTank("Display")
	alk := e.enableTest(tank, "alkalinity", "2026-10-01T00:00:00Z", 1)
	testutil.Exec(t, e.db, `UPDATE tank_parameters SET interval_days = 7 WHERE tank_id = ? AND parameter_id = ?`, tank, alk)
	testutil.Exec(t, e.db, `INSERT INTO test_results (tank_id, parameter_id, measured_at, value) VALUES (?, ?, '2026-10-23T12:00:00Z', 8)`, tank, alk)
	e.addTank("Empty tank")

	e.tickTests("2026-10-24T09:00:00Z")
	e.tickTests("2026-10-24T18:00:00Z")
	if got := e.notes(); len(got) != 0 {
		t.Errorf("notifications = %+v, want none", got)
	}
}

func TestTestsSummaryUsesTheInstanceTimeZoneForTheDay(t *testing.T) {
	e := newEnv(t)
	e.setTimezone("Pacific/Auckland")
	e.setReminderTime("09:00")
	tank := e.addTank("Display")
	e.enableTest(tank, "alkalinity", "2026-10-20T00:00:00Z", 1)

	// 09:00 on 24 October in Auckland (UTC+13) is 20:00 UTC on the 23rd.
	e.tickTests("2026-10-23T19:59:00Z")
	if n := len(e.notes()); n != 0 {
		t.Fatalf("one minute before the reminder time: %d notifications", n)
	}
	e.tickTests("2026-10-23T20:00:00Z")
	got := e.notes()
	if len(got) != 1 || got[0].Occurrence != "2026-10-24" {
		t.Errorf("notifications = %+v, want one for the local date 2026-10-24", got)
	}
}

func TestTestsSummaryFollowsTheReminderTimeAcrossDaylightSavingChanges(t *testing.T) {
	tests := []struct {
		name         string
		since        string
		before, from string
		date         string
	}{
		// Clocks go back on 1 November 2026: 09:00 is 14:00 UTC after the change, one hour later than the day before.
		{"clocks go back", "2026-10-01T00:00:00Z", "2026-11-01T13:59:00Z", "2026-11-01T14:00:00Z", "2026-11-01"},
		// Clocks go forward on 8 March 2026: 09:00 is 13:00 UTC after the change.
		{"clocks go forward", "2026-02-01T00:00:00Z", "2026-03-08T12:59:00Z", "2026-03-08T13:00:00Z", "2026-03-08"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.setTimezone("America/New_York")
			e.setReminderTime("09:00")
			e.enableTest(e.addTank("Display"), "alkalinity", tc.since, 1)

			e.tickTests(tc.before)
			if n := len(e.notes()); n != 0 {
				t.Fatalf("a minute before 09:00 local: %d notifications", n)
			}
			e.tickTests(tc.from)
			got := e.notes()
			if len(got) != 1 || got[0].Occurrence != tc.date {
				t.Errorf("notifications at 09:00 local = %+v, want one for %s", got, tc.date)
			}
		})
	}
}

func TestTestsSummaryRestartRaisesNothingExtra(t *testing.T) {
	e := newEnv(t)
	tank := e.addTank("Display")
	e.enableTest(tank, "alkalinity", "2026-10-20T00:00:00Z", 1)
	e.tickTests("2026-10-24T09:00:00Z")

	restarted := NewService(e.deps)
	for _, now := range []string{"2026-10-24T09:30:00Z", "2026-10-24T20:00:00Z", "2026-10-25T09:00:00Z"} {
		if err := restarted.RaiseTestsDue(t.Context(), at(now)); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(e.notes()); n != 1 {
		t.Errorf("notifications after a restart = %d, want 1", n)
	}
}

func TestTestsSummaryAfterDowntimeIsOneSummary(t *testing.T) {
	e := newEnv(t)
	tank := e.addTank("Display")
	e.enableTest(tank, "alkalinity", "2026-10-20T00:00:00Z", 1)
	e.enableTest(tank, "calcium", "2026-10-22T00:00:00Z", 2)

	// The first tick in days is on the 27th.
	e.tickTests("2026-10-27T10:00:00Z")
	got := e.notes()
	if len(got) != 1 || got[0].Title != "2 tests due on Display" || got[0].Occurrence != "2026-10-27" {
		t.Errorf("notifications = %+v, want one summary of both", got)
	}
}

func TestTestsSummaryFailureIsReturnedAndRetriedOnTheNextTick(t *testing.T) {
	e := newEnv(t)
	display, frag := e.addTank("Display"), e.addTank("Frag")
	e.enableTest(display, "alkalinity", "2026-10-20T00:00:00Z", 1)
	e.enableTest(frag, "alkalinity", "2026-10-20T00:00:00Z", 1)
	boom := errors.New("boom")
	e.raiser.fail = func(ev notify.Event) error {
		if ev.SourceID == display {
			return boom
		}
		return nil
	}

	e.now = at("2026-10-24T09:00:00Z")
	if err := e.svc.RaiseTestsDue(t.Context(), e.now); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want the raise failure", err)
	}
	if got := e.notes(); len(got) != 1 || got[0].SourceID != frag {
		t.Fatalf("notifications = %+v, want only the other tank", got)
	}

	e.raiser.fail = nil
	e.tickTests("2026-10-24T09:01:00Z")
	if got := e.titles(); len(got) != 2 {
		t.Errorf("notifications after the retry = %v, want 2", got)
	}
}

func TestTestsSummaryReportsAnUnreadableReminderTime(t *testing.T) {
	e := newEnv(t)
	testutil.Exec(t, e.db, `INSERT INTO settings (key, value) VALUES ('reminder_default_time', 'soon') ON CONFLICT (key) DO UPDATE SET value = excluded.value`)
	if err := e.svc.RaiseTestsDue(t.Context(), e.now); err == nil {
		t.Error("an unreadable reminder time raised no error")
	}
}
