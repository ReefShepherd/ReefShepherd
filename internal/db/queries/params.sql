-- name: ListParameterDefinitions :many
SELECT
    id,
    preset_key,
    name,
    unit_family,
    unit_label,
    decimals,
    default_low,
    default_high,
    default_test_interval_days,
    sort_order,
    archived_at,
    created_at,
    updated_at
FROM parameter_definitions
WHERE CAST(sqlc.arg(include_archived) AS INTEGER) = 1 OR archived_at IS NULL
ORDER BY archived_at IS NOT NULL, sort_order, id;

-- name: GetParameterDefinition :one
SELECT
    id,
    preset_key,
    name,
    unit_family,
    unit_label,
    decimals,
    default_low,
    default_high,
    default_test_interval_days,
    sort_order,
    archived_at,
    created_at,
    updated_at
FROM parameter_definitions
WHERE id = ?;

-- name: CountParametersNamed :one
SELECT COUNT(*)
FROM parameter_definitions
WHERE name = ? COLLATE NOCASE AND id <> ?;

-- name: CreateCustomParameter :one
INSERT INTO parameter_definitions (
    name, unit_family, unit_label, decimals, default_low, default_high, default_test_interval_days, sort_order, created_at, updated_at
)
VALUES (
    ?, 'fixed', ?, ?, ?, ?, ?,
    (SELECT COALESCE(MAX(sort_order) + 1, 0) FROM parameter_definitions),
    ?, ?
)
RETURNING id;

-- name: UpdateParameterDefinition :execrows
UPDATE parameter_definitions
SET
    name = ?,
    unit_label = ?,
    decimals = ?,
    default_low = ?,
    default_high = ?,
    default_test_interval_days = ?,
    updated_at = ?
WHERE id = ?;

-- name: SetParameterArchived :execrows
UPDATE parameter_definitions
SET
    archived_at = ?,
    updated_at = ?
WHERE id = ?;

-- name: DeleteCustomParameter :execrows
DELETE FROM parameter_definitions
WHERE parameter_definitions.id = ?
    AND parameter_definitions.preset_key IS NULL
    AND parameter_definitions.id NOT IN (SELECT test_results.parameter_id FROM test_results);

-- Every definition with the tank's settings (NULL columns when the tank has no row for it) and the newest result, in one statement.
-- name: ListTankParameters :many
SELECT
    d.id AS parameter_id,
    d.preset_key,
    d.name,
    d.unit_family,
    d.unit_label,
    d.decimals,
    d.default_low,
    d.default_high,
    d.default_test_interval_days,
    d.sort_order AS default_sort_order,
    d.archived_at,
    tp.enabled,
    tp.ideal_low,
    tp.ideal_high,
    tp.schedule_kind,
    tp.interval_days,
    tp.weekdays,
    tp.interval_weeks,
    tp.month_day,
    tp.anchor_date,
    tp.due_time,
    tp.sort_order AS tank_sort_order,
    tp.schedule_since,
    r.id AS latest_result_id,
    r.value AS latest_value,
    r.measured_at AS latest_measured_at
FROM parameter_definitions AS d
LEFT JOIN tank_parameters AS tp
    ON tp.parameter_id = d.id AND tp.tank_id = sqlc.arg(tank_id)
LEFT JOIN test_results AS r
    ON r.id = (
        SELECT l.id
        FROM test_results AS l
        WHERE l.tank_id = sqlc.arg(tank_id) AND l.parameter_id = d.id
        ORDER BY l.measured_at DESC, l.id DESC
        LIMIT 1
    )
WHERE (CAST(sqlc.narg(parameter_id) AS INTEGER) IS NULL OR d.id = sqlc.narg(parameter_id))
    AND (d.archived_at IS NULL OR CAST(sqlc.arg(include_archived) AS INTEGER) = 1)
ORDER BY COALESCE(tp.sort_order, d.sort_order), d.sort_order, d.id;

-- name: UpsertTankParameter :exec
INSERT INTO tank_parameters (
    tank_id, parameter_id, enabled, ideal_low, ideal_high,
    schedule_kind, interval_days, weekdays, interval_weeks, month_day, anchor_date, due_time,
    sort_order, schedule_since, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (tank_id, parameter_id) DO UPDATE
SET
    enabled = excluded.enabled,
    ideal_low = excluded.ideal_low,
    ideal_high = excluded.ideal_high,
    schedule_kind = excluded.schedule_kind,
    interval_days = excluded.interval_days,
    weekdays = excluded.weekdays,
    interval_weeks = excluded.interval_weeks,
    month_day = excluded.month_day,
    anchor_date = excluded.anchor_date,
    due_time = excluded.due_time,
    sort_order = excluded.sort_order,
    schedule_since = excluded.schedule_since,
    updated_at = excluded.updated_at;

-- name: InsertTestResult :one
INSERT INTO test_results (tank_id, parameter_id, measured_at, value, note, source, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: GetTestResult :one
SELECT id, tank_id, parameter_id, measured_at, value, note, source, created_by, created_at, updated_at
FROM test_results
WHERE id = ?;

-- name: UpdateTestResult :execrows
UPDATE test_results
SET
    measured_at = ?,
    value = ?,
    note = ?,
    updated_at = ?
WHERE id = ?;

-- name: DeleteTestResult :execrows
DELETE FROM test_results
WHERE id = ?;

-- name: ListTankResults :many
SELECT id, tank_id, parameter_id, measured_at, value, note, source, created_by, created_at, updated_at
FROM test_results
WHERE tank_id = sqlc.arg(tank_id)
    AND (CAST(sqlc.narg(parameter_id) AS INTEGER) IS NULL OR parameter_id = sqlc.narg(parameter_id))
    AND (CAST(sqlc.narg(from_at) AS TEXT) IS NULL OR measured_at >= sqlc.narg(from_at))
    AND (CAST(sqlc.narg(to_at) AS TEXT) IS NULL OR measured_at <= sqlc.narg(to_at))
    AND (
        CAST(sqlc.narg(cursor_at) AS TEXT) IS NULL
        OR measured_at < sqlc.narg(cursor_at)
        OR (measured_at = sqlc.narg(cursor_at) AND id < sqlc.narg(cursor_id))
    )
ORDER BY measured_at DESC, id DESC
LIMIT sqlc.arg(row_limit);

-- name: ListSeriesPoints :many
SELECT id, measured_at, value, note
FROM test_results
WHERE tank_id = sqlc.arg(tank_id)
    AND parameter_id = sqlc.arg(parameter_id)
    AND (CAST(sqlc.narg(from_at) AS TEXT) IS NULL OR measured_at >= sqlc.narg(from_at))
    AND (CAST(sqlc.narg(to_at) AS TEXT) IS NULL OR measured_at <= sqlc.narg(to_at))
ORDER BY measured_at, id;
