package maintenance

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

// Completion outcomes.
const (
	outcomeDone    = "done"
	outcomeSkipped = "skipped"
)

// Completion page sizes.
const (
	defaultPageSize = 50
	maxPageSize     = 200
)

// Completion is one entry in a task's history.
type Completion struct {
	ID         int64   `json:"id"`
	TaskID     int64   `json:"task_id"`
	DoneAt     string  `json:"done_at"`
	Outcome    string  `json:"outcome"`
	Note       string  `json:"note"`
	DoneByName *string `json:"done_by_name"`
	CreatedAt  string  `json:"created_at"`
}

// completionOf builds the API item, naming the user by display name and falling back to the username.
func completionOf(id, taskID int64, doneAt, outcome, note, createdAt string, displayName, username *string) Completion {
	c := Completion{ID: id, TaskID: taskID, DoneAt: doneAt, Outcome: outcome, Note: note, CreatedAt: createdAt}
	switch {
	case displayName != nil && *displayName != "":
		c.DoneByName = displayName
	case username != nil:
		c.DoneByName = username
	}
	return c
}

func readCompletion(ctx context.Context, conn dbgen.DBTX, id int64) (Completion, error) {
	row, err := db.Q(conn).GetCompletion(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Completion{}, errNoCompletion
	}
	if err != nil {
		return Completion{}, fmt.Errorf("read completion: %w", err)
	}
	return completionOf(row.ID, row.TaskID, row.DoneAt, row.Outcome, row.Note, row.CreatedAt, row.DoneByDisplayName, row.DoneByUsername), nil
}

// cleanDoneAt reads an RFC 3339 completion time, which may lie at most maxFuture past now.
func cleanDoneAt(raw string, now time.Time, fields *httpx.FieldErrors) time.Time {
	t, err := db.ParseTime(raw)
	switch {
	case err != nil:
		fields.Add("done_at", httpx.CodeInvalid)
	case t.After(now.Add(maxFuture)):
		fields.Add("done_at", httpx.CodeOutOfRange)
	}
	return t.Truncate(time.Second)
}

func cleanOutcome(outcome string, fields *httpx.FieldErrors) string {
	if outcome != outcomeDone && outcome != outcomeSkipped {
		fields.Add("outcome", httpx.CodeInvalid)
	}
	return outcome
}

// CompleteInput is a completion to record; nil fields take their defaults of now and done.
type CompleteInput struct {
	DoneAt  *string
	Outcome *string
	Note    string
}

// CompleteResult is a recorded completion with the task as it stands afterwards.
type CompleteResult struct {
	Completion Completion `json:"completion"`
	Task       Task       `json:"task"`
}

// Complete marks a task done or skipped; userID is recorded as who did it.
func (s *Service) Complete(ctx context.Context, taskID, userID int64, in CompleteInput) (CompleteResult, error) {
	loc, err := s.location(ctx)
	if err != nil {
		return CompleteResult{}, err
	}
	now := s.now()
	fields := httpx.FieldErrors{Message: "invalid completion"}
	doneAt := now.Truncate(time.Second)
	if in.DoneAt != nil {
		doneAt = cleanDoneAt(*in.DoneAt, now, &fields)
	}
	outcome := outcomeDone
	if in.Outcome != nil {
		outcome = cleanOutcome(*in.Outcome, &fields)
	}
	note := cleanText(in.Note, "note", &fields)

	var out CompleteResult
	err = s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		if n, err := q.TaskExists(ctx, taskID); err != nil {
			return fmt.Errorf("check task: %w", err)
		} else if n == 0 {
			return errNoTask
		}
		if err := fields.Err(); err != nil {
			return err
		}
		id, err := q.InsertCompletion(ctx, dbgen.InsertCompletionParams{
			TaskID: taskID, DoneAt: db.FormatTime(doneAt), Outcome: outcome, Note: note, DoneBy: userRef(userID), CreatedAt: db.FormatTime(now),
		})
		if err != nil {
			return fmt.Errorf("record completion: %w", err)
		}
		if out.Completion, err = readCompletion(ctx, tx, id); err != nil {
			return err
		}
		out.Task, err = s.readTask(ctx, tx, taskID, loc)
		return err
	})
	return out, err
}

