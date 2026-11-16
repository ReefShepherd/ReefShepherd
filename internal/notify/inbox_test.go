package notify

import (
	"net/http"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

const inboxPath = "/api/v1/notifications"

type coreInboxPage struct {
	Items      []Notification `json:"items"`
	NextCursor *string        `json:"next_cursor"`
}

func (e *coreEnv) inbox(userID int64, query string) coreInboxPage {
	e.t.Helper()
	rec := e.do(userID, "member", http.MethodGet, inboxPath+query, nil)
	testutil.ExpectStatus(e.t, rec, http.StatusOK)
	return testutil.Decode[coreInboxPage](e.t, rec)
}

func (e *coreEnv) unread(userID int64) int64 {
	e.t.Helper()
	rec := e.do(userID, "member", http.MethodGet, inboxPath+"/unread-count", nil)
	testutil.ExpectStatus(e.t, rec, http.StatusOK)
	return testutil.Decode[struct {
		Count int64 `json:"count"`
	}](e.t, rec).Count
}

func (e *coreEnv) markRead(userID int64, body any) {
	e.t.Helper()
	testutil.ExpectStatus(e.t, e.do(userID, "member", http.MethodPost, inboxPath+"/read", body), http.StatusNoContent)
}

// raiseN raises n notifications, titled n1 to nN, one second apart from the environment's clock.
func (e *coreEnv) raiseN(n int) {
	e.t.Helper()
	for i := 1; i <= n; i++ {
		ev := coreEvent("o" + strconv.Itoa(i))
		ev.Title = "n" + strconv.Itoa(i)
		e.raise(ev)
		e.clock = e.clock.Add(time.Second)
	}
}

func coreTitles(items []Notification) []string {
	out := make([]string, len(items))
	for i, n := range items {
		out[i] = n.Title
	}
	return out
}

func TestInboxPagesNewestFirstWithACursor(t *testing.T) {
	e := newCoreEnv(t)
	e.addUser(1, "member", coreStart.Add(-time.Hour))
	e.raiseN(5)

	page := e.inbox(1, "?limit=2")
	if !slices.Equal(coreTitles(page.Items), []string{"n5", "n4"}) || page.NextCursor == nil {
		t.Fatalf("first page = %v, cursor %v", coreTitles(page.Items), page.NextCursor)
	}
	page = e.inbox(1, "?limit=2&cursor="+*page.NextCursor)
	if !slices.Equal(coreTitles(page.Items), []string{"n3", "n2"}) || page.NextCursor == nil {
		t.Fatalf("second page = %v, cursor %v", coreTitles(page.Items), page.NextCursor)
	}
	page = e.inbox(1, "?limit=2&cursor="+*page.NextCursor)
	if !slices.Equal(coreTitles(page.Items), []string{"n1"}) || page.NextCursor != nil {
		t.Fatalf("last page = %v, cursor %v, want one item and no cursor", coreTitles(page.Items), page.NextCursor)
	}

	// A page that ends exactly at the last item has no cursor either.
	if page := e.inbox(1, "?limit=5"); len(page.Items) != 5 || page.NextCursor != nil {
		t.Errorf("exact page = %d items, cursor %v", len(page.Items), page.NextCursor)
	}
}

func TestInboxShowsTheNotificationFieldsAndAnEmptyInboxIsAnEmptyList(t *testing.T) {
	e := newCoreEnv(t)
	e.addUser(1, "member", coreStart.Add(-time.Hour))
	if page := e.inbox(1, ""); page.Items == nil || len(page.Items) != 0 || page.NextCursor != nil {
		t.Errorf("empty inbox = %+v", page)
	}

	tank := testutil.Insert(t, e.db, `INSERT INTO tanks (name) VALUES ('Display')`)
	ev := coreEvent("a")
	ev.TankID = &tank
	e.raise(ev)

	rec := e.do(1, "member", http.MethodGet, inboxPath, nil)
	items := testutil.DecodeItems[map[string]any](t, rec)
	want := map[string]any{
		"event_type": "maintenance_due", "severity": "warning", "title": "Water change is due", "body": "Main tank",
		"link": "/tanks/1/maintenance", "tank_id": float64(tank), "created_at": coreTime(coreStart), "read": false,
	}
	if len(items) != 1 {
		t.Fatalf("items = %v", items)
	}
	for k, v := range want {
		if items[0][k] != v {
			t.Errorf("%s = %v, want %v", k, items[0][k], v)
		}
	}
	if len(items[0]) != len(want)+1 {
		t.Errorf("fields = %v, want the id plus %d others", items[0], len(want))
	}
}

func TestInboxFiltersToUnreadAndReadByIDsOrAll(t *testing.T) {
	e := newCoreEnv(t)
	e.addUser(1, "member", coreStart.Add(-time.Hour))
	e.addUser(2, "member", coreStart.Add(-time.Hour))
	e.raiseN(4)

	if got := e.unread(1); got != 4 {
		t.Fatalf("unread = %d, want 4", got)
	}
	e.markRead(1, map[string]any{"ids": []int64{4, 2}})
	if got := e.unread(1); got != 2 {
		t.Errorf("unread after reading two = %d, want 2", got)
	}
	if page := e.inbox(1, "?unread=true"); !slices.Equal(coreTitles(page.Items), []string{"n3", "n1"}) {
		t.Errorf("unread list = %v, want n3 n1", coreTitles(page.Items))
	}
	all := e.inbox(1, "?unread=false")
	reads := map[string]bool{}
	for _, n := range all.Items {
		reads[n.Title] = n.Read
	}
	if len(all.Items) != 4 || !reads["n4"] || !reads["n2"] || reads["n3"] || reads["n1"] {
		t.Errorf("read flags = %v", reads)
	}

	// The unread filter pages like the full list.
	page := e.inbox(1, "?unread=true&limit=1")
	if !slices.Equal(coreTitles(page.Items), []string{"n3"}) || page.NextCursor == nil {
		t.Fatalf("unread page = %v cursor %v", coreTitles(page.Items), page.NextCursor)
	}
	if next := e.inbox(1, "?unread=true&limit=1&cursor="+*page.NextCursor); !slices.Equal(coreTitles(next.Items), []string{"n1"}) || next.NextCursor != nil {
		t.Errorf("unread page 2 = %v cursor %v", coreTitles(next.Items), next.NextCursor)
	}

	// Reading is per user, and reading twice changes nothing.
	if got := e.unread(2); got != 4 {
		t.Errorf("the other user's unread = %d, want 4", got)
	}
	e.markRead(1, map[string]any{"ids": []int64{4}})
	e.markRead(1, map[string]any{"all": true})
	if got := e.unread(1); got != 0 {
		t.Errorf("unread after all = %d, want 0", got)
	}
	if page := e.inbox(1, "?unread=true"); len(page.Items) != 0 {
		t.Errorf("unread list after all = %v", coreTitles(page.Items))
	}
	if got := e.unread(2); got != 4 {
		t.Errorf("the other user's unread after all = %d, want 4", got)
	}
}

func TestInboxStartsAtTheUsersAccountCreation(t *testing.T) {
	e := newCoreEnv(t)
	joined := coreStart.Add(10 * time.Second)
	e.addUser(1, "member", coreStart.Add(-time.Hour))
	e.addUser(2, "member", joined)
	// Raised at +0s, +10s (the moment user 2 joined) and +20s.
	for i, offset := range []time.Duration{0, 10 * time.Second, 20 * time.Second} {
		e.clock = coreStart.Add(offset)
		ev := coreEvent("o" + strconv.Itoa(i))
		ev.Title = "n" + strconv.Itoa(i)
		e.raise(ev)
	}

	if page := e.inbox(1, ""); len(page.Items) != 3 {
		t.Errorf("the older user sees %v, want all three", coreTitles(page.Items))
	}
	if page := e.inbox(2, ""); !slices.Equal(coreTitles(page.Items), []string{"n2", "n1"}) {
		t.Errorf("the new user sees %v, want only n2 n1", coreTitles(page.Items))
	}
	if got := e.unread(2); got != 2 {
		t.Errorf("the new user's unread = %d, want 2", got)
	}

	// Reading by id cannot reach a notification from before the account, and reading all skips it too.
	var before int64
	if err := e.db.R.QueryRowContext(t.Context(), `SELECT id FROM notifications WHERE title = 'n0'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	e.markRead(2, map[string]any{"ids": []int64{before}})
	e.markRead(2, map[string]any{"all": true})
	if n := testutil.Count(t, e.db, `SELECT COUNT(*) FROM notification_reads WHERE user_id = 2 AND notification_id = ?`, before); n != 0 {
		t.Errorf("a notification from before the account was marked read")
	}
	if n := testutil.Count(t, e.db, `SELECT COUNT(*) FROM notification_reads WHERE user_id = 2`); n != 2 {
		t.Errorf("reads of the new user = %d, want 2", n)
	}
}

func TestInboxRejectsBadQueries(t *testing.T) {
	e := newCoreEnv(t)
	e.addUser(1, "member", coreStart)
	cases := []struct{ query, field, code string }{
		{"?limit=0", "limit", "out_of_range"},
		{"?limit=201", "limit", "out_of_range"},
		{"?limit=-1", "limit", "out_of_range"},
		{"?limit=many", "limit", "invalid"},
		{"?cursor=abc", "cursor", "invalid"},
		{"?cursor=0", "cursor", "invalid"},
		{"?unread=maybe", "unread", "invalid"},
	}
	for _, tc := range cases {
		rec := e.do(1, "member", http.MethodGet, inboxPath+tc.query, nil)
		body := testutil.ExpectError(t, rec, http.StatusUnprocessableEntity, "validation", tc.field)
		if body.Error.Fields[tc.field] != tc.code {
			t.Errorf("%s: %s = %q, want %q", tc.query, tc.field, body.Error.Fields[tc.field], tc.code)
		}
	}
	if page := e.inbox(1, "?limit=200"); page.Items == nil {
		t.Error("limit 200 refused")
	}
}

func TestMarkReadRejectsBadBodies(t *testing.T) {
	e := newCoreEnv(t)
	e.addUser(1, "member", coreStart.Add(-time.Hour))
	e.raiseN(2)
	tooMany := make([]int64, maxReadIDs+1)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	cases := []struct {
		name string
		body any
		code string
	}{
		{"empty body object", map[string]any{}, "required"},
		{"no ids", map[string]any{"ids": []int64{}}, "required"},
		{"all false and no ids", map[string]any{"all": false}, "required"},
		{"ids and all", map[string]any{"ids": []int64{1}, "all": true}, "invalid"},
		{"id zero", map[string]any{"ids": []int64{1, 0}}, "invalid"},
		{"negative id", map[string]any{"ids": []int64{-3}}, "invalid"},
		{"too many ids", map[string]any{"ids": tooMany}, "too_long"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := e.do(1, "member", http.MethodPost, inboxPath+"/read", tc.body)
			body := testutil.ExpectError(t, rec, http.StatusUnprocessableEntity, "validation", "ids")
			if body.Error.Fields["ids"] != tc.code {
				t.Errorf("ids = %q, want %q", body.Error.Fields["ids"], tc.code)
			}
		})
	}
	testutil.ExpectError(t, e.do(1, "member", http.MethodPost, inboxPath+"/read", map[string]any{"everything": true}), http.StatusUnprocessableEntity, "validation", "everything")

	// Ids that match nothing are ignored.
	e.markRead(1, map[string]any{"ids": []int64{999}})
	if got := e.unread(1); got != 2 {
		t.Errorf("unread after reading an unknown id = %d, want 2", got)
	}
}
