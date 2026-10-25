-- name: CountUsers :one
SELECT COUNT(*)
FROM users;

-- name: GetUserByID :one
SELECT
    u.id,
    u.username,
    u.display_name,
    u.email,
    u.role,
    u.disabled,
    CAST(u.password_hash IS NOT NULL AS INTEGER) AS has_password,
    CAST(t.confirmed_at IS NOT NULL AS INTEGER) AS totp_enabled,
    CAST(COALESCE(p.theme, 'system') AS TEXT) AS theme,
    CAST(COALESCE(p.locale, 'en') AS TEXT) AS locale,
    p.default_tank_id,
    u.created_at
FROM users AS u
LEFT JOIN user_totp AS t ON t.user_id = u.id
LEFT JOIN user_preferences AS p ON p.user_id = u.id
WHERE u.id = ?;

-- name: ListUsers :many
SELECT
    u.id,
    u.username,
    u.display_name,
    u.email,
    u.role,
    u.disabled,
    CAST(u.password_hash IS NOT NULL AS INTEGER) AS has_password,
    CAST(t.confirmed_at IS NOT NULL AS INTEGER) AS totp_enabled,
    u.created_at
FROM users AS u
LEFT JOIN user_totp AS t ON t.user_id = u.id
ORDER BY u.id;

-- name: GetUserCredentialsByUsername :one
SELECT id, username, display_name, password_hash, role, disabled
FROM users
WHERE username = ?;

-- name: GetUserCredentialsByID :one
SELECT id, username, display_name, password_hash, role, disabled
FROM users
WHERE id = ?;

-- name: UserExistsByUsername :one
SELECT COUNT(*)
FROM users
WHERE username = ?;

-- name: CreateUser :one
INSERT INTO users (username, display_name, email, password_hash, role, password_changed_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: UpdateUserAccount :exec
UPDATE users
SET display_name = ?, email = ?, role = ?, disabled = ?, updated_at = ?
WHERE id = ?;

-- name: UpdateUserProfile :exec
UPDATE users
SET display_name = ?, email = ?, updated_at = ?
WHERE id = ?;

-- name: SetUserPassword :exec
UPDATE users
SET password_hash = ?, password_changed_at = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteUser :execrows
DELETE FROM users
WHERE id = ?;

-- name: CountOtherEnabledAdmins :one
SELECT COUNT(*)
FROM users
WHERE role = 'admin' AND disabled = 0 AND id <> ?;

-- name: CountEnabledAdminsWithPassword :one
SELECT COUNT(*)
FROM users
WHERE role = 'admin' AND disabled = 0 AND password_hash IS NOT NULL;

-- name: FirstEnabledAdminID :one
SELECT id
FROM users
WHERE role = 'admin' AND disabled = 0
ORDER BY id
LIMIT 1;

-- name: UpsertPreferences :exec
INSERT INTO user_preferences (user_id, theme, locale, default_tank_id, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (user_id) DO UPDATE
SET theme = excluded.theme,
    locale = excluded.locale,
    default_tank_id = excluded.default_tank_id,
    updated_at = excluded.updated_at;

-- name: TankExists :one
SELECT COUNT(*)
FROM tanks
WHERE id = ?;

-- name: RehashPassword :execrows
UPDATE users
SET password_hash = ?
WHERE id = ? AND password_hash = ?;

-- name: FirstOtherEnabledAdminID :one
SELECT id
FROM users
WHERE role = 'admin' AND disabled = 0 AND id <> ?
ORDER BY id
LIMIT 1;
