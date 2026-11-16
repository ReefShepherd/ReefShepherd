package maintenance

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/schedule"
)

// Limits on request fields.
const (
	maxNameRunes = 100
	maxTextRunes = 2000
	// maxFuture is how far past the server clock a completion time may lie, which absorbs small clock slips.
	maxFuture = 5 * time.Minute
)

// Refusals the handlers turn into 404 responses; the text is the message sent.
var (
	errNoTask       = errors.New("no such task")
	errNoPreset     = errors.New("no such preset")
	errNoCompletion = errors.New("no such completion")
)

// Service is the maintenance logic behind the HTTP handlers, the scheduler and the reminders; the errors it returns for a client mistake are field errors and the sentinels above.
type Service struct {
	Deps
	now func() time.Time
}

// NewService returns the service for deps.
func NewService(deps Deps) *Service {
	return &Service{Deps: deps, now: time.Now}
}

// DueTask is a maintenance task that is due for a reminder.
type DueTask struct {
	ID       int64
	TankID   *int64
	TankName string // empty for whole-system tasks
	Name     string
	DueAt    time.Time
}

// DueForReminders lists tasks that are not paused, have notify on, do not belong to an archived tank, and are due at or before now.
func (s *Service) DueForReminders(ctx context.Context, now time.Time) ([]DueTask, error) {
	loc, err := s.location(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.Q(s.DB.R).ListTasks(ctx, dbgen.ListTasksParams{RemindersOnly: 1})
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	var out []DueTask
	for _, row := range rows {
		_, due, err := evaluate(row, loc, now)
		if err != nil {
			return nil, err
		}
		if due == nil || due.After(now) {
			continue
		}
		t := DueTask{ID: row.ID, TankID: row.TankID, Name: row.Name, DueAt: *due}
		if row.TankName != nil {
			t.TankName = *row.TankName
		}
		out = append(out, t)
	}
	sortDue(out)
	return out, nil
}

// sortDue orders tasks by due time, then id.
func sortDue(tasks []DueTask) {
	slices.SortFunc(tasks, func(a, b DueTask) int {
		return cmp.Or(a.DueAt.Compare(b.DueAt), cmp.Compare(a.ID, b.ID))
	})
}

// location returns the instance timezone.
func (s *Service) location(ctx context.Context) (*time.Location, error) {
	name, err := s.Settings.GetTimezone(ctx)
	if err != nil {
		return nil, fmt.Errorf("read timezone: %w", err)
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("load timezone %q: %w", name, err)
	}
	return loc, nil
}

// reminderTime returns the instance reminder time, the due time of a schedule that names none.
func (s *Service) reminderTime(ctx context.Context) (string, error) {
	t, err := s.Settings.GetReminderDefaultTime(ctx)
	if err != nil {
		return "", fmt.Errorf("read reminder time: %w", err)
	}
	return t, nil
}

// Defaults are the instance values a new task or template starts from.
type Defaults struct {
	DueTime string `json:"due_time"`
}

// Defaults returns the instance reminder time.
func (s *Service) Defaults(ctx context.Context) (Defaults, error) {
	t, err := s.reminderTime(ctx)
	return Defaults{DueTime: t}, err
}

// scheduleFrom validates a schedule from a request, filling an empty due time from reminder and reporting problems in fields; a non-nil error is a fault that is not the client's.
func scheduleFrom(c *schedule.Columns, reminder string, fields *httpx.FieldErrors) (schedule.Schedule, error) {
	if c == nil {
		fields.Add("schedule", httpx.CodeRequired)
		return schedule.Schedule{}, nil
	}
	cols := *c
	if cols.Kind == "" {
		fields.Add("schedule.kind", httpx.CodeRequired)
		return schedule.Schedule{}, nil
	}
	if cols.DueTime == "" {
		cols.DueTime = reminder
	}
	sched, err := schedule.Parse(cols)
	if err != nil {
		if !fields.AddFrom(err) {
			return schedule.Schedule{}, err
		}
		return schedule.Schedule{}, nil
	}
	return sched, nil
}

func cleanName(name string, fields *httpx.FieldErrors) string {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		fields.Add("name", httpx.CodeRequired)
	case utf8.RuneCountInString(name) > maxNameRunes:
		fields.Add("name", httpx.CodeTooLong)
	}
	return name
}

// cleanText checks the length of a free text field, which is stored as typed.
func cleanText(text, field string, fields *httpx.FieldErrors) string {
	if utf8.RuneCountInString(text) > maxTextRunes {
		fields.Add(field, httpx.CodeTooLong)
	}
	return text
}

// optional returns the value p points at, or fallback when it is nil.
func optional[T any](p *T, fallback T) T {
	if p == nil {
		return fallback
	}
	return *p
}

// userRef returns the id as a nullable reference, nil for the zero id of a caller with no user.
func userRef(id int64) *int64 {
	if id == 0 {
		return nil
	}
	return &id
}
