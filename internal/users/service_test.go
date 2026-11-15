package users

import (
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/auth/password"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

func TestCreateStampsTheRowFromTheServiceClock(t *testing.T) {
	database := testutil.OpenDB(t)
	logger := testutil.Logger(t)
	authSvc := auth.NewService(auth.Deps{
		DB: database, Passwords: password.NewHasher(password.Params{Time: 1, MemoryKiB: 8, Threads: 1}), Logger: logger,
	})
	svc := NewService(Deps{DB: database, Auth: authSvc, Logger: logger})
	at := time.Date(2031, 3, 4, 5, 6, 7, 0, time.UTC)
	svc.now = func() time.Time { return at }

	u, err := svc.Create(t.Context(), CreateInput{Username: "reefer", Role: "member", Password: "a long enough password"})
	if err != nil {
		t.Fatal(err)
	}
	stamp := db.FormatTime(at)
	for _, column := range []string{"created_at", "updated_at", "password_changed_at"} {
		if n := testutil.Count(t, database, `SELECT COUNT(*) FROM users WHERE id = ? AND `+column+` = ?`, u.ID, stamp); n != 1 {
			t.Errorf("%s is not %s", column, stamp)
		}
	}
	if n := testutil.Count(t, database, `SELECT COUNT(*) FROM users WHERE id = ? AND password_hash LIKE '$argon2id$%'`, u.ID); n != 1 {
		t.Error("the password was not stored")
	}
	if u.CreatedAt != stamp {
		t.Errorf("CreatedAt = %q, want %q", u.CreatedAt, stamp)
	}
}
