package reminders

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/notify"
	"github.com/reefshepherd/reefshepherd/internal/params"
)

// RaiseTestsDue raises, at the first call on or after the reminder time each local day, one summary per tank of the due tests not yet listed at their current due time.
func (s *Service) RaiseTestsDue(ctx context.Context, now time.Time) error {
	loc, err := s.location(ctx)
	if err != nil {
		return err
	}
	local := now.In(loc)
	day := local.Format(time.DateOnly)
	if s.digestDay == day {
		return nil
	}
	clock, err := s.Settings.GetReminderDefaultTime(ctx)
	if err != nil {
		return fmt.Errorf("read reminder time: %w", err)
	}
	at, err := time.Parse("15:04", clock)
	if err != nil {
		return fmt.Errorf("reminder time %q: %w", clock, err)
	}
	if now.Before(time.Date(local.Year(), local.Month(), local.Day(), at.Hour(), at.Minute(), 0, 0, loc)) {
		return nil
	}

	due, err := s.Params.DueTests(ctx, now)
	if err != nil {
		return fmt.Errorf("list due tests: %w", err)
	}
	var errs []error
	for _, tank := range groupByTank(due) {
		if err := s.raiseTankSummary(ctx, tank, day); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	s.digestDay = day
	return nil
}

// tankTests are the due parameters of one tank in display order.
type tankTests struct {
	id         int64
	name       string
	parameters []params.DueParameter
}

// groupByTank splits due, which lists each tank's parameters together, into one entry per tank.
func groupByTank(due []params.DueParameter) []tankTests {
	var out []tankTests
	for _, p := range due {
		if n := len(out); n == 0 || out[n-1].id != p.TankID {
			out = append(out, tankTests{id: p.TankID, name: p.TankName})
		}
		last := &out[len(out)-1]
		last.parameters = append(last.parameters, p)
	}
	return out
}

// raiseTankSummary raises the tank's summary for the local day when some due test has not been listed at its current due time, then records those tests as listed.
func (s *Service) raiseTankSummary(ctx context.Context, tank tankTests, day string) error {
	listed, err := db.Q(s.DB.R).ListTestReminders(ctx, tank.id)
	if err != nil {
		return fmt.Errorf("read listed tests of tank %d: %w", tank.id, err)
	}
	seen := make(map[listedTest]bool, len(listed))
	for _, l := range listed {
		seen[listedTest{l.ParameterID, l.DueAt}] = true
	}
	var fresh []params.DueParameter
	var names []string
	for _, p := range tank.parameters {
		if !seen[listedTest{p.ParameterID, db.FormatTime(p.DueAt)}] {
			fresh = append(fresh, p)
			names = append(names, p.Name)
		}
	}
	if len(fresh) == 0 {
		return nil
	}
	e := notify.Event{
		Type: eventTestsDue, Severity: "info",
		Title:  text("reminders_tests_due_title_one", tank.name),
		Body:   strings.Join(names, text("reminders_tests_due_separator")),
		Link:   fmt.Sprintf("/tanks/%d/tests", tank.id),
		TankID: &tank.id, SourceKind: sourceTank, SourceID: tank.id, Occurrence: day,
	}
	if len(names) > 1 {
		e.Title = text("reminders_tests_due_title_many", len(names), tank.name)
	}
	raised, err := s.Notify.Raise(ctx, e)
	if err != nil {
		return fmt.Errorf("raise tests summary for tank %d: %w", tank.id, err)
	}
	// A summary already raised today means these tests wait for the next day's.
	if !raised {
		return nil
	}
	// The notification service cannot share a transaction, so a crash right here lists these tests once more.
	return s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		for _, p := range fresh {
			arg := dbgen.InsertTestReminderParams{TankID: tank.id, ParameterID: p.ParameterID, DueAt: db.FormatTime(p.DueAt)}
			if err := q.InsertTestReminder(ctx, arg); err != nil {
				return fmt.Errorf("record listed test: %w", err)
			}
		}
		return nil
	})
}

// listedTest identifies a test at the due time that was listed.
type listedTest struct {
	parameterID int64
	dueAt       string
}
