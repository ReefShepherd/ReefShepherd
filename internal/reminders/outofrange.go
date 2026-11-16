package reminders

import (
	"context"
	"fmt"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/notify"
	"github.com/reefshepherd/reefshepherd/internal/params"
)

// alertWindow is how far back a result may have been measured and still raise an out-of-range alert, so backfilling old tests stays quiet.
const alertWindow = 24 * time.Hour

// ResultCreated is the hook params calls once for each stored result; it never fails the caller, and problems are only logged.
func (s *Service) ResultCreated(ctx context.Context, r params.Result) {
	defer func() {
		if p := recover(); p != nil {
			s.Logger.ErrorContext(ctx, "out-of-range alert panicked", "result", r.ID, "panic", p)
		}
	}()
	if err := s.alertOutOfRange(ctx, r); err != nil {
		s.Logger.WarnContext(ctx, "out-of-range alert failed", "result", r.ID, "error", err)
	}
}

func (s *Service) alertOutOfRange(ctx context.Context, r params.Result) error {
	if r.MeasuredAt.Before(s.now().Add(-alertWindow)) {
		return nil
	}
	alert, outside, err := s.Params.RangeAlert(ctx, r)
	if err != nil {
		return fmt.Errorf("check range: %w", err)
	}
	if !outside {
		return nil
	}
	tankID := alert.TankID
	_, err = s.Notify.Raise(ctx, notify.Event{
		Type: eventOutOfRange, Severity: "warning",
		Title:  text("reminders_out_of_range_title", alert.ParameterName, alert.TankName),
		Body:   rangeBody(alert),
		Link:   fmt.Sprintf("/tanks/%d/charts/%d", alert.TankID, alert.ParameterID),
		TankID: &tankID, SourceKind: sourceResult, SourceID: r.ID, Occurrence: db.FormatTime(r.MeasuredAt),
	})
	if err != nil {
		return fmt.Errorf("raise alert: %w", err)
	}
	return nil
}

// rangeBody describes which side of the range the value is on, naming both limits when the range has both.
func rangeBody(a params.RangeAlert) string {
	switch {
	case a.Below && a.High != "":
		return text("reminders_out_of_range_body_below", a.Value, a.Low, a.High)
	case a.Below:
		return text("reminders_out_of_range_body_below_min", a.Value, a.Low)
	case a.Low != "":
		return text("reminders_out_of_range_body_above", a.Value, a.Low, a.High)
	default:
		return text("reminders_out_of_range_body_above_max", a.Value, a.High)
	}
}
