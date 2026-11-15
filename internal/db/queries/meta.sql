-- name: GetSetting :one
SELECT value
FROM settings
WHERE key = ?;

-- name: SetSetting :exec
INSERT INTO settings (key, value)
VALUES (?, ?)
ON CONFLICT (key) DO UPDATE
SET value = excluded.value, updated_at = excluded.updated_at;

-- name: DeleteSetting :execrows
DELETE FROM settings
WHERE key = ?;
