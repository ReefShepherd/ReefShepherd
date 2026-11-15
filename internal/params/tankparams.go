package params

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/schedule"
	"github.com/reefshepherd/reefshepherd/internal/tanks"
)

// Latest is the newest result of a parameter in a tank.
type Latest struct {
	ResultID   int64   `json:"result_id"`
	Value      float64 `json:"value"`
	MeasuredAt string  `json:"measured_at"`
	InRange    bool    `json:"in_range"`
}

// TankParameter is one parameter's settings in a tank plus its test status; values are in display units.
type TankParameter struct {
	ParameterID int64   `json:"parameter_id"`
	PresetKey   *string `json:"preset_key"`
	Name        string  `json:"name"`
	UnitFamily  string  `json:"unit_family"`
	Unit        string  `json:"unit"`
	UnitLabel   string  `json:"unit_label"`
	Decimals    int     `json:"decimals"`
	// Configured is false for a parameter the tank has no settings row for, whose other settings then show the defaults.
	Configured bool             `json:"configured"`
	Enabled    bool             `json:"enabled"`
	IdealLow   *float64         `json:"ideal_low"`
	IdealHigh  *float64         `json:"ideal_high"`
	Schedule   schedule.Columns `json:"schedule"`
	SortOrder  int64            `json:"sort_order"`
	// Latest is null until the parameter has been tested in this tank.
	Latest *Latest `json:"latest"`
	// NextDue and Overdue are only set for an enabled parameter with a schedule.
	NextDue *string `json:"next_due"`
	Overdue bool    `json:"overdue"`
}

// TankParameters is the response of the per-tank parameter list.
type TankParameters struct {
	TankID     int64           `json:"tank_id"`
	Timezone   string          `json:"timezone"`
	Parameters []TankParameter `json:"parameters"`
}

