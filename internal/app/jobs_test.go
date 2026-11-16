package app

import (
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/settings"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

func TestSessionsCleanupJobRemovesExpiredSessionsOnTheFirstTickThenHourly(t *testing.T) {
	database := openTestDB(t)
	a, err := New(config.Config{}, testutil.Logger(t), database, settings.NewStore(database))
	if err != nil {
		t.Fatal(err)
	}
	if len(a.jobs) == 0 || a.jobs[0].Name != "sessions-cleanup" {
		t.Fatalf("jobs = %+v, want sessions-cleanup first", a.jobs)
	}
	job := a.jobs[0]
	testutil.Exec(t, database, `INSERT INTO users (id, username) VALUES (1, 'u')`)
	testutil.Exec(t, database, `INSERT INTO sessions (token_hash, user_id, stage, auth_method, expires_at) VALUES (x'01', 1, 'active', 'password', '2000-01-01T00:00:00Z')`)
	testutil.Exec(t, database, `INSERT INTO sessions (token_hash, user_id, stage, auth_method, expires_at) VALUES (x'02', 1, 'active', 'password', '2999-01-01T00:00:00Z')`)

	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	if due, err := job.Trigger.Due(t.Context(), now, time.Time{}); err != nil || !due {
		t.Errorf("first tick due = %v, %v; want true", due, err)
	}
	if due, _ := job.Trigger.Due(t.Context(), now.Add(59*time.Minute), now); due {
		t.Error("due again before an hour passed")
	}
	if due, _ := job.Trigger.Due(t.Context(), now.Add(time.Hour), now); !due {
		t.Error("not due after an hour")
	}
	if err := job.Run(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	if n := testutil.Count(t, database, `SELECT COUNT(*) FROM sessions WHERE token_hash = x'01'`); n != 0 {
		t.Errorf("expired sessions left = %d, want 0", n)
	}
	if n := testutil.Count(t, database, `SELECT COUNT(*) FROM sessions WHERE token_hash = x'02'`); n != 1 {
		t.Errorf("live sessions left = %d, want 1", n)
	}
}

func TestBuildingTheAppStartsNoJobs(t *testing.T) {
	database := openTestDB(t)
	testutil.Exec(t, database, `INSERT INTO users (id, username) VALUES (1, 'u')`)
	testutil.Exec(t, database, `INSERT INTO sessions (token_hash, user_id, stage, auth_method, expires_at) VALUES (x'01', 1, 'active', 'password', '2000-01-01T00:00:00Z')`)
	if _, err := New(config.Config{}, testutil.Logger(t), database, settings.NewStore(database)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if n := testutil.Count(t, database, `SELECT COUNT(*) FROM sessions`); n != 1 {
		t.Errorf("sessions after New = %d, want the expired one still there", n)
	}
}
