package params

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/schedule"
	"github.com/reefshepherd/reefshepherd/internal/tanks"
)

// Schedule is a test schedule as the API reads and writes it.
type Schedule struct {
	Kind          string  `json:"kind"`
	IntervalDays  *int    `json:"interval_days"`
	Weekdays      *int    `json:"weekdays"`
	IntervalWeeks int     `json:"interval_weeks"`
	MonthDay      *int    `json:"month_day"`
	AnchorDate    *string `json:"anchor_date"`
	DueTime       string  `json:"due_time"`
}

func scheduleOf(s schedule.Schedule) Schedule {
	c := s.Columns()
	return Schedule{
		Kind: c.Kind, IntervalDays: c.IntervalDays, Weekdays: c.Weekdays, IntervalWeeks: c.IntervalWeeks,
		MonthDay: c.MonthDay, AnchorDate: c.AnchorDate, DueTime: c.DueTime,
	}
}

func (s Schedule) columns() schedule.Columns {
	return schedule.Columns{
		Kind: s.Kind, IntervalDays: s.IntervalDays, Weekdays: s.Weekdays, IntervalWeeks: s.IntervalWeeks,
		MonthDay: s.MonthDay, AnchorDate: s.AnchorDate, DueTime: s.DueTime,
	}
}

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
	Configured bool     `json:"configured"`
	Enabled    bool     `json:"enabled"`
	IdealLow   *float64 `json:"ideal_low"`
	IdealHigh  *float64 `json:"ideal_high"`
	Schedule   Schedule `json:"schedule"`
	SortOrder  int64    `json:"sort_order"`
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

func toInt64Ptr(p *int) *int64 {
	if p == nil {
		return nil
	}
	v := int64(*p)
	return &v
}

func toIntPtr(p *int64) *int {
	if p == nil {
		return nil
	}
	v := int(*p)
	return &v
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
	if row.Enabled == nil {
		cols := schedule.Columns{Kind: string(schedule.None), IntervalWeeks: 1}
		if n := deref(row.DefaultTestIntervalDays); n >= 1 {
			iv := int(n)
			cols = schedule.Columns{Kind: string(schedule.AfterLast), IntervalDays: &iv, IntervalWeeks: 1}
		}
		sched, err := schedule.Parse(cols)
		if err != nil {
			return tankSettings{}, fmt.Errorf("default schedule of parameter %d: %w", row.ParameterID, err)
		}
		return tankSettings{low: low, high: high, sched: sched, sortOrder: row.DefaultSortOrder}, nil
	}
	sched, err := schedule.Parse(schedule.Columns{
		Kind: deref(row.ScheduleKind), IntervalDays: toIntPtr(row.IntervalDays), Weekdays: toIntPtr(row.Weekdays),
		IntervalWeeks: int(deref(row.IntervalWeeks)), MonthDay: toIntPtr(row.MonthDay), AnchorDate: row.AnchorDate,
		DueTime: deref(row.DueTime),
	})
	if err != nil {
		return tankSettings{}, fmt.Errorf("schedule of parameter %d: %w", row.ParameterID, err)
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
		Schedule: scheduleOf(st.sched), SortOrder: st.sortOrder,
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

// tankParameters lists the tank's parameters, or just one when parameterID is set, which then also finds an archived one.
func (s *Service) tankParameters(ctx context.Context, q dbgen.DBTX, tankID int64, parameterID *int64) ([]TankParameter, string, error) {
	loc, err := s.location(ctx)
	if err != nil {
		return nil, "", err
	}
	rows, err := db.Q(q).ListTankParameters(ctx, dbgen.ListTankParametersParams{TankID: tankID, ParameterID: parameterID, IncludeArchived: db.Flag(parameterID != nil)})
	if err != nil {
		return nil, "", fmt.Errorf("list tank parameters: %w", err)
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
	items, tz, err := s.tankParameters(ctx, s.r, tankID, nil)
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
	Schedule  *Schedule
	SortOrder *int
}

// SetTankParameter creates or updates a parameter's settings in a tank and returns the result with its status.
func (s *Service) SetTankParameter(ctx context.Context, tankID, parameterID int64, in SetTankParameterInput) (TankParameter, error) {
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		if err := tanks.Require(ctx, tx, tankID); err != nil {
			return err
		}
		q := db.Q(tx)
		rows, err := q.ListTankParameters(ctx, dbgen.ListTankParametersParams{TankID: tankID, ParameterID: &parameterID, IncludeArchived: 1})
		if err != nil {
			return fmt.Errorf("read tank parameter: %w", err)
		}
		if len(rows) == 0 {
			return notFound("parameter")
		}
		row := rows[0]
		if row.ArchivedAt != nil {
			return httpx.Conflict("this parameter is archived")
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

		fields := fieldErrors{}
		if in.Enabled != nil {
			cur.enabled = *in.Enabled
		}
		cur.low = mergeCanonical(m, cur.low, in.IdealLow, "ideal_low", fields)
		cur.high = mergeCanonical(m, cur.high, in.IdealHigh, "ideal_high", fields)
		checkRangePair(cur.low, cur.high, "ideal_low", "ideal_high", fields)
		if in.Schedule != nil {
			if in.Schedule.Kind == "" {
				fields.add("schedule.kind", "is required")
			} else if sched, err := schedule.Parse(in.Schedule.columns()); err != nil {
				fields.add("schedule", scheduleMessage(err))
			} else {
				cur.sched = sched
			}
		}
		if in.SortOrder != nil {
			if *in.SortOrder < 0 || *in.SortOrder > maxSortOrder {
				fields.add("sort_order", fmt.Sprintf("must be between 0 and %d", maxSortOrder))
			} else {
				cur.sortOrder = int64(*in.SortOrder)
			}
		}
		if err := fields.err("invalid parameter settings"); err != nil {
			return err
		}

		cols := cur.sched.Columns()
		since := db.FormatTime(before.scheduleSince)
		if !before.configured || (cur.enabled && !before.enabled) || !reflect.DeepEqual(before.sched.Columns(), cols) {
			since = db.FormatTime(s.now())
		}
		if err := q.UpsertTankParameter(ctx, dbgen.UpsertTankParameterParams{
			TankID: tankID, ParameterID: parameterID, Enabled: db.Flag(cur.enabled), IdealLow: cur.low, IdealHigh: cur.high,
			ScheduleKind: cols.Kind, IntervalDays: toInt64Ptr(cols.IntervalDays), Weekdays: toInt64Ptr(cols.Weekdays),
			IntervalWeeks: int64(cols.IntervalWeeks), MonthDay: toInt64Ptr(cols.MonthDay), AnchorDate: cols.AnchorDate,
			DueTime: cols.DueTime, SortOrder: cur.sortOrder, ScheduleSince: since, UpdatedAt: db.FormatTime(s.now()),
		}); err != nil {
			return fmt.Errorf("save tank parameter: %w", err)
		}
		return nil
	})
	if err != nil {
		return TankParameter{}, err
	}
	items, _, err := s.tankParameters(ctx, s.r, tankID, &parameterID)
	if err != nil {
		return TankParameter{}, err
	}
	if len(items) != 1 {
		return TankParameter{}, errors.New("saved tank parameter not found")
	}
	return items[0], nil
}