// UpdateCompletionInput changes the fields that are set; a nil field is left alone.
type UpdateCompletionInput struct {
	DoneAt  *string
	Outcome *string
	Note    *string
}

// UpdateCompletion corrects a history entry, which moves the task's next due date.
func (s *Service) UpdateCompletion(ctx context.Context, id int64, in UpdateCompletionInput) (Completion, error) {
	now := s.now()
	var out Completion
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		cur, err := readCompletion(ctx, tx, id)
		if err != nil {
			return err
		}
		fields := httpx.FieldErrors{Message: "invalid completion"}
		p := dbgen.UpdateCompletionParams{ID: id, DoneAt: cur.DoneAt, Outcome: cur.Outcome, Note: cur.Note}
		if in.DoneAt != nil {
			p.DoneAt = db.FormatTime(cleanDoneAt(*in.DoneAt, now, &fields))
		}
		if in.Outcome != nil {
			p.Outcome = cleanOutcome(*in.Outcome, &fields)
		}
		if in.Note != nil {
			p.Note = cleanText(*in.Note, "note", &fields)
		}
		if err := fields.Err(); err != nil {
			return err
		}
		if _, err := q.UpdateCompletion(ctx, p); err != nil {
			return fmt.Errorf("update completion: %w", err)
		}
		out, err = readCompletion(ctx, tx, id)
		return err
	})
	return out, err
}

// DeleteCompletion removes a history entry, which moves the task's next due date.
func (s *Service) DeleteCompletion(ctx context.Context, id int64) error {
	return s.DB.InTx(ctx, func(tx *sql.Tx) error {
		n, err := db.Q(tx).DeleteCompletion(ctx, id)
		if err != nil {
			return fmt.Errorf("delete completion: %w", err)
		}
		if n == 0 {
			return errNoCompletion
		}
		return nil
	})
}

// CompletionPage is one page of a task's history, newest first.
type CompletionPage = httpx.Page[Completion]

// ListCompletions returns a page of the task's history; limit 0 means the default page size.
func (s *Service) ListCompletions(ctx context.Context, taskID int64, cursor string, limit int) (CompletionPage, error) {
	limit = cmp.Or(limit, defaultPageSize)
	arg := dbgen.ListTaskCompletionsParams{TaskID: taskID, RowLimit: int64(limit) + 1}
	if cursor != "" {
		at, id, err := decodeCursor(cursor)
		if err != nil {
			return CompletionPage{}, httpx.Reject("invalid cursor", "cursor", httpx.CodeInvalid)
		}
		arg.CursorAt, arg.CursorID = &at, &id
	}
	var rows []dbgen.ListTaskCompletionsRow
	err := s.DB.InReadTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		if n, err := q.TaskExists(ctx, taskID); err != nil {
			return fmt.Errorf("check task: %w", err)
		} else if n == 0 {
			return errNoTask
		}
		var err error
		rows, err = q.ListTaskCompletions(ctx, arg)
		if err != nil {
			return fmt.Errorf("list completions: %w", err)
		}
		return nil
	})
	if err != nil {
		return CompletionPage{}, err
	}
	page := CompletionPage{Items: []Completion{}}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		c := encodeCursor(last.DoneAt, last.ID)
		page.NextCursor = &c
	}
	for _, row := range rows {
		page.Items = append(page.Items, completionOf(row.ID, row.TaskID, row.DoneAt, row.Outcome, row.Note, row.CreatedAt, row.DoneByDisplayName, row.DoneByUsername))
	}
	return page, nil
}

func encodeCursor(doneAt string, id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(doneAt + "|" + strconv.FormatInt(id, 10)))
}

func decodeCursor(c string) (doneAt string, id int64, err error) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return "", 0, err
	}
	at, idText, ok := strings.Cut(string(raw), "|")
	if !ok {
		return "", 0, errors.New("malformed cursor")
	}
	if _, err := db.ParseTime(at); err != nil {
		return "", 0, err
	}
	id, err = strconv.ParseInt(idText, 10, 64)
	if err != nil {
		return "", 0, err
	}
	return at, id, nil
}