// tankSettings is a row's settings with the defaults filled in for a parameter the tank has no row for.
type tankSettings struct {
	configured bool
	enabled    bool
	low, high  *float64
	sched      schedule.Schedule
	sortOrder  int64
	// scheduleSince is when the parameter was last enabled or given a new schedule, the due-time baseline until it is first tested.
	scheduleSince time.Time
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

// rangeOf returns the ideal range in canonical units, the definition's default while the tank has no settings.
func rangeOf(row dbgen.ListTankParametersRow) (low, high *float64) {
	if row.Enabled == nil {
		return row.DefaultLow, row.DefaultHigh
	}
	return row.IdealLow, row.IdealHigh
}

func settingsOf(row dbgen.ListTankParametersRow) (tankSettings, error) {
	low, high := rangeOf(row)
	sched, err := schedule.Parse(schedule.Columns{
		Kind: row.ScheduleKind, IntervalDays: row.IntervalDays, Weekdays: row.Weekdays, IntervalWeeks: deref(row.IntervalWeeks),
		MonthDay: row.MonthDay, AnchorDate: row.AnchorDate, DueTime: deref(row.DueTime),
	})
	if err != nil {
		// Not wrapped: a bad stored schedule is a server fault and must not reach the client as a field error.
		return tankSettings{}, fmt.Errorf("schedule of parameter %d: %v", row.ParameterID, err) //nolint:errorlint // see above
	}
	if row.Enabled == nil {
		return tankSettings{low: low, high: high, sched: sched, sortOrder: row.DefaultSortOrder}, nil
	}
	scheduleSince, err := db.ParseTime(deref(row.ScheduleSince))
	if err != nil {
		return tankSettings{}, fmt.Errorf("settings time of parameter %d: %w", row.ParameterID, err)
	}
	return tankSettings{
		configured: true, enabled: *row.Enabled == 1, low: low, high: high, sched: sched,
		sortOrder: deref(row.TankSortOrder), scheduleSince: scheduleSince,
	}, nil
}

// tankParameterOf builds the API item for a row, judging due times against now in loc.
func tankParameterOf(d *display, row dbgen.ListTankParametersRow, loc *time.Location, now time.Time) (TankParameter, error) {
	m, err := d.meta(row.UnitFamily, row.UnitLabel, row.Decimals)
	if err != nil {
		return TankParameter{}, err
	}
	st, err := settingsOf(row)
	if err != nil {
		return TankParameter{}, err
	}
	out := TankParameter{
		ParameterID: row.ParameterID, PresetKey: row.PresetKey, Name: row.Name,
		UnitFamily: row.UnitFamily, Unit: string(m.unit), UnitLabel: m.label, Decimals: m.decimals,
		Configured: st.configured, Enabled: st.enabled,
		IdealLow: m.showPtr(st.low), IdealHigh: m.showPtr(st.high),
		Schedule: st.sched.Columns(), SortOrder: st.sortOrder,
	}
	var lastDone *time.Time
	if row.LatestResultID != nil {
		at, err := db.ParseTime(deref(row.LatestMeasuredAt))
		if err != nil {
			return TankParameter{}, fmt.Errorf("measured time of result %d: %w", *row.LatestResultID, err)
		}
		value := deref(row.LatestValue)
		out.Latest = &Latest{
			ResultID: *row.LatestResultID, Value: m.show(value),
			MeasuredAt: db.FormatTime(at), InRange: m.inRange(value, st.low, st.high),
		}
		lastDone = &at
	}
	if st.enabled {
		if due, ok := schedule.NextDue(st.sched, lastDone, st.scheduleSince, loc); ok {
			next := db.FormatTime(due)
			out.NextDue = &next
			out.Overdue = now.After(due)
		}
	}
	return out, nil
}

// tankParameterRows reads the tank's parameter rows in display order, leaving out archived definitions unless includeArchived is set.
func tankParameterRows(ctx context.Context, q dbgen.DBTX, tankID int64, includeArchived bool) ([]dbgen.ListTankParametersRow, error) {
	rows, err := db.Q(q).ListTankParameters(ctx, dbgen.ListTankParametersParams{TankID: tankID, IncludeArchived: db.Flag(includeArchived)})
	if err != nil {
		return nil, fmt.Errorf("list tank parameters: %w", err)
	}
	return rows, nil
}

// tankParameterRow reads one parameter's row in the tank, archived or not, or errNoParameter when there is no such definition.
func tankParameterRow(ctx context.Context, q dbgen.DBTX, tankID, parameterID int64) (dbgen.ListTankParametersRow, error) {
	rows, err := db.Q(q).ListTankParameters(ctx, dbgen.ListTankParametersParams{TankID: tankID, ParameterID: &parameterID, IncludeArchived: 1})
	if err != nil {
		return dbgen.ListTankParametersRow{}, fmt.Errorf("read tank parameter: %w", err)
	}
	if len(rows) == 0 {
		return dbgen.ListTankParametersRow{}, errNoParameter
	}
	return rows[0], nil
}

// tankParameterItems builds the API items for rows and returns the name of the time zone their due times are judged in.
func (s *Service) tankParameterItems(ctx context.Context, rows []dbgen.ListTankParametersRow) ([]TankParameter, string, error) {
	loc, err := s.location(ctx)
	if err != nil {
		return nil, "", err
	}
	d, now := s.newDisplay(ctx), s.now()
	out := make([]TankParameter, 0, len(rows))
	for _, row := range rows {
		item, err := tankParameterOf(d, row, loc, now)
		if err != nil {
			return nil, "", err
		}
		out = append(out, item)
	}
	return out, loc.String(), nil
}

// ListTankParameters returns every active parameter with the tank's settings and test status.
func (s *Service) ListTankParameters(ctx context.Context, tankID int64) (TankParameters, error) {
	if err := tanks.Require(ctx, s.r, tankID); err != nil {
		return TankParameters{}, err
	}
	rows, err := tankParameterRows(ctx, s.r, tankID, false)
	if err != nil {
		return TankParameters{}, err
	}
	items, tz, err := s.tankParameterItems(ctx, rows)
	if err != nil {
		return TankParameters{}, err
	}
	return TankParameters{TankID: tankID, Timezone: tz, Parameters: items}, nil
}

// SetTankParameterInput holds the changes to a parameter's settings in a tank; nil and unset fields keep the current value, or the default for a parameter the tank has no settings for.
type SetTankParameterInput struct {
	Enabled   *bool
	IdealLow  httpx.Nullable[float64]
	IdealHigh httpx.Nullable[float64]
	Schedule  *schedule.Columns
	SortOrder *int
}

// SetTankParameter creates or updates a parameter's settings in a tank and returns the result with its status.
func (s *Service) SetTankParameter(ctx context.Context, tankID, parameterID int64, in SetTankParameterInput) (TankParameter, error) {
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		if err := tanks.Require(ctx, tx, tankID); err != nil {
			return err
		}
		row, err := tankParameterRow(ctx, tx, tankID, parameterID)
		if err != nil {
			return err
		}
		if row.ArchivedAt != nil {
			return errParameterArchived
		}
		m, err := s.newDisplay(ctx).meta(row.UnitFamily, row.UnitLabel, row.Decimals)
		if err != nil {
			return err
		}
		cur, err := settingsOf(row)
		if err != nil {
			return err
		}
		before := cur

		fields := httpx.FieldErrors{Message: "invalid parameter settings"}
		if in.Enabled != nil {
			cur.enabled = *in.Enabled
		}
		cur.low = mergeCanonical(m, cur.low, in.IdealLow, "ideal_low", &fields)
		cur.high = mergeCanonical(m, cur.high, in.IdealHigh, "ideal_high", &fields)
		checkRangePair(cur.low, cur.high, "ideal_low", "ideal_high", &fields)
		if in.Schedule != nil {
			if in.Schedule.Kind == "" {
				fields.Add("schedule.kind", httpx.CodeRequired)
			} else if sched, err := schedule.Parse(*in.Schedule); err != nil {
				if !fields.AddFrom(err) {
					return err
				}
			} else {
				cur.sched = sched
			}
		}
		if in.SortOrder != nil {
			if *in.SortOrder < 0 || *in.SortOrder > maxSortOrder {
				fields.Add("sort_order", httpx.CodeOutOfRange)
			} else {
				cur.sortOrder = int64(*in.SortOrder)
			}
		}
		if err := fields.Err(); err != nil {
			return err
		}

		cols := cur.sched.Columns()
		since := db.FormatTime(before.scheduleSince)
		if !before.configured || (cur.enabled && !before.enabled) || !before.sched.Columns().Equal(cols) {
			since = db.FormatTime(s.now())
		}
		if err := db.Q(tx).UpsertTankParameter(ctx, dbgen.UpsertTankParameterParams{
			TankID: tankID, ParameterID: parameterID, Enabled: db.Flag(cur.enabled), IdealLow: cur.low, IdealHigh: cur.high,
			ScheduleKind: cols.Kind, IntervalDays: cols.IntervalDays, Weekdays: cols.Weekdays,
			IntervalWeeks: cols.IntervalWeeks, MonthDay: cols.MonthDay, AnchorDate: cols.AnchorDate,
			DueTime: cols.DueTime, SortOrder: cur.sortOrder, ScheduleSince: since, UpdatedAt: db.FormatTime(s.now()),
		}); err != nil {
			return fmt.Errorf("save tank parameter: %w", err)
		}
		return nil
	})
	if err != nil {
		return TankParameter{}, err
	}
	row, err := tankParameterRow(ctx, s.r, tankID, parameterID)
	if err != nil {
		return TankParameter{}, err
	}
	items, _, err := s.tankParameterItems(ctx, []dbgen.ListTankParametersRow{row})
	if err != nil {
		return TankParameter{}, err
	}
	return items[0], nil
}
