-- +goose Up

-- Task templates offered when adding a maintenance task, both shipped and user-made.
CREATE TABLE maintenance_presets (
    id             INTEGER PRIMARY KEY,
    -- Stable key for shipped presets such as water_change, NULL for user templates.
    preset_key     TEXT UNIQUE,
    name           TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    -- True for tasks that usually belong to the whole system, such as RO/DI cartridges.
    tankless       INTEGER NOT NULL DEFAULT 0 CHECK (tankless IN (0, 1)),
    -- Suggested schedule; presets have no time of day because tasks take the instance reminder time.
    schedule_kind  TEXT NOT NULL DEFAULT 'none',
    interval_days  INTEGER CHECK (interval_days IS NULL OR interval_days >= 1),
    weekdays       INTEGER CHECK (weekdays IS NULL OR weekdays BETWEEN 1 AND 127),
    interval_weeks INTEGER NOT NULL DEFAULT 1 CHECK (interval_weeks >= 1),
    month_day      INTEGER CHECK (month_day IS NULL OR month_day BETWEEN 1 AND 31),
    anchor_date    TEXT,
    sort_order     INTEGER NOT NULL DEFAULT 0,
    -- Hidden presets are not offered when adding a task.
    archived_at    TEXT,
    created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
) STRICT;

CREATE TABLE maintenance_tasks (
    id             INTEGER PRIMARY KEY,
    -- NULL means the task belongs to the whole system rather than one tank.
    tank_id        INTEGER REFERENCES tanks (id) ON DELETE CASCADE,
    preset_id      INTEGER REFERENCES maintenance_presets (id) ON DELETE SET NULL,
    name           TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    -- Allowed values: none, after_last, every_n_days, weekly, monthly.
    schedule_kind  TEXT NOT NULL DEFAULT 'none',
    -- Used by after_last and every_n_days.
    interval_days  INTEGER CHECK (interval_days IS NULL OR interval_days >= 1),
    -- Weekly days as a bitmask: Mon=1, Tue=2, Wed=4, Thu=8, Fri=16, Sat=32, Sun=64.
    weekdays       INTEGER CHECK (weekdays IS NULL OR weekdays BETWEEN 1 AND 127),
    -- Weekly only: repeat every N weeks.
    interval_weeks INTEGER NOT NULL DEFAULT 1 CHECK (interval_weeks >= 1),
    -- Monthly only: day of month, clamped to the last day in short months.
    month_day      INTEGER CHECK (month_day IS NULL OR month_day BETWEEN 1 AND 31),
    -- First occurrence date for every_n_days and multi-week schedules.
    anchor_date    TEXT,
    -- Local time of day in the instance timezone when the task becomes due.
    due_time       TEXT NOT NULL DEFAULT '09:00',
    -- Baseline for the first due date until the task is first done, reset whenever the schedule changes.
    schedule_since TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    notify         INTEGER NOT NULL DEFAULT 1 CHECK (notify IN (0, 1)),
    paused         INTEGER NOT NULL DEFAULT 0 CHECK (paused IN (0, 1)),
    created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
) STRICT;
CREATE INDEX maintenance_tasks_tank ON maintenance_tasks (tank_id);

CREATE TABLE maintenance_completions (
    id         INTEGER PRIMARY KEY,
    task_id    INTEGER NOT NULL REFERENCES maintenance_tasks (id) ON DELETE CASCADE,
    done_at    TEXT NOT NULL,
    -- Allowed values: done, skipped.
    outcome    TEXT NOT NULL DEFAULT 'done',
    note       TEXT NOT NULL DEFAULT '',
    done_by    INTEGER REFERENCES users (id) ON DELETE SET NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
) STRICT;
CREATE INDEX maintenance_completions_task ON maintenance_completions (task_id, done_at);

