package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/auth/password"
	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/scheduler"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
	"github.com/reefshepherd/reefshepherd/internal/webui"
)

// webhookReceiver records the bodies of the requests it gets.
type webhookReceiver struct {
	*httptest.Server
	mu     sync.Mutex
	bodies [][]byte
}

func newWebhookReceiver(t *testing.T) *webhookReceiver {
	t.Helper()
	r := &webhookReceiver{}
	r.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.bodies = append(r.bodies, body)
		r.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(r.Close)
	return r
}

func (r *webhookReceiver) received() [][]byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([][]byte(nil), r.bodies...)
}

// tickTwice runs the app's jobs at now and again right after, because the reminder job and the dispatcher run side by side on a tick and the delivery of a fresh notification can only go out on the next one.
func tickTwice(t *testing.T, a *App, now time.Time) {
	t.Helper()
	sched := scheduler.New(testutil.Logger(t), a.jobs)
	sched.Tick(t.Context(), now)
	sched.Tick(t.Context(), now.Add(time.Second))
}

// TestDueMaintenanceReachesTheInboxAndAWebhookOnceAcrossRestarts drives the whole path through the assembled app: a due task, scheduler ticks, the inbox, a webhook receiver, and then a second app on the same database.
func TestDueMaintenanceReachesTheInboxAndAWebhookOnceAcrossRestarts(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	a := newAPITestApp(t, cfg)
	admin := newClient(t, a)
	admin.setup(nil)
	tank := mustCreateTank(t, admin)
	receiver := newWebhookReceiver(t)

	rec := admin.do(http.MethodPost, "/api/v1/notification-channels", map[string]any{
		"name": "Receiver", "kind": "webhook", "config": map[string]any{"url": receiver.URL}, "events": []string{"maintenance_due"},
	})
	testutil.ExpectStatus(t, rec, http.StatusCreated)

	// Last done ten days ago and due again after seven, so the task has been due for three days.
	rec = admin.do(http.MethodPost, "/api/v1/maintenance/tasks", map[string]any{
		"name": "Water change", "tank_id": tank.ID, "schedule": map[string]any{"kind": "after_last", "interval_days": 7},
		"last_done_on": time.Now().UTC().AddDate(0, 0, -10).Format(time.DateOnly),
	})
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	// A task that is not due must stay quiet.
	testutil.ExpectStatus(t, admin.do(http.MethodPost, "/api/v1/maintenance/tasks", map[string]any{
		"name": "Clean skimmer", "tank_id": tank.ID, "schedule": map[string]any{"kind": "after_last", "interval_days": 30},
		"last_done_on": time.Now().UTC().AddDate(0, 0, -1).Format(time.DateOnly),
	}), http.StatusCreated)

	// The delivery is queued with the real clock, so the injected tick time lies a little ahead of it.
	tick := time.Now().UTC().Add(time.Hour)
	tickTwice(t, a.App, tick)

	type inboxItem struct {
		EventType string `json:"event_type"`
		Severity  string `json:"severity"`
		Title     string `json:"title"`
		Link      string `json:"link"`
		TankID    *int64 `json:"tank_id"`
		Read      bool   `json:"read"`
	}
	inbox := func() []inboxItem {
		t.Helper()
		rec := admin.do(http.MethodGet, "/api/v1/notifications", nil)
		testutil.ExpectStatus(t, rec, http.StatusOK)
		return testutil.DecodeItems[inboxItem](t, rec)
	}
	items := inbox()
	if len(items) != 1 {
		t.Fatalf("inbox = %+v, want one item", items)
	}
	wantLink := "/tanks/" + strconv.FormatInt(tank.ID, 10) + "/maintenance"
	if got := items[0]; got.EventType != "maintenance_due" || got.Severity != "info" || got.Title != "Water change is due on Reef" ||
		got.Link != wantLink || got.TankID == nil || *got.TankID != tank.ID || got.Read {
		t.Errorf("inbox item = %+v", got)
	}
	rec = admin.do(http.MethodGet, "/api/v1/notifications/unread-count", nil)
	testutil.ExpectStatus(t, rec, http.StatusOK)
	if count := testutil.Decode[struct {
		Count int `json:"count"`
	}](t, rec).Count; count != 1 {
		t.Errorf("unread count = %d, want 1", count)
	}

	requests := receiver.received()
	if len(requests) != 1 {
		t.Fatalf("webhook requests = %d, want 1", len(requests))
	}
	var payload struct {
		Version int    `json:"version"`
		Event   string `json:"event"`
		Title   string `json:"title"`
		Tank    *struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"tank"`
	}
	if err := json.Unmarshal(requests[0], &payload); err != nil {
		t.Fatalf("webhook body %s: %v", requests[0], err)
	}
	if payload.Version != 1 || payload.Event != "maintenance_due" || payload.Title != "Water change is due on Reef" || payload.Tank == nil || payload.Tank.ID != tank.ID {
		t.Errorf("webhook payload = %s", requests[0])
	}
	if n := testutil.Count(t, a.db, `SELECT COUNT(*) FROM notification_deliveries WHERE status = 'sent'`); n != 1 {
		t.Errorf("sent deliveries = %d, want 1", n)
	}

	// A restart builds a new app and scheduler on the same database; the task is still due, yet nothing is raised or sent again.
	second, err := newApp(cfg, testutil.Logger(t), a.db, a.store, password.NewHasher(password.Params{Time: 1, MemoryKiB: 64, Threads: 1}), webui.Handler(testutil.Logger(t)))
	if err != nil {
		t.Fatal(err)
	}
	rec = admin.do(http.MethodGet, "/api/v1/maintenance/tasks", nil)
	testutil.ExpectStatus(t, rec, http.StatusOK)
	if due := testutil.DecodeItems[struct{}](t, rec); len(due) != 1 {
		t.Fatalf("due tasks before the second tick = %d, want 1", len(due))
	}
	tickTwice(t, second, tick.Add(time.Minute))

	if items := inbox(); len(items) != 1 {
		t.Errorf("inbox after the second app's tick = %+v, want still one item", items)
	}
	if n := testutil.Count(t, a.db, `SELECT COUNT(*) FROM notifications WHERE event_type = 'maintenance_due'`); n != 1 {
		t.Errorf("maintenance_due notifications = %d, want 1", n)
	}
	if n := testutil.Count(t, a.db, `SELECT COUNT(*) FROM notification_deliveries`); n != 1 {
		t.Errorf("deliveries = %d, want 1", n)
	}
	if got := len(receiver.received()); got != 1 {
		t.Errorf("webhook requests after the second app's tick = %d, want 1", got)
	}
}
