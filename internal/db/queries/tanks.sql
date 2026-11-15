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
