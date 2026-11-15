-- name: CreateSession :one
INSERT INTO sessions (token_hash, user_id, stage, auth_method, user_agent, ip_address, created_at, last_seen_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: GetSessionByTokenHash :one
SELECT
    s.id AS session_id,
    s.user_id,
    s.stage,
    s.auth_method,
    s.last_seen_at,
    s.expires_at,
    u.username,
    u.display_name,
    u.role
FROM sessions AS s
INNER JOIN users AS u ON u.id = s.user_id
WHERE s.token_hash = ? AND s.expires_at > ? AND u.disabled = 0;

-- name: TouchSession :exec
UPDATE sessions
SET last_seen_at = ?, expires_at = ?
WHERE id = ?;

-- name: UpgradeSession :execrows
UPDATE sessions
SET token_hash = ?, stage = 'active', created_at = ?, last_seen_at = ?, expires_at = ?
WHERE id = ? AND stage = 'mfa_pending';

-- name: DeleteSessionByTokenHash :execrows
DELETE FROM sessions
WHERE token_hash = ?;

-- name: DeleteUserSession :execrows
DELETE FROM sessions
WHERE id = ? AND user_id = ?;

-- name: DeleteOtherUserSessions :execrows
DELETE FROM sessions
WHERE user_id = ? AND id <> ?;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions
WHERE expires_at <= ?;

-- name: ListUserSessions :many
SELECT id, auth_method, user_agent, ip_address, created_at, last_seen_at, expires_at
FROM sessions
WHERE user_id = ? AND stage = 'active' AND expires_at > ?
ORDER BY id;

-- name: GetTOTP :one
SELECT secret, confirmed_at, last_used_step
FROM user_totp
WHERE user_id = ?;

-- name: StartTOTPEnrollment :execrows
INSERT INTO user_totp (user_id, secret, created_at)
VALUES (?, ?, ?)
ON CONFLICT (user_id) DO UPDATE
SET secret = excluded.secret, last_used_step = 0, created_at = excluded.created_at
WHERE user_totp.confirmed_at IS NULL;

-- name: ConfirmTOTP :execrows
UPDATE user_totp
SET confirmed_at = ?, last_used_step = ?
WHERE user_id = ? AND confirmed_at IS NULL;

-- name: AdvanceTOTPStep :execrows
UPDATE user_totp
SET last_used_step = ?
WHERE user_id = ? AND confirmed_at IS NOT NULL AND last_used_step < ?;

-- name: DeleteTOTP :execrows
DELETE FROM user_totp
WHERE user_id = ?;

-- name: InsertRecoveryCode :exec
INSERT INTO user_recovery_codes (user_id, code_hash, created_at)
VALUES (?, ?, ?);

-- name: DeleteRecoveryCodes :exec
DELETE FROM user_recovery_codes
WHERE user_id = ?;

-- name: UseRecoveryCode :execrows
UPDATE user_recovery_codes
SET used_at = ?
WHERE user_id = ? AND code_hash = ? AND used_at IS NULL;
