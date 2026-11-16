package maintenance

import (
	"encoding/base64"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

func (e *env) completions(taskID int64, query string) httpxPage {
	e.t.Helper()
	var page httpxPage
	testutil.DecodeInto(e.t, e.must(http.StatusOK, http.MethodGet, taskPath(taskID, "/completions")+query, nil), &page)
	return page
}

type httpxPage struct {
	Items      []Completion `json:"items"`
	NextCursor *string      `json:"next_cursor"`
}

func TestCompleteDefaultsToDoneNowAndMovesTheDueDate(t *testing.T) {
	e := newEnv(t)
	task := e.newTask(map[string]any{"name": "a", "schedule": afterLast(7)})
	e.now = e.now.Add(3 * time.Hour)
	got := e.complete(task.ID, nil)
	c := got.Completion
	if c.ID == 0 || c.TaskID != task.ID || c.DoneAt != "2026-10-24T15:00:00Z" || c.Outcome != "done" || c.Note != "" || c.CreatedAt != c.DoneAt {
		t.Errorf("completion = %+v", c)
	}
	if c.DoneByName == nil || *c.DoneByName != "Tess" {
		t.Errorf("done_by_name = %v, want the display name of the user", c.DoneByName)
	}
	if !isStr(got.Task.LastDoneAt, c.DoneAt) || !isStr(got.Task.LastOutcome, "done") || !isStr(got.Task.NextDueAt, "2026-10-31T09:00:00Z") || got.Task.Overdue {
		t.Errorf("task after completion = %+v", got.Task)
	}
	if again := e.getTask(task.ID); !isStr(again.NextDueAt, "2026-10-31T09:00:00Z") {
		t.Errorf("stored task next_due_at = %v", again.NextDueAt)
	}
}

func TestCompleteAcceptsExplicitValues(t *testing.T) {
	e := newEnv(t)
	task := e.newTask(map[string]any{"name": "a", "schedule": afterLast(7)})
	got := e.complete(task.ID, map[string]any{"done_at": "2026-10-20T08:15:30.9+02:00", "outcome": "skipped", "note": "  no time "})
	if got.Completion.DoneAt != "2026-10-20T06:15:30Z" || got.Completion.Outcome != "skipped" || got.Completion.Note != "  no time " {
		t.Errorf("completion = %+v", got.Completion)
	}
	if !isStr(got.Task.NextDueAt, "2026-10-27T09:00:00Z") {
		t.Errorf("next_due_at = %v", got.Task.NextDueAt)
	}
	// A time a few minutes ahead is tolerated, a later one is not.
	e.complete(task.ID, map[string]any{"done_at": "2026-10-24T12:04:00Z"})
}

func TestCompleteValidatesFields(t *testing.T) {
	e := newEnv(t)
	task := e.newTask(map[string]any{"name": "a", "schedule": afterLast(7)})
	path := taskPath(task.ID, "/complete")
	e.invalid(http.MethodPost, path, map[string]any{"outcome": "maybe"}, "outcome=invalid")
	e.invalid(http.MethodPost, path, map[string]any{"outcome": ""}, "outcome=invalid")
	e.invalid(http.MethodPost, path, map[string]any{"done_at": "tomorrow"}, "done_at=invalid")
	e.invalid(http.MethodPost, path, map[string]any{"done_at": "2026-10-24"}, "done_at=invalid")
	e.invalid(http.MethodPost, path, map[string]any{"done_at": "2026-10-24T12:06:00Z"}, "done_at=out_of_range")
	e.invalid(http.MethodPost, path, map[string]any{"note": strings.Repeat("n", 2001)}, "note=too_long")
	e.invalid(http.MethodPost, path, map[string]any{"outcome": "x", "done_at": "x", "note": strings.Repeat("n", 2001)}, "outcome=invalid", "done_at=invalid", "note=too_long")
	e.fail(http.StatusNotFound, "not_found", http.MethodPost, taskPath(9999, "/complete"), map[string]any{})
	e.fail(http.StatusNotFound, "not_found", http.MethodPost, tasksPath+"/abc/complete", map[string]any{})
	if n := testutil.Count(t, e.db, `SELECT COUNT(*) FROM maintenance_completions`); n != 0 {
		t.Errorf("%d completions stored by refused requests", n)
	}
	// A body that is not JSON is refused like everywhere else.
	testutil.ExpectStatus(t, testutil.DoRaw(t, e.mux, http.MethodPost, path, "text/plain", "x", testutil.AsUser(e.userID, "admin")), http.StatusUnsupportedMediaType)
}

func TestSkippingCountsLikeDoingForTheNextDueDate(t *testing.T) {
	e := newEnv(t)
	e.now = at("2026-10-26T09:30:00Z") // Monday
	for _, sched := range []map[string]any{afterLast(7), weeklyOn(1)} {
		done := e.newTask(map[string]any{"name": "done", "schedule": sched})
		skipped := e.newTask(map[string]any{"name": "skipped", "schedule": sched})
		a := e.complete(done.ID, map[string]any{"outcome": "done"}).Task
		b := e.complete(skipped.ID, map[string]any{"outcome": "skipped"}).Task
		if a.NextDueAt == nil || !isStr(b.NextDueAt, *a.NextDueAt) {
			t.Errorf("%s: skipped next_due_at = %v, done = %v", sched["kind"], b.NextDueAt, a.NextDueAt)
		}
		if !isStr(b.LastOutcome, "skipped") || a.NextDueAt != nil && !isStr(a.NextDueAt, "2026-11-02T09:00:00Z") {
			t.Errorf("%s: outcome %v next %v, want skipped and the following Monday", sched["kind"], b.LastOutcome, a.NextDueAt)
		}
	}
	if got := e.completions(e.tasks("?status=all")[0].ID, "").Items; len(got) != 1 {
		t.Fatalf("history = %+v", got)
	}
}

func TestEditingAndDeletingACompletionMovesTheNextDueDate(t *testing.T) {
	e := newEnv(t)
	task := e.newTask(map[string]any{"name": "a", "schedule": afterLast(7)})
	older := e.complete(task.ID, map[string]any{"done_at": "2026-10-10T12:00:00Z"}).Completion
	newer := e.complete(task.ID, map[string]any{"done_at": "2026-10-20T12:00:00Z"}).Completion
	if got := e.getTask(task.ID); !isStr(got.NextDueAt, "2026-10-27T09:00:00Z") {
		t.Fatalf("next_due_at = %v", got.NextDueAt)
	}

	var edited Completion
	testutil.DecodeInto(t, e.must(http.StatusOK, http.MethodPatch, completionPath(newer.ID), map[string]any{"done_at": "2026-10-22T12:00:00Z"}), &edited)
	if edited.DoneAt != "2026-10-22T12:00:00Z" || edited.Outcome != "done" || edited.ID != newer.ID {
		t.Errorf("edited completion = %+v", edited)
	}
	if got := e.getTask(task.ID); !isStr(got.NextDueAt, "2026-10-29T09:00:00Z") {
		t.Errorf("next_due_at = %v after moving the completion later", got.NextDueAt)
	}

	testutil.DecodeInto(t, e.must(http.StatusOK, http.MethodPatch, completionPath(newer.ID), map[string]any{"outcome": "skipped", "note": "why"}), &edited)
	if edited.Outcome != "skipped" || edited.Note != "why" || edited.DoneAt != "2026-10-22T12:00:00Z" {
		t.Errorf("edited completion = %+v", edited)
	}
	if got := e.getTask(task.ID); !isStr(got.LastOutcome, "skipped") {
		t.Errorf("last_outcome = %v", got.LastOutcome)
	}

	e.must(http.StatusNoContent, http.MethodDelete, completionPath(newer.ID), nil)
	if got := e.getTask(task.ID); !isStr(got.NextDueAt, "2026-10-17T09:00:00Z") || !isStr(got.LastDoneAt, older.DoneAt) {
		t.Errorf("after deleting the newest: next_due_at = %v last_done_at = %v", got.NextDueAt, got.LastDoneAt)
	}
	e.must(http.StatusNoContent, http.MethodDelete, completionPath(older.ID), nil)
	got := e.getTask(task.ID)
	if got.LastDoneAt != nil || !isStr(got.NextDueAt, "2026-10-24T12:00:00Z") {
		t.Errorf("with no history: last_done_at = %v next_due_at = %v, want the schedule start", got.LastDoneAt, got.NextDueAt)
	}
	e.fail(http.StatusNotFound, "not_found", http.MethodDelete, completionPath(older.ID), nil)
	e.fail(http.StatusNotFound, "not_found", http.MethodDelete, "/api/v1/maintenance/completions/abc", nil)
}

func TestPatchCompletionValidatesFields(t *testing.T) {
	e := newEnv(t)
	task := e.newTask(map[string]any{"name": "a", "schedule": afterLast(7)})
	c := e.complete(task.ID, map[string]any{"note": "keep"}).Completion
	path := completionPath(c.ID)
	e.invalid(http.MethodPatch, path, map[string]any{"done_at": nil, "outcome": nil, "note": nil}, "done_at=required", "outcome=required", "note=required")
	e.invalid(http.MethodPatch, path, map[string]any{"done_at": "soon"}, "done_at=invalid")
	e.invalid(http.MethodPatch, path, map[string]any{"done_at": "2026-10-25T12:00:00Z"}, "done_at=out_of_range")
	e.invalid(http.MethodPatch, path, map[string]any{"outcome": "half"}, "outcome=invalid")
	e.invalid(http.MethodPatch, path, map[string]any{"note": strings.Repeat("n", 2001)}, "note=too_long")
	e.fail(http.StatusNotFound, "not_found", http.MethodPatch, completionPath(9999), map[string]any{"note": "x"})
	e.fail(http.StatusNotFound, "not_found", http.MethodPatch, "/api/v1/maintenance/completions/abc", map[string]any{"note": "x"})
	if got := e.completions(task.ID, "").Items[0]; got.Note != "keep" || got.DoneAt != c.DoneAt || got.Outcome != "done" {
		t.Errorf("refused edits changed the completion: %+v", got)
	}
}

func TestCompletionHistoryIsPagedNewestFirst(t *testing.T) {
	e := newEnv(t)
	task := e.newTask(map[string]any{"name": "a", "schedule": afterLast(7)})
	other := e.newTask(map[string]any{"name": "b", "schedule": afterLast(7)})
	e.complete(other.ID, nil)
	for _, day := range []string{"03", "01", "05", "05", "02", "04"} {
		e.complete(task.ID, map[string]any{"done_at": "2026-10-" + day + "T08:00:00Z", "note": day})
	}

	full := e.completions(task.ID, "")
	if len(full.Items) != 6 || full.NextCursor != nil {
		t.Fatalf("full page = %d items, cursor %v", len(full.Items), full.NextCursor)
	}
	var days []string
	for _, c := range full.Items {
		days = append(days, c.Note)
	}
	if want := []string{"05", "05", "04", "03", "02", "01"}; !slices.Equal(days, want) {
		t.Errorf("order = %v, want %v", days, want)
	}
	if full.Items[0].ID < full.Items[1].ID {
		t.Error("completions with the same time are not ordered by id, newest first")
	}

	for _, size := range []string{"1", "2", "4"} {
		var ids []int64
		query := "?limit=" + size
		pages := 0
		for {
			page := e.completions(task.ID, query)
			if pages++; pages > 10 {
				t.Fatalf("limit %s did not reach the last page after %d pages", size, pages)
			}
			for _, c := range page.Items {
				ids = append(ids, c.ID)
			}
			if page.NextCursor == nil {
				break
			}
			query = "?limit=" + size + "&cursor=" + *page.NextCursor
		}
		var want []int64
		for _, c := range full.Items {
			want = append(want, c.ID)
		}
		if !slices.Equal(ids, want) {
			t.Errorf("limit %s paged to %v, want %v", size, ids, want)
		}
		if size == "4" && pages != 2 {
			t.Errorf("limit 4 took %d pages, want 2", pages)
		}
	}

	path := taskPath(task.ID, "/completions")
	for _, limit := range []string{"0", "201", "abc", "-1"} {
		e.invalid(http.MethodGet, path+"?limit="+limit, nil, "limit=out_of_range")
	}
	e.invalid(http.MethodGet, path+"?cursor=%21%21", nil, "cursor=invalid")
	e.invalid(http.MethodGet, path+"?cursor="+base64.RawURLEncoding.EncodeToString([]byte("no separator")), nil, "cursor=invalid")
	e.invalid(http.MethodGet, path+"?cursor="+base64.RawURLEncoding.EncodeToString([]byte("yesterday|3")), nil, "cursor=invalid")
	e.invalid(http.MethodGet, path+"?cursor="+base64.RawURLEncoding.EncodeToString([]byte("2026-10-01T00:00:00Z|x")), nil, "cursor=invalid")
	e.must(http.StatusOK, http.MethodGet, path+"?limit=200", nil)
	e.fail(http.StatusNotFound, "not_found", http.MethodGet, taskPath(9999, "/completions"), nil)
	e.fail(http.StatusNotFound, "not_found", http.MethodGet, tasksPath+"/abc/completions", nil)
}

func TestCompletionOfADeletedUserHasNoName(t *testing.T) {
	e := newEnv(t)
	task := e.newTask(map[string]any{"name": "a", "schedule": afterLast(7)})
	e.complete(task.ID, nil)
	testutil.Exec(t, e.db, `DELETE FROM users WHERE id = ?`, e.userID)
	if got := e.completions(task.ID, "").Items[0]; got.DoneByName != nil {
		t.Errorf("done_by_name = %v, want null after the user was deleted", *got.DoneByName)
	}
}
