-- name: ListTanks :many
SELECT
    id,
    name,
    tank_type,
    volume_liters,
    started_on,
    note,
    sort_order,
    archived_at,
    created_at,
    updated_at
FROM tanks
WHERE CAST(sqlc.arg(include_archived) AS INTEGER) = 1 OR archived_at IS NULL
ORDER BY archived_at IS NOT NULL, sort_order, id;

-- name: GetTank :one
SELECT
    id,
    name,
    tank_type,
    volume_liters,
    started_on,
    note,
    sort_order,
    archived_at,
    created_at,
    updated_at
FROM tanks
WHERE id = ?;

-- name: TankExists :one
SELECT COUNT(*)
FROM tanks
WHERE id = ?;

-- name: CreateTank :one
INSERT INTO tanks (name, tank_type, volume_liters, started_on, note, sort_order, created_at, updated_at)
VALUES (
    ?, ?, ?, ?, ?,
    (SELECT COALESCE(MAX(sort_order) + 1, 0) FROM tanks),
    ?, ?
)
RETURNING
    id,
    name,
    tank_type,
    volume_liters,
    started_on,
    note,
    sort_order,
    archived_at,
    created_at,
    updated_at;

-- name: UpdateTank :one
UPDATE tanks
SET
    name = ?,
    tank_type = ?,
    volume_liters = ?,
    started_on = ?,
    note = ?,
    sort_order = ?,
    updated_at = ?
WHERE id = ?
RETURNING
    id,
    name,
    tank_type,
    volume_liters,
    started_on,
    note,
    sort_order,
    archived_at,
    created_at,
    updated_at;

-- name: ArchiveTank :one
UPDATE tanks
SET
    archived_at = COALESCE(archived_at, ?),
    updated_at = ?
WHERE id = ?
RETURNING
    id,
    name,
    tank_type,
    volume_liters,
    started_on,
    note,
    sort_order,
    archived_at,
    created_at,
    updated_at;

-- name: UnarchiveTank :one
UPDATE tanks
SET
    archived_at = NULL,
    updated_at = ?
WHERE id = ?
RETURNING
    id,
    name,
    tank_type,
    volume_liters,
    started_on,
    note,
    sort_order,
    archived_at,
    created_at,
    updated_at;

-- name: DeleteTank :execrows
DELETE FROM tanks
WHERE id = ?;

-- name: ListActivePresetKeys :many
SELECT CAST(preset_key AS TEXT) AS preset_key
FROM parameter_definitions
WHERE preset_key IS NOT NULL AND archived_at IS NULL
ORDER BY sort_order, id;

-- name: EnablePresetParameters :execrows
INSERT INTO tank_parameters (
    tank_id, parameter_id, enabled, ideal_low, ideal_high, schedule_kind, interval_days, sort_order, schedule_since, updated_at
)
SELECT
    ?,
    id,
    1,
    default_low,
    default_high,
    CASE WHEN default_test_interval_days >= 1 THEN 'after_last' ELSE 'none' END,
    CASE WHEN default_test_interval_days >= 1 THEN default_test_interval_days END,
    sort_order,
    ?,
    ?
FROM parameter_definitions
WHERE archived_at IS NULL AND preset_key IN (sqlc.slice('preset_keys'));