-- Where notifications can be sent, configured by an admin and shared by the instance.
CREATE TABLE notification_channels (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    -- Allowed values: discord, slack, ntfy, webhook, email.
    kind        TEXT NOT NULL,
    -- Provider settings as JSON, stored as entered and masked by the API when read.
    config_json TEXT NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
) STRICT;

-- Which event types each channel receives.
CREATE TABLE notification_subscriptions (
    channel_id INTEGER NOT NULL REFERENCES notification_channels (id) ON DELETE CASCADE,
    -- Allowed values: maintenance_due, tests_due, out_of_range, backup_failed, and later feeding_due, update_available.
    event_type TEXT NOT NULL,
    PRIMARY KEY (channel_id, event_type)
) STRICT;

-- Every notification the app raised; this is also the in-app inbox.
CREATE TABLE notifications (
    id          INTEGER PRIMARY KEY,
    event_type  TEXT NOT NULL,
    -- Allowed values: info, warning, critical.
    severity    TEXT NOT NULL DEFAULT 'info',
    -- Text rendered in the instance language when the notification was raised.
    title       TEXT NOT NULL,
    body        TEXT NOT NULL DEFAULT '',
    -- Path inside the app the notification links to, such as /tanks/1/maintenance.
    link        TEXT NOT NULL DEFAULT '',
    tank_id     INTEGER REFERENCES tanks (id) ON DELETE CASCADE,
    -- What raised it: maintenance_task, tank_tests, test_result or backup.
    source_kind TEXT NOT NULL,
    -- Row id of the source, 0 when the source is not a row.
    source_id   INTEGER NOT NULL,
    -- Which occurrence of the source this is, such as a due time or a local date, so each one is raised once.
    occurrence  TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE (source_kind, source_id, occurrence)
) STRICT;
CREATE INDEX notifications_created ON notifications (created_at);

-- Per-user read state for the in-app inbox.
CREATE TABLE notification_reads (
    notification_id INTEGER NOT NULL REFERENCES notifications (id) ON DELETE CASCADE,
    user_id         INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    read_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    PRIMARY KEY (notification_id, user_id)
) STRICT;

-- One row per notification per channel, with retry state.
CREATE TABLE notification_deliveries (
    id              INTEGER PRIMARY KEY,
    notification_id INTEGER NOT NULL REFERENCES notifications (id) ON DELETE CASCADE,
    channel_id      INTEGER NOT NULL REFERENCES notification_channels (id) ON DELETE CASCADE,
    -- Allowed values: pending, sent, failed.
    status          TEXT NOT NULL DEFAULT 'pending',
    attempts        INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT NOT NULL DEFAULT '',
    -- When the next attempt is due, NULL once the delivery is sent or has failed for good.
    next_attempt_at TEXT,
    sent_at         TEXT,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE (notification_id, channel_id)
) STRICT;
CREATE INDEX notification_deliveries_pending ON notification_deliveries (status, next_attempt_at);

-- Shipped task presets; every one repeats a number of days after it was last done.
INSERT INTO maintenance_presets
    (preset_key, name, tankless, schedule_kind, interval_days, sort_order)
VALUES
    ('water_change', 'Water change', 0, 'after_last', 7, 10),
    ('skimmer_cup', 'Empty skimmer cup', 0, 'after_last', 3, 20),
    ('filter_media', 'Change filter socks or fleece', 0, 'after_last', 3, 30),
    ('glass', 'Clean glass', 0, 'after_last', 3, 40),
    ('ato_reservoir', 'Refill ATO reservoir', 0, 'after_last', 7, 50),
    ('carbon_gfo', 'Replace carbon and GFO', 0, 'after_last', 30, 60),
    ('probe_calibration', 'Calibrate probes', 0, 'after_last', 30, 70),
    ('overflow', 'Clean overflow and plumbing', 0, 'after_last', 30, 80),
    ('pumps', 'Clean return pump and powerheads', 0, 'after_last', 90, 90),
    ('rodi_cartridges', 'Replace RO/DI cartridges', 1, 'after_last', 180, 100);
