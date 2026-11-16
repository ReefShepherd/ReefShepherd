package notify

import (
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

func TestRaiseStoresTheNotificationAndARepeatedOccurrenceRaisesNothing(t *testing.T) {
	e := newCoreEnv(t)
	e.addChannel("main")
	tank := testutil.Insert(t, e.db, `INSERT INTO tanks (name) VALUES ('Display')`)

	ev := coreEvent("2026-03-01T09:00:00Z")
	ev.TankID = &tank
	if !e.raise(ev) {
		t.Fatal("first raise reported a duplicate")
	}

	var got struct {
		EventType, Severity, Title, Body, Link, SourceKind, Occurrence, CreatedAt string
		TankID                                                                    *int64
		SourceID                                                                  int64
	}
	row := e.db.R.QueryRowContext(t.Context(), `SELECT event_type, severity, title, body, link, tank_id, source_kind, source_id, occurrence, created_at FROM notifications`)
	if err := row.Scan(&got.EventType, &got.Severity, &got.Title, &got.Body, &got.Link, &got.TankID, &got.SourceKind, &got.SourceID, &got.Occurrence, &got.CreatedAt); err != nil {
		t.Fatal(err)
	}
	if got.EventType != "maintenance_due" || got.Severity != "warning" || got.Title != "Water change is due" || got.Body != "Main tank" ||
		got.Link != "/tanks/1/maintenance" || got.TankID == nil || *got.TankID != tank || got.SourceKind != "maintenance_task" ||
		got.SourceID != 7 || got.Occurrence != "2026-03-01T09:00:00Z" || got.CreatedAt != coreTime(coreStart) {
		t.Errorf("stored notification = %+v", got)
	}

	if e.raise(ev) {
		t.Error("the same occurrence was raised twice")
	}
	if n := testutil.Count(t, e.db, `SELECT COUNT(*) FROM notifications`); n != 1 {
		t.Errorf("notifications after a repeat = %d, want 1", n)
	}
	if n := testutil.Count(t, e.db, `SELECT COUNT(*) FROM notification_deliveries`); n != 1 {
		t.Errorf("deliveries after a repeat = %d, want 1", n)
	}

	next := ev
	next.Occurrence = "2026-03-02T09:00:00Z"
	if !e.raise(next) {
		t.Error("a new occurrence of the same source was refused")
	}
	other := ev
	other.SourceID = 8
	if !e.raise(other) {
		t.Error("the same occurrence of another source was refused")
	}
}

func TestRaiseQueuesDeliveriesOnlyForEnabledSubscribedChannels(t *testing.T) {
	e := newCoreEnv(t)
	both := e.addChannel("both")
	onlyDue := e.addChannel("only due", "maintenance_due")
	otherEvent := e.addChannel("other event", "tests_due", "backup_failed")
	off := e.addChannel("off")
	rec := e.admin(http.MethodPatch, "/api/v1/notification-channels/"+coreID(off.ID), map[string]any{"enabled": false})
	testutil.ExpectStatus(t, rec, http.StatusOK)

	e.raise(coreEvent("a"))

	var got []int64
	for _, d := range e.deliveries() {
		got = append(got, d.ChannelID)
		if d.Status != "pending" || d.Attempts != 0 || d.LastError != "" || d.SentAt != nil || d.NextAttemptAt == nil || *d.NextAttemptAt != coreTime(coreStart) {
			t.Errorf("new delivery = %+v, want pending, no attempts, due now", d)
		}
	}
	want := []int64{both.ID, onlyDue.ID}
	if !slices.Equal(got, want) {
		t.Errorf("channels with a delivery = %v, want %v (not %d subscribed to other events, not %d disabled)", got, want, otherEvent.ID, off.ID)
	}
}

func TestRaiseWithoutChannelsStillFillsTheInbox(t *testing.T) {
	e := newCoreEnv(t)
	if !e.raise(coreEvent("a")) {
		t.Error("raise reported a duplicate")
	}
	if n := testutil.Count(t, e.db, `SELECT COUNT(*) FROM notifications`); n != 1 {
		t.Errorf("notifications = %d, want 1", n)
	}
}

func TestRaiseRejectsAnEventThatCannotBeStored(t *testing.T) {
	e := newCoreEnv(t)
	missingTank := int64(99)
	cases := map[string]func(ev *Event){
		"unknown type":     func(ev *Event) { ev.Type = "nope" },
		"empty type":       func(ev *Event) { ev.Type = "" },
		"unknown severity": func(ev *Event) { ev.Severity = "loud" },
		"empty severity":   func(ev *Event) { ev.Severity = "" },
		"empty title":      func(ev *Event) { ev.Title = "" },
		"no source kind":   func(ev *Event) { ev.SourceKind = "" },
		"no occurrence":    func(ev *Event) { ev.Occurrence = "" },
	}
	for name, mutate := range cases {
		ev := coreEvent("a")
		mutate(&ev)
		if _, err := e.svc.Raise(t.Context(), ev); !errors.Is(err, ErrInvalidEvent) {
			t.Errorf("%s: err = %v, want ErrInvalidEvent", name, err)
		}
	}

	// A tank that does not exist fails the whole transaction, so no half-raised notification is left.
	ev := coreEvent("b")
	ev.TankID = &missingTank
	if _, err := e.svc.Raise(t.Context(), ev); err == nil {
		t.Error("an event for a missing tank was raised")
	}
	if n := testutil.Count(t, e.db, `SELECT COUNT(*) FROM notifications`); n != 0 {
		t.Errorf("notifications after the rejected events = %d, want 0", n)
	}
}
