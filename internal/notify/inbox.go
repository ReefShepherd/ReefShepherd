package notify

import (
	"context"
	"fmt"
	"strconv"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

const (
	defaultPageSize = 50
	maxPageSize     = 200
	// maxReadIDs bounds one mark-as-read request.
	maxReadIDs = 1000
)

// Notification is an inbox entry as the API shows it, with the read state of the user asking.
type Notification struct {
	ID        int64  `json:"id"`
	EventType string `json:"event_type"`
	Severity  string `json:"severity"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Link      string `json:"link"`
	TankID    *int64 `json:"tank_id"`
	CreatedAt string `json:"created_at"`
	Read      bool   `json:"read"`
}

// Inbox returns one page of the notifications the user can see, newest first; beforeID is the cursor of the previous page, 0 for the first.
func (s *Service) Inbox(ctx context.Context, userID int64, unreadOnly bool, beforeID int64, limit int) (httpx.Page[Notification], error) {
	rows, err := db.Q(s.DB.R).ListInbox(ctx, dbgen.ListInboxParams{
		UserID: userID, UnreadOnly: db.Flag(unreadOnly), BeforeID: beforeID, RowLimit: int64(limit) + 1,
	})
	if err != nil {
		return httpx.Page[Notification]{}, fmt.Errorf("list inbox: %w", err)
	}
	var page httpx.Page[Notification]
	if len(rows) > limit {
		rows = rows[:limit]
		cursor := strconv.FormatInt(rows[limit-1].ID, 10)
		page.NextCursor = &cursor
	}
	page.Items = make([]Notification, len(rows))
	for i, r := range rows {
		page.Items[i] = Notification{
			ID: r.ID, EventType: r.EventType, Severity: r.Severity, Title: r.Title, Body: r.Body, Link: r.Link,
			TankID: r.TankID, CreatedAt: r.CreatedAt, Read: r.IsRead == 1,
		}
	}
	return page, nil
}

// UnreadCount is how many notifications the user can see and has not read.
func (s *Service) UnreadCount(ctx context.Context, userID int64) (int64, error) {
	n, err := db.Q(s.DB.R).CountUnread(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("count unread: %w", err)
	}
	return n, nil
}

// MarkRead marks the notifications with the given ids as read by the user; ids the user cannot see are ignored.
func (s *Service) MarkRead(ctx context.Context, userID int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	err := db.Q(s.DB.W).MarkRead(ctx, dbgen.MarkReadParams{ReadAt: db.FormatTime(s.now()), UserID: userID, Ids: ids})
	if err != nil {
		return fmt.Errorf("mark read: %w", err)
	}
	return nil
}

// MarkAllRead marks every notification the user can see as read by them.
func (s *Service) MarkAllRead(ctx context.Context, userID int64) error {
	err := db.Q(s.DB.W).MarkAllRead(ctx, dbgen.MarkAllReadParams{ReadAt: db.FormatTime(s.now()), UserID: userID})
	if err != nil {
		return fmt.Errorf("mark all read: %w", err)
	}
	return nil
}
