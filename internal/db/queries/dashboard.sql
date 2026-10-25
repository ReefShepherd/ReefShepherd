-- name: CountDashboardCards :one
SELECT COUNT(*)
FROM dashboard_cards
WHERE user_id = ? AND tank_id = ?;

-- Every card the user has for the tank, shown or not; the caller hides cards of parameters that are no longer enabled and active.
-- name: ListDashboardCards :many
SELECT
    id,
    card_type,
    parameter_id,
    size,
    options_json,
    created_at
FROM dashboard_cards
WHERE user_id = ? AND tank_id = ?
ORDER BY position, id;

-- name: ListEnabledTankParameterIDs :many
SELECT d.id
FROM tank_parameters AS tp
INNER JOIN parameter_definitions AS d ON d.id = tp.parameter_id
WHERE tp.tank_id = ? AND tp.enabled = 1 AND d.archived_at IS NULL
ORDER BY tp.sort_order, d.sort_order, d.id;

-- name: DeleteDashboardCards :execrows
DELETE FROM dashboard_cards
WHERE user_id = ? AND tank_id = ?;

-- name: InsertDashboardCard :exec
INSERT INTO dashboard_cards (user_id, tank_id, card_type, parameter_id, position, size, options_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);
