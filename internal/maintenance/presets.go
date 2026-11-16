package maintenance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/schedule"
)

// Preset is a task template as the API shows it; its schedule always carries the current reminder time because templates store none.
type Preset struct {
	ID          int64            `json:"id"`
	PresetKey   *string          `json:"preset_key"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Tankless    bool             `json:"tankless"`
	Schedule    schedule.Columns `json:"schedule"`
	SortOrder   int64            `json:"sort_order"`
	ArchivedAt  *string          `json:"archived_at"`
}

func presetView(row dbgen.MaintenancePreset, reminder string) (Preset, error) {
	sched, err := schedule.Parse(schedule.Columns{
		Kind: row.ScheduleKind, IntervalDays: row.IntervalDays, Weekdays: row.Weekdays, IntervalWeeks: row.IntervalWeeks,
		MonthDay: row.MonthDay, AnchorDate: row.AnchorDate, DueTime: reminder,
	})
	if err != nil {
		// Not wrapped: a bad stored schedule is a server fault and must not reach the client as a field error.
		return Preset{}, fmt.Errorf("schedule of preset %d: %v", row.ID, err) //nolint:errorlint // see above
	}
	return Preset{
		ID: row.ID, PresetKey: row.PresetKey, Name: row.Name, Description: row.Description, Tankless: row.Tankless == 1,
		Schedule: sched.Columns(), SortOrder: row.SortOrder, ArchivedAt: row.ArchivedAt,
	}, nil
}

// ListPresets returns the templates in display order, hidden ones only when asked for.
func (s *Service) ListPresets(ctx context.Context, includeArchived bool) ([]Preset, error) {
	reminder, err := s.reminderTime(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.Q(s.DB.R).ListPresets(ctx, db.Flag(includeArchived))
	if err != nil {
		return nil, fmt.Errorf("list presets: %w", err)
	}
	out := make([]Preset, len(rows))
	for i, row := range rows {
		if out[i], err = presetView(row, reminder); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// CreatePresetInput is a new user template.
type CreatePresetInput struct {
	Name        string
	Description string
	Tankless    bool
	Schedule    *schedule.Columns
}

// CreatePreset adds a user template.
func (s *Service) CreatePreset(ctx context.Context, in CreatePresetInput) (Preset, error) {
	reminder, err := s.reminderTime(ctx)
	if err != nil {
		return Preset{}, err
	}
	fields := httpx.FieldErrors{Message: "invalid preset"}
	name := cleanName(in.Name, &fields)
	description := cleanText(in.Description, "description", &fields)
	sched, err := scheduleFrom(in.Schedule, reminder, &fields)
	if err != nil {
		return Preset{}, err
	}
	if err := fields.Err(); err != nil {
		return Preset{}, err
	}
	cols, now := sched.Columns(), db.FormatTime(s.now())
	var out Preset
	err = s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		id, err := q.InsertPreset(ctx, dbgen.InsertPresetParams{
			Name: name, Description: description, Tankless: db.Flag(in.Tankless), ScheduleKind: cols.Kind,
			IntervalDays: cols.IntervalDays, Weekdays: cols.Weekdays, IntervalWeeks: cols.IntervalWeeks,
			MonthDay: cols.MonthDay, AnchorDate: cols.AnchorDate, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("create preset: %w", err)
		}
		row, err := q.GetPreset(ctx, id)
		if err != nil {
			return fmt.Errorf("read preset: %w", err)
		}
		out, err = presetView(row, reminder)
		return err
	})
	return out, err
}

// UpdatePresetInput changes the fields that are set; a nil field is left alone.
type UpdatePresetInput struct {
	Name        *string
	Description *string
	Tankless    *bool
	Schedule    *schedule.Columns
	// Archived hides (true) or shows (false) the template.
	Archived *bool
}

// UpdatePreset changes a template, shipped or not.
func (s *Service) UpdatePreset(ctx context.Context, id int64, in UpdatePresetInput) (Preset, error) {
	reminder, err := s.reminderTime(ctx)
	if err != nil {
		return Preset{}, err
	}
	now := db.FormatTime(s.now())
	var out Preset
	err = s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		cur, err := q.GetPreset(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return errNoPreset
		}
		if err != nil {
			return fmt.Errorf("read preset: %w", err)
		}
		fields := httpx.FieldErrors{Message: "invalid preset"}
		p := dbgen.UpdatePresetParams{
			ID: id, Name: cur.Name, Description: cur.Description, Tankless: cur.Tankless, ScheduleKind: cur.ScheduleKind,
			IntervalDays: cur.IntervalDays, Weekdays: cur.Weekdays, IntervalWeeks: cur.IntervalWeeks, MonthDay: cur.MonthDay,
			AnchorDate: cur.AnchorDate, ArchivedAt: cur.ArchivedAt, UpdatedAt: now,
		}
		if in.Name != nil {
			p.Name = cleanName(*in.Name, &fields)
		}
		if in.Description != nil {
			p.Description = cleanText(*in.Description, "description", &fields)
		}
		if in.Tankless != nil {
			p.Tankless = db.Flag(*in.Tankless)
		}
		if in.Schedule != nil {
			sched, err := scheduleFrom(in.Schedule, reminder, &fields)
			if err != nil {
				return err
			}
			cols := sched.Columns()
			p.ScheduleKind, p.IntervalDays, p.Weekdays, p.IntervalWeeks = cols.Kind, cols.IntervalDays, cols.Weekdays, cols.IntervalWeeks
			p.MonthDay, p.AnchorDate = cols.MonthDay, cols.AnchorDate
		}
		switch {
		case in.Archived == nil:
		case !*in.Archived:
			p.ArchivedAt = nil
		case cur.ArchivedAt == nil:
			p.ArchivedAt = &now
		}
		if err := fields.Err(); err != nil {
			return err
		}
		if _, err := q.UpdatePreset(ctx, p); err != nil {
			return fmt.Errorf("update preset: %w", err)
		}
		row, err := q.GetPreset(ctx, id)
		if err != nil {
			return fmt.Errorf("read preset: %w", err)
		}
		out, err = presetView(row, reminder)
		return err
	})
	return out, err
}
