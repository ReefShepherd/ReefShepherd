-- name: ListTestReminders :many
SELECT parameter_id, due_at
FROM test_reminders
WHERE tank_id = ?;

-- name: InsertTestReminder :exec
INSERT INTO test_reminders (tank_id, parameter_id, due_at)
VALUES (?, ?, ?)
ON CONFLICT (tank_id, parameter_id, due_at) DO NOTHING;
