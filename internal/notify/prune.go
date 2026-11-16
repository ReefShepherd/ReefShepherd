package notify

import (
	"context"
	"fmt"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
)

// retention is how long notifications are kept; their read marks and deliveries go with them.
const retention = 90 * 24 * time.Hour

// Prune deletes the notifications that are older than the retention period at now.
func (s *Service) Prune(ctx context.Context, now time.Time) error {
	if _, err := db.Q(s.DB.W).PruneNotifications(ctx, db.FormatTime(now.Add(-retention))); err != nil {
		return fmt.Errorf("prune notifications: %w", err)
	}
	return nil
}
