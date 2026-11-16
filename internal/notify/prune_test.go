package notify

import (
	"net/http"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

func TestPruneDeletesNotificationsOlderThan90DaysWithTheirReadsAndDeliveries(t *testing.T) {
	e := newCoreEnv(t)
	now := coreStart.Add(200 * 24 * time.Hour)
	e.addUser(1, "admin", coreStart.Add(-time.Hour))
	e.addChannel("main")

	raiseAt := func(age time.Duration, occurrence string) {
		e.clock = now.Add(-age)
		e.raise(coreEvent(occurrence))
	}
	const day = 24 * time.Hour
	raiseAt(90*day+time.Second, "too-old")
	raiseAt(90*day, "exactly-90-days")
	raiseAt(0, "fresh")
	e.clock = now

	rec := e.do(1, "admin", http.MethodPost, "/api/v1/notifications/read", map[string]any{"all": true})
	testutil.ExpectStatus(t, rec, http.StatusNoContent)
	if n := testutil.Count(t, e.db, `SELECT COUNT(*) FROM notification_reads`); n != 3 {
		t.Fatalf("reads before pruning = %d, want 3", n)
	}

	if err := e.svc.Prune(t.Context(), now); err != nil {
		t.Fatal(err)
	}

	var left []string
	rows, err := e.db.R.QueryContext(t.Context(), `SELECT occurrence FROM notifications ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var o string
		if err := rows.Scan(&o); err != nil {
			t.Fatal(err)
		}
		left = append(left, o)
	}
	if len(left) != 2 || left[0] != "exactly-90-days" || left[1] != "fresh" {
		t.Errorf("notifications left = %v, want the one exactly 90 days old and the fresh one", left)
	}
	if n := testutil.Count(t, e.db, `SELECT COUNT(*) FROM notification_reads`); n != 2 {
		t.Errorf("reads left = %d, want 2", n)
	}
	if n := testutil.Count(t, e.db, `SELECT COUNT(*) FROM notification_deliveries`); n != 2 {
		t.Errorf("deliveries left = %d, want 2", n)
	}
}

func TestPruneJobPrunes(t *testing.T) {
	e := newCoreEnv(t)
	e.raise(coreEvent("old"))
	now := coreStart.Add(91 * 24 * time.Hour)

	if err := Jobs(e.svc.Deps)[1].Run(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	if n := testutil.Count(t, e.db, `SELECT COUNT(*) FROM notifications`); n != 0 {
		t.Errorf("notifications left = %d, want 0", n)
	}
}
