package notify

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
)

// eventTypes are the event types that can be raised and subscribed to, in the order the API lists them.
var eventTypes = []string{"maintenance_due", "tests_due", "out_of_range", "backup_failed"}

// severities are the allowed values of an event's severity.
var severities = []string{"info", "warning", "critical"}

// check reports why the event cannot be raised.
func (e Event) check() error {
	switch {
	case !slices.Contains(eventTypes, e.Type):
		return fmt.Errorf("%w: unknown type %q", ErrInvalidEvent, e.Type)
	case !slices.Contains(severities, e.Severity):
		return fmt.Errorf("%w: unknown severity %q", ErrInvalidEvent, e.Severity)
	case e.Title == "":
		return fmt.Errorf("%w: empty title", ErrInvalidEvent)
	case e.SourceKind == "" || e.Occurrence == "":
		return fmt.Errorf("%w: no source or occurrence to deduplicate on", ErrInvalidEvent)
	}
	return nil
}

// Raise stores the notification and queues a delivery to every enabled channel subscribed to its type, all in one transaction; it reports false when this occurrence was already raised.
func (s *Service) Raise(ctx context.Context, e Event) (bool, error) {
	if err := e.check(); err != nil {
		return false, err
	}
	now := db.FormatTime(s.now())
	raised := false
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		id, err := q.InsertNotification(ctx, dbgen.InsertNotificationParams{
			EventType: e.Type, Severity: e.Severity, Title: e.Title, Body: e.Body, Link: e.Link, TankID: e.TankID,
			SourceKind: e.SourceKind, SourceID: e.SourceID, Occurrence: e.Occurrence, CreatedAt: now,
		})
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("store notification: %w", err)
		}
		if _, err := q.QueueDeliveries(ctx, dbgen.QueueDeliveriesParams{NotificationID: id, DueAt: now, CreatedAt: now, EventType: e.Type}); err != nil {
			return fmt.Errorf("queue deliveries: %w", err)
		}
		raised = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return raised, nil
}
