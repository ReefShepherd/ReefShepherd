package maintenance

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/schedule"
	"github.com/reefshepherd/reefshepherd/internal/tanks"
)

// Task list statuses.
const (
	StatusDue      = "due"
	StatusUpcoming = "upcoming"
	StatusAll      = "all"
)

// Task is a maintenance task as the API shows it.
type Task struct {
	ID          int64            `json:"id"`
	TankID      *int64           `json:"tank_id"`
	TankName    *string          `json:"tank_name"`
	PresetID    *int64           `json:"preset_id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Schedule    schedule.Columns `json:"schedule"`
	Notify      bool             `json:"notify"`
	Paused      bool             `json:"paused"`
	LastDoneAt  *string          `json:"last_done_at"`
	LastOutcome *string          `json:"last_outcome"`
	NextDueAt   *string          `json:"next_due_at"`
	Overdue     bool             `json:"overdue"`
	CreatedAt   string           `json:"created_at"`
	UpdatedAt   string           `json:"updated_at"`
}

func taskColumns(row dbgen.ListTasksRow) schedule.Columns {
	return schedule.Columns{
		Kind: row.ScheduleKind, IntervalDays: row.IntervalDays, Weekdays: row.Weekdays, IntervalWeeks: row.IntervalWeeks,
		MonthDay: row.MonthDay, AnchorDate: row.AnchorDate, DueTime: row.DueTime,
	}
}

// parseStored reads a stored schedule, where a failure is a server fault.
func parseStored(row dbgen.ListTasksRow) (schedule.Schedule, error) {
	sched, err := schedule.Parse(taskColumns(row))
	if err != nil {
		// Not wrapped: a bad stored schedule is a server fault and must not reach the client as a field error.
		return schedule.Schedule{}, fmt.Errorf("schedule of task %d: %v", row.ID, err) //nolint:errorlint // see above
	}
	return sched, nil
}

// evaluate builds the API item for a row and returns the instant the task is next due, nil when its schedule has none.
func evaluate(row dbgen.ListTasksRow, loc *time.Location, now time.Time) (Task, *time.Time, error) {
	sched, err := parseStored(row)
	if err != nil {
		return Task{}, nil, err
	}
	since, err := db.ParseTime(row.ScheduleSince)
	if err != nil {
		return Task{}, nil, fmt.Errorf("schedule time of task %d: %w", row.ID, err)
	}
	var lastDone *time.Time
	if row.LastDoneAt != nil {
		at, err := db.ParseTime(*row.LastDoneAt)
		if err != nil {
			return Task{}, nil, fmt.Errorf("completion time of task %d: %w", row.ID, err)
		}
		lastDone = &at
	}
	t := Task{
		ID: row.ID, TankID: row.TankID, TankName: row.TankName, PresetID: row.PresetID, Name: row.Name, Description: row.Description,
		Schedule: sched.Columns(), Notify: row.Notify == 1, Paused: row.Paused == 1,
		LastDoneAt: row.LastDoneAt, LastOutcome: row.LastOutcome, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	due, ok := schedule.NextDue(sched, lastDone, since, loc)
	if !ok {
		return t, nil, nil
	}
	next := db.FormatTime(due)
	t.NextDueAt = &next
	t.Overdue = due.Before(now)
	return t, &due, nil
}

// readTaskRow returns one task by id from conn, or errNoTask.
func readTaskRow(ctx context.Context, conn dbgen.DBTX, id int64) (dbgen.ListTasksRow, error) {
	rows, err := db.Q(conn).ListTasks(ctx, dbgen.ListTasksParams{TaskID: &id})
	if err != nil {
		return dbgen.ListTasksRow{}, fmt.Errorf("read task: %w", err)
	}
	if len(rows) == 0 {
		return dbgen.ListTasksRow{}, errNoTask
	}
	return rows[0], nil
}

func (s *Service) readTask(ctx context.Context, conn dbgen.DBTX, id int64, loc *time.Location) (Task, error) {
	row, err := readTaskRow(ctx, conn, id)
	if err != nil {
		return Task{}, err
	}
	t, _, err := evaluate(row, loc, s.now())
	return t, err
}

// GetTask returns one task, whatever its tank.
func (s *Service) GetTask(ctx context.Context, id int64) (Task, error) {
	loc, err := s.location(ctx)
	if err != nil {
		return Task{}, err
	}
	return s.readTask(ctx, s.DB.R, id, loc)
}

// TaskFilter narrows the task list.
type TaskFilter struct {
	// TankID limits the list to one tank; nil lists every tank except archived ones.
	TankID *int64
	// IncludeTankless adds whole-system tasks when TankID is set.
	IncludeTankless bool
	// Status is StatusDue, StatusUpcoming or StatusAll; empty means StatusDue.
	Status string
}

// ListTasks returns the tasks matching f, soonest due first with paused and unscheduled tasks last.
func (s *Service) ListTasks(ctx context.Context, f TaskFilter) ([]Task, error) {
	status := cmp.Or(f.Status, StatusDue)
	if !slices.Contains([]string{StatusDue, StatusUpcoming, StatusAll}, status) {
		return nil, httpx.Reject("invalid query", "status", httpx.CodeInvalid)
	}
	loc, err := s.location(ctx)
	if err != nil {
		return nil, err
	}
	var rows []dbgen.ListTasksRow
	err = s.DB.InReadTx(ctx, func(tx *sql.Tx) error {
		if f.TankID != nil {
			if err := tanks.Require(ctx, tx, *f.TankID); err != nil {
				return err
			}
		}
		var err error
		rows, err = db.Q(tx).ListTasks(ctx, dbgen.ListTasksParams{TankID: f.TankID, IncludeTankless: db.Flag(f.IncludeTankless)})
		if err != nil {
			return fmt.Errorf("list tasks: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	now := s.now()
	y, m, d := now.In(loc).Date()
	endOfToday := time.Date(y, m, d+1, 0, 0, 0, 0, loc)
	type item struct {
		task Task
		due  *time.Time
	}
	var items []item
	for _, row := range rows {
		t, due, err := evaluate(row, loc, now)
		if err != nil {
			return nil, err
		}
		if status != StatusAll {
			active := !t.Paused && due != nil
			if !active || (status == StatusDue) != due.Before(endOfToday) {
				continue
			}
		}
		items = append(items, item{t, due})
	}
	slices.SortStableFunc(items, func(a, b item) int {
		if c := cmp.Compare(boolRank(a.task.Paused), boolRank(b.task.Paused)); c != 0 {
			return c
		}
		switch {
		case a.due == nil && b.due != nil:
			return 1
		case a.due != nil && b.due == nil:
			return -1
		case a.due != nil:
			if c := a.due.Compare(*b.due); c != 0 {
				return c
			}
		}
		return cmp.Or(strings.Compare(strings.ToLower(a.task.Name), strings.ToLower(b.task.Name)), cmp.Compare(a.task.ID, b.task.ID))
	})
	out := make([]Task, len(items))
	for i, it := range items {
		out[i] = it.task
	}
	return out, nil
}

func boolRank(b bool) int {
	if b {
		return 1
	}
	return 0
}

// CreateTaskInput is a new task; a nil TankID makes it a whole-system task.
type CreateTaskInput struct {
	TankID      *int64
	PresetID    *int64
	Name        string
	Description string
	Schedule    *schedule.Columns
	// Notify defaults to true and Paused to false.
	Notify *bool
	Paused *bool
	// LastDoneOn is a YYYY-MM-DD date the task was last done, recorded as a completion.
	LastDoneOn *string
}

// CreateTask adds a task, and the completion for LastDoneOn when given, in one transaction; userID is recorded on that completion.
func (s *Service) CreateTask(ctx context.Context, userID int64, in CreateTaskInput) (Task, error) {
	reminder, err := s.reminderTime(ctx)
	if err != nil {
		return Task{}, err
	}
	loc, err := s.location(ctx)
	if err != nil {
		return Task{}, err
	}
	now := s.now()
	fields := httpx.FieldErrors{Message: "invalid task"}
	name := cleanName(in.Name, &fields)
	description := cleanText(in.Description, "description", &fields)
	sched, err := scheduleFrom(in.Schedule, reminder, &fields)
	if err != nil {
		return Task{}, err
	}
	lastDone := cleanLastDoneOn(in.LastDoneOn, sched, loc, now, &fields)

	var out Task
	err = s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		if err := checkTankAndPreset(ctx, tx, in.TankID, in.PresetID, &fields); err != nil {
			return err
		}
		if err := fields.Err(); err != nil {
			return err
		}
		cols, stamp := sched.Columns(), db.FormatTime(now)
		id, err := q.InsertTask(ctx, dbgen.InsertTaskParams{
			TankID: in.TankID, PresetID: in.PresetID, Name: name, Description: description, ScheduleKind: cols.Kind,
			IntervalDays: cols.IntervalDays, Weekdays: cols.Weekdays, IntervalWeeks: cols.IntervalWeeks, MonthDay: cols.MonthDay,
			AnchorDate: cols.AnchorDate, DueTime: cols.DueTime, ScheduleSince: stamp,
			Notify: db.Flag(optional(in.Notify, true)), Paused: db.Flag(optional(in.Paused, false)), CreatedAt: stamp, UpdatedAt: stamp,
		})
		if err != nil {
			return fmt.Errorf("create task: %w", err)
		}
		if lastDone != nil {
			if _, err := q.InsertCompletion(ctx, dbgen.InsertCompletionParams{
				TaskID: id, DoneAt: db.FormatTime(*lastDone), Outcome: outcomeDone, DoneBy: userRef(userID), CreatedAt: stamp,
			}); err != nil {
				return fmt.Errorf("record last done: %w", err)
			}
		}
		out, err = s.readTask(ctx, tx, id, loc)
		return err
	})
	return out, err
}

// checkTankAndPreset reports a tank or preset id that does not exist in fields.
func checkTankAndPreset(ctx context.Context, conn dbgen.DBTX, tankID, presetID *int64, fields *httpx.FieldErrors) error {
	if tankID != nil {
		if err := tanks.Require(ctx, conn, *tankID); errors.Is(err, tanks.ErrNotFound) {
			fields.Add("tank_id", httpx.CodeInvalid)
		} else if err != nil {
			return err
		}
	}
	if presetID != nil {
		if _, err := db.Q(conn).GetPreset(ctx, *presetID); errors.Is(err, sql.ErrNoRows) {
			fields.Add("preset_id", httpx.CodeInvalid)
		} else if err != nil {
			return fmt.Errorf("read preset: %w", err)
		}
	}
	return nil
}

// cleanLastDoneOn turns a last-done date into the instant of that day's due time, capped at now so a task done today is not dated in the future.
func cleanLastDoneOn(raw *string, sched schedule.Schedule, loc *time.Location, now time.Time, fields *httpx.FieldErrors) *time.Time {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil
	}
	s := strings.TrimSpace(*raw)
	day, err := schedule.ParseDate(s)
	if err != nil || day.String() != s {
		fields.Add("last_done_on", httpx.CodeInvalid)
		return nil
	}
	if schedule.DateOf(now, loc).Before(day) {
		fields.Add("last_done_on", httpx.CodeOutOfRange)
		return nil
	}
	at := day.At(sched.DueTime, loc)
	if at.After(now) {
		at = now
	}
	return &at
}

// UpdateTaskInput changes the fields that are set; a nil field is left alone.
type UpdateTaskInput struct {
	// TankID moves the task to a tank, or to the whole system when set to null.
	TankID      httpx.Nullable[int64]
	Name        *string
	Description *string
	Schedule    *schedule.Columns
	Notify      *bool
	Paused      *bool
}

// UpdateTask changes a task; a real schedule change restarts the baseline of its first due date.
func (s *Service) UpdateTask(ctx context.Context, id int64, in UpdateTaskInput) (Task, error) {
	reminder, err := s.reminderTime(ctx)
	if err != nil {
		return Task{}, err
	}
	loc, err := s.location(ctx)
	if err != nil {
		return Task{}, err
	}
	now := db.FormatTime(s.now())
	var out Task
	err = s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		cur, err := readTaskRow(ctx, tx, id)
		if err != nil {
			return err
		}
		curSched, err := parseStored(cur)
		if err != nil {
			return err
		}
		fields := httpx.FieldErrors{Message: "invalid task"}
		p := dbgen.UpdateTaskParams{
			ID: id, TankID: cur.TankID, Name: cur.Name, Description: cur.Description, ScheduleKind: cur.ScheduleKind,
			IntervalDays: cur.IntervalDays, Weekdays: cur.Weekdays, IntervalWeeks: cur.IntervalWeeks, MonthDay: cur.MonthDay,
			AnchorDate: cur.AnchorDate, DueTime: cur.DueTime, ScheduleSince: cur.ScheduleSince, Notify: cur.Notify,
			Paused: cur.Paused, UpdatedAt: now,
		}
		if in.TankID.Set {
			p.TankID = in.TankID.Value
			if err := checkTankAndPreset(ctx, tx, in.TankID.Value, nil, &fields); err != nil {
				return err
			}
		}
		if in.Name != nil {
			p.Name = cleanName(*in.Name, &fields)
		}
		if in.Description != nil {
			p.Description = cleanText(*in.Description, "description", &fields)
		}
		if in.Schedule != nil {
			sched, err := scheduleFrom(in.Schedule, reminder, &fields)
			if err != nil {
				return err
			}
			if !curSched.Columns().Equal(sched.Columns()) {
				cols := sched.Columns()
				p.ScheduleKind, p.IntervalDays, p.Weekdays, p.IntervalWeeks = cols.Kind, cols.IntervalDays, cols.Weekdays, cols.IntervalWeeks
				p.MonthDay, p.AnchorDate, p.DueTime, p.ScheduleSince = cols.MonthDay, cols.AnchorDate, cols.DueTime, now
			}
		}
		if in.Notify != nil {
			p.Notify = db.Flag(*in.Notify)
		}
		if in.Paused != nil {
			p.Paused = db.Flag(*in.Paused)
		}
		if err := fields.Err(); err != nil {
			return err
		}
		if _, err := q.UpdateTask(ctx, p); err != nil {
			return fmt.Errorf("update task: %w", err)
		}
		out, err = s.readTask(ctx, tx, id, loc)
		return err
	})
	return out, err
}

// DeleteTask removes a task and its completions.
func (s *Service) DeleteTask(ctx context.Context, id int64) error {
	return s.DB.InTx(ctx, func(tx *sql.Tx) error {
		n, err := db.Q(tx).DeleteTask(ctx, id)
		if err != nil {
			return fmt.Errorf("delete task: %w", err)
		}
		if n == 0 {
			return errNoTask
		}
		return nil
	})
}
