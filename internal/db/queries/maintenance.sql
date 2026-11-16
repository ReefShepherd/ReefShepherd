-- name: ListPresets :many
SELECT
    id,
    preset_key,
    name,
    description,
    tankless,
    schedule_kind,
    interval_days,
    weekdays,
    interval_weeks,
    month_day,
    anchor_date,
    sort_order,
    archived_at,
    created_at,
    updated_at
FROM maintenance_presets
WHERE CAST(sqlc.arg(include_archived) AS INTEGER) = 1 OR archived_at IS NULL
ORDER BY sort_order, name, id;

-- name: GetPreset :one
SELECT
    id,
    preset_key,
    name,
    description,
    tankless,
    schedule_kind,
    interval_days,
    weekdays,
    interval_weeks,
    month_day,
    anchor_date,
    sort_order,
    archived_at,
    created_at,
    updated_at
FROM maintenance_presets
WHERE id = ?;

-- name: InsertPreset :one
INSERT INTO maintenance_presets (
    name, description, tankless, schedule_kind, interval_days, weekdays, interval_weeks, month_day, anchor_date,
    sort_order, created_at, updated_at
)
VALUES (
    sqlc.arg(name), sqlc.arg(description), sqlc.arg(tankless), sqlc.arg(schedule_kind), sqlc.arg(interval_days),
    sqlc.arg(weekdays), sqlc.arg(interval_weeks), sqlc.arg(month_day), sqlc.arg(anchor_date),
    (SELECT COALESCE(MAX(sort_order) + 10, 10) FROM maintenance_presets),
    sqlc.arg(created_at), sqlc.arg(updated_at)
)
RETURNING id;

-- name: UpdatePreset :execrows
UPDATE maintenance_presets
SET
    name = sqlc.arg(name),
    description = sqlc.arg(description),
    tankless = sqlc.arg(tankless),
    schedule_kind = sqlc.arg(schedule_kind),
    interval_days = sqlc.arg(interval_days),
    weekdays = sqlc.arg(weekdays),
    interval_weeks = sqlc.arg(interval_weeks),
    month_day = sqlc.arg(month_day),
    anchor_date = sqlc.arg(anchor_date),
    archived_at = sqlc.arg(archived_at),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- Tasks with their tank name and newest completion: a task id returns that task whatever its tank, a tank id returns that tank's tasks (plus whole-system ones on request), and no id returns every task except those of archived tanks.
-- name: ListTasks :many
SELECT
    t.id,
    t.tank_id,
    k.name AS tank_name,
    t.preset_id,
    t.name,
    t.description,
    t.schedule_kind,
    t.interval_days,
    t.weekdays,
    t.interval_weeks,
    t.month_day,
    t.anchor_date,
    t.due_time,
    t.schedule_since,
    t.notify,
    t.paused,
    t.created_at,
    t.updated_at,
    c.done_at AS last_done_at,
    c.outcome AS last_outcome
FROM maintenance_tasks AS t
LEFT JOIN tanks AS k ON k.id = t.tank_id
LEFT JOIN maintenance_completions AS c
    ON c.id = (
        SELECT l.id
        FROM maintenance_completions AS l
        WHERE l.task_id = t.id
        ORDER BY l.done_at DESC, l.id DESC
        LIMIT 1
    )
WHERE (
        CASE
            WHEN CAST(sqlc.narg(task_id) AS INTEGER) IS NOT NULL THEN t.id = sqlc.narg(task_id)
            WHEN CAST(sqlc.narg(tank_id) AS INTEGER) IS NOT NULL THEN (
                t.tank_id = sqlc.narg(tank_id)
                OR (t.tank_id IS NULL AND CAST(sqlc.arg(include_tankless) AS INTEGER) = 1)
            )
            ELSE k.archived_at IS NULL
        END
    )
    AND (CAST(sqlc.arg(reminders_only) AS INTEGER) = 0 OR (t.paused = 0 AND t.notify = 1))
ORDER BY t.id;

-- name: InsertTask :one
INSERT INTO maintenance_tasks (
    tank_id, preset_id, name, description, schedule_kind, interval_days, weekdays, interval_weeks, month_day, anchor_date,
    due_time, schedule_since, notify, paused, created_at, updated_at
)
VALUES (
    sqlc.arg(tank_id), sqlc.arg(preset_id), sqlc.arg(name), sqlc.arg(description), sqlc.arg(schedule_kind),
    sqlc.arg(interval_days), sqlc.arg(weekdays), sqlc.arg(interval_weeks), sqlc.arg(month_day), sqlc.arg(anchor_date),
    sqlc.arg(due_time), sqlc.arg(schedule_since), sqlc.arg(notify), sqlc.arg(paused), sqlc.arg(created_at), sqlc.arg(updated_at)
)
RETURNING id;

-- name: UpdateTask :execrows
UPDATE maintenance_tasks
SET
    tank_id = sqlc.arg(tank_id),
    name = sqlc.arg(name),
    description = sqlc.arg(description),
    schedule_kind = sqlc.arg(schedule_kind),
    interval_days = sqlc.arg(interval_days),
    weekdays = sqlc.arg(weekdays),
    interval_weeks = sqlc.arg(interval_weeks),
    month_day = sqlc.arg(month_day),
    anchor_date = sqlc.arg(anchor_date),
    due_time = sqlc.arg(due_time),
    schedule_since = sqlc.arg(schedule_since),
    notify = sqlc.arg(notify),
    paused = sqlc.arg(paused),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: DeleteTask :execrows
DELETE FROM maintenance_tasks
WHERE id = ?;

-- name: TaskExists :one
SELECT COUNT(*)
FROM maintenance_tasks
WHERE id = ?;

-- name: InsertCompletion :one
INSERT INTO maintenance_completions (task_id, done_at, outcome, note, done_by, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: GetCompletion :one
SELECT
    c.id,
    c.task_id,
    c.done_at,
    c.outcome,
    c.note,
    c.created_at,
    u.display_name AS done_by_display_name,
    u.username AS done_by_username
FROM maintenance_completions AS c
LEFT JOIN users AS u ON u.id = c.done_by
WHERE c.id = ?;

-- name: ListTaskCompletions :many
SELECT
    c.id,
    c.task_id,
    c.done_at,
    c.outcome,
    c.note,
    c.created_at,
    u.display_name AS done_by_display_name,
    u.username AS done_by_username
FROM maintenance_completions AS c
LEFT JOIN users AS u ON u.id = c.done_by
WHERE c.task_id = sqlc.arg(task_id)
    AND (
        CAST(sqlc.narg(cursor_at) AS TEXT) IS NULL
        OR c.done_at < sqlc.narg(cursor_at)
        OR (c.done_at = sqlc.narg(cursor_at) AND c.id < sqlc.narg(cursor_id))
    )
ORDER BY c.done_at DESC, c.id DESC
LIMIT sqlc.arg(row_limit);

-- name: UpdateCompletion :execrows
UPDATE maintenance_completions
SET
    done_at = sqlc.arg(done_at),
    outcome = sqlc.arg(outcome),
    note = sqlc.arg(note)
WHERE id = sqlc.arg(id);

-- name: DeleteCompletion :execrows
DELETE FROM maintenance_completions
WHERE id = ?;
