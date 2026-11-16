-- name: ListChannels :many
SELECT id, name, kind, config_json, enabled, created_at, updated_at
FROM notification_channels
ORDER BY id;

-- name: GetChannel :one
SELECT id, name, kind, config_json, enabled, created_at, updated_at
FROM notification_channels
WHERE id = ?;

-- name: CreateChannel :one
INSERT INTO notification_channels (name, kind, config_json, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING id, name, kind, config_json, enabled, created_at, updated_at;

-- name: UpdateChannel :one
UPDATE notification_channels
SET name = ?, config_json = ?, enabled = ?, updated_at = ?
WHERE id = ?
RETURNING id, name, kind, config_json, enabled, created_at, updated_at;

-- name: DeleteChannel :execrows
DELETE FROM notification_channels
WHERE id = ?;

-- name: ListSubscriptions :many
SELECT channel_id, event_type
FROM notification_subscriptions
ORDER BY channel_id, event_type;

-- name: AddSubscription :exec
INSERT INTO notification_subscriptions (channel_id, event_type)
VALUES (?, ?);

-- name: ClearSubscriptions :exec
DELETE FROM notification_subscriptions
WHERE channel_id = ?;

-- name: LastSentAt :one
SELECT sent_at
FROM notification_deliveries
WHERE channel_id = ? AND status = 'sent'
ORDER BY sent_at DESC, id DESC
LIMIT 1;

-- name: LastAttemptedDelivery :one
SELECT status, last_error, last_attempt_at
FROM notification_deliveries
WHERE channel_id = ? AND attempts > 0
ORDER BY id DESC
LIMIT 1;

-- name: InsertNotification :one
INSERT INTO notifications (event_type, severity, title, body, link, tank_id, source_kind, source_id, occurrence, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (source_kind, source_id, occurrence) DO NOTHING
RETURNING id;

-- name: QueueDeliveries :execrows
INSERT INTO notification_deliveries (notification_id, channel_id, status, attempts, next_attempt_at, created_at)
SELECT
    CAST(sqlc.arg(notification_id) AS INTEGER),
    c.id,
    'pending',
    0,
    CAST(sqlc.arg(due_at) AS TEXT),
    CAST(sqlc.arg(created_at) AS TEXT)
FROM notification_channels c
JOIN notification_subscriptions s ON s.channel_id = c.id
WHERE c.enabled = 1 AND s.event_type = sqlc.arg(event_type);

-- name: ListDueDeliveries :many
SELECT
    d.id AS delivery_id,
    d.attempts,
    n.event_type,
    n.severity,
    n.title,
    n.body,
    n.link,
    n.tank_id,
    n.created_at AS occurred_at,
    c.id AS channel_id,
    c.kind,
    c.config_json,
    c.enabled,
    t.name AS tank_name
FROM notification_deliveries d
JOIN notifications n ON n.id = d.notification_id
JOIN notification_channels c ON c.id = d.channel_id
LEFT JOIN tanks t ON t.id = n.tank_id
WHERE d.status = 'pending' AND d.next_attempt_at <= sqlc.arg(now)
ORDER BY d.next_attempt_at, d.id
LIMIT sqlc.arg(row_limit);

-- name: MarkDeliverySent :exec
UPDATE notification_deliveries
SET status = 'sent', attempts = sqlc.arg(attempts), last_error = '', next_attempt_at = NULL,
    sent_at = sqlc.arg(attempted_at), last_attempt_at = sqlc.arg(attempted_at)
WHERE id = sqlc.arg(id);

-- name: RecordDeliveryFailure :exec
UPDATE notification_deliveries
SET status = ?, attempts = ?, last_error = ?, next_attempt_at = ?, last_attempt_at = ?
WHERE id = ?;

-- name: PruneNotifications :execrows
DELETE FROM notifications
WHERE created_at < ?;

-- name: ListInbox :many
SELECT
    n.id,
    n.event_type,
    n.severity,
    n.title,
    n.body,
    n.link,
    n.tank_id,
    n.created_at,
    CAST(r.user_id IS NOT NULL AS INTEGER) AS is_read
FROM notifications n
JOIN users u ON u.id = sqlc.arg(user_id)
LEFT JOIN notification_reads r ON r.notification_id = n.id AND r.user_id = u.id
WHERE n.created_at >= u.created_at
    AND (CAST(sqlc.arg(unread_only) AS INTEGER) = 0 OR r.user_id IS NULL)
    AND (CAST(sqlc.arg(before_id) AS INTEGER) = 0 OR n.id < sqlc.arg(before_id))
ORDER BY n.id DESC
LIMIT sqlc.arg(row_limit);

-- name: CountUnread :one
SELECT COUNT(*)
FROM notifications n
JOIN users u ON u.id = ?
LEFT JOIN notification_reads r ON r.notification_id = n.id AND r.user_id = u.id
WHERE n.created_at >= u.created_at AND r.user_id IS NULL;

-- name: MarkAllRead :exec
INSERT INTO notification_reads (notification_id, user_id, read_at)
SELECT n.id, u.id, CAST(sqlc.arg(read_at) AS TEXT)
FROM notifications n
JOIN users u ON u.id = sqlc.arg(user_id)
WHERE n.created_at >= u.created_at
ON CONFLICT (notification_id, user_id) DO NOTHING;

-- name: MarkRead :exec
INSERT INTO notification_reads (notification_id, user_id, read_at)
SELECT n.id, u.id, CAST(sqlc.arg(read_at) AS TEXT)
FROM notifications n
JOIN users u ON u.id = sqlc.arg(user_id)
WHERE n.created_at >= u.created_at AND n.id IN (sqlc.slice('ids'))
ON CONFLICT (notification_id, user_id) DO NOTHING;
