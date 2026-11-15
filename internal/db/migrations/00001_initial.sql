-- +goose Up

-- Instance-wide settings as key/value pairs; missing keys use the defaults in the Go settings package.
CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
) STRICT;

CREATE TABLE users (
    id                  INTEGER PRIMARY KEY,
    username            TEXT NOT NULL UNIQUE COLLATE NOCASE,
    display_name        TEXT NOT NULL DEFAULT '',
    email               TEXT,
    -- Argon2id hash in PHC string form, NULL is reserved for future SSO-only users.
    password_hash       TEXT,
    -- Allowed values: admin, member.
    role                TEXT NOT NULL DEFAULT 'member',
    disabled            INTEGER NOT NULL DEFAULT 0 CHECK (disabled IN (0, 1)),
    password_changed_at TEXT,
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
) STRICT;

-- One row per user who has started or finished TOTP setup.
CREATE TABLE user_totp (
    user_id        INTEGER PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    -- Base32 shared secret as shown in the authenticator QR code.
    secret         TEXT NOT NULL,
    -- NULL until the user proves the setup with a first valid code.
    confirmed_at   TEXT,
    -- Last accepted 30-second time step, so a code cannot be reused.
    last_used_step INTEGER NOT NULL DEFAULT 0,
    created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
) STRICT;

CREATE TABLE user_recovery_codes (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- SHA-256 of the one-time code, the plain code is shown to the user once.
    code_hash  TEXT NOT NULL,
    used_at    TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
) STRICT;

CREATE INDEX user_recovery_codes_user ON user_recovery_codes (user_id);

CREATE TABLE sessions (
    id           INTEGER PRIMARY KEY,
    -- SHA-256 of the cookie token, so a copied database cannot be used to log in.
    token_hash   BLOB NOT NULL UNIQUE,
    user_id      INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- Allowed values: mfa_pending, active.
    stage        TEXT NOT NULL,
    -- Allowed values: password, and later oidc.
    auth_method  TEXT NOT NULL,
    user_agent   TEXT NOT NULL DEFAULT '',
    ip_address   TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    last_seen_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    expires_at   TEXT NOT NULL
) STRICT;

CREATE INDEX sessions_user ON sessions (user_id);
CREATE INDEX sessions_expires ON sessions (expires_at);

CREATE TABLE tanks (
    id            INTEGER PRIMARY KEY,
    name          TEXT NOT NULL,
    -- Allowed values: reef, fowlr, frag, quarantine, other.
    tank_type     TEXT NOT NULL DEFAULT 'reef',
    -- Total system water volume in liters, used later by dosing calculators.
    volume_liters REAL CHECK (volume_liters IS NULL OR volume_liters > 0),
    started_on    TEXT,
    note          TEXT NOT NULL DEFAULT '',
    sort_order    INTEGER NOT NULL DEFAULT 0,
    -- Archived tanks are hidden but keep their history.
    archived_at   TEXT,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
) STRICT;

CREATE TABLE user_preferences (
    user_id         INTEGER PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    -- Allowed values: system, light, dark, oled.
    theme           TEXT NOT NULL DEFAULT 'system',
    locale          TEXT NOT NULL DEFAULT 'en',
    default_tank_id INTEGER REFERENCES tanks (id) ON DELETE SET NULL,
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
) STRICT;

-- Every parameter that can be tested, both shipped presets and user-made ones.
CREATE TABLE parameter_definitions (
    id                         INTEGER PRIMARY KEY,
    -- Stable key for presets such as alkalinity, NULL for custom parameters.
    preset_key                 TEXT UNIQUE,
    name                       TEXT NOT NULL,
    -- Allowed values: temperature, salinity, alkalinity, phosphate, fixed.
    unit_family                TEXT NOT NULL DEFAULT 'fixed',
    -- Unit shown for the fixed family, such as ppm or mV, empty for pH.
    unit_label                 TEXT NOT NULL DEFAULT '',
    -- Display decimals for the fixed family, other families use the unit catalog.
    decimals                   INTEGER NOT NULL DEFAULT 2 CHECK (decimals BETWEEN 0 AND 4),
    -- Default ideal range in canonical units, copied to a tank when the parameter is enabled there.
    default_low                REAL,
    default_high               REAL,
    default_test_interval_days INTEGER,
    sort_order                 INTEGER NOT NULL DEFAULT 0,
    archived_at                TEXT,
    created_at                 TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at                 TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
) STRICT;

-- Per-tank settings for each parameter: on or off, ideal range, test schedule.
CREATE TABLE tank_parameters (
    tank_id        INTEGER NOT NULL REFERENCES tanks (id) ON DELETE CASCADE,
    parameter_id   INTEGER NOT NULL REFERENCES parameter_definitions (id) ON DELETE CASCADE,
    enabled        INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    -- Ideal range in canonical units, NULL means no limit on that side.
    ideal_low      REAL,
    ideal_high     REAL,
    -- Allowed values: none, after_last, every_n_days, weekly, monthly.
    schedule_kind  TEXT NOT NULL DEFAULT 'none',
    -- Used by after_last ("N days after last done") and every_n_days.
    interval_days  INTEGER CHECK (interval_days IS NULL OR interval_days >= 1),
    -- Weekly days as a bitmask: Mon=1, Tue=2, Wed=4, Thu=8, Fri=16, Sat=32, Sun=64, every day=127.
    weekdays       INTEGER CHECK (weekdays IS NULL OR weekdays BETWEEN 1 AND 127),
    -- Weekly only: repeat every N weeks.
    interval_weeks INTEGER NOT NULL DEFAULT 1 CHECK (interval_weeks >= 1),
    -- Monthly only: day of month, clamped to the last day in short months.
    month_day      INTEGER CHECK (month_day IS NULL OR month_day BETWEEN 1 AND 31),
    -- First occurrence date for every_n_days and multi-week schedules.
    anchor_date    TEXT,
    -- Local time of day (instance timezone) when the item becomes due.
    due_time       TEXT NOT NULL DEFAULT '09:00',
    -- When the parameter was last switched on or given a new schedule, the baseline for its due time until it is first tested.
    schedule_since TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    sort_order     INTEGER NOT NULL DEFAULT 0,
    updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    PRIMARY KEY (tank_id, parameter_id)
) STRICT;

CREATE TABLE test_results (
    id           INTEGER PRIMARY KEY,
    tank_id      INTEGER NOT NULL REFERENCES tanks (id) ON DELETE CASCADE,
    -- RESTRICT so a parameter with history can only be archived, not deleted.
    parameter_id INTEGER NOT NULL REFERENCES parameter_definitions (id) ON DELETE RESTRICT,
    measured_at  TEXT NOT NULL,
    -- Value in the parameter's canonical unit.
    value        REAL NOT NULL,
    note         TEXT NOT NULL DEFAULT '',
    -- Allowed values: manual, import, and later icp or controller sources.
    source       TEXT NOT NULL DEFAULT 'manual',
    created_by   INTEGER REFERENCES users (id) ON DELETE SET NULL,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
) STRICT;

CREATE INDEX test_results_series ON test_results (tank_id, parameter_id, measured_at);

-- Each user's dashboard for each tank, one row per card in display order.
CREATE TABLE dashboard_cards (
    id           INTEGER PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    tank_id      INTEGER NOT NULL REFERENCES tanks (id) ON DELETE CASCADE,
    -- Allowed values: parameter, parameter_chart, tests_due, and later maintenance_due, feeding_today, journal_today.
    card_type    TEXT NOT NULL,
    -- Set for parameter and parameter_chart cards.
    parameter_id INTEGER REFERENCES parameter_definitions (id) ON DELETE CASCADE,
    position     INTEGER NOT NULL,
    -- Allowed values: normal, wide.
    size         TEXT NOT NULL DEFAULT 'normal',
    -- Small per-card options as JSON, such as the chart time range.
    options_json TEXT NOT NULL DEFAULT '{}',
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
) STRICT;

CREATE INDEX dashboard_cards_owner ON dashboard_cards (user_id, tank_id, position);

-- Shipped parameter presets; the ideal ranges are in canonical units and are only starting points.
INSERT INTO parameter_definitions
    (preset_key, name, unit_family, unit_label, decimals,
     default_low, default_high, default_test_interval_days, sort_order)
VALUES
    ('temperature', 'Temperature', 'temperature', '', 1, 24.5, 27.5, 7, 10),
    ('salinity', 'Salinity', 'salinity', '', 3, 34.0, 35.5, 7, 20),
    ('ph', 'pH', 'fixed', '', 1, 7.9, 8.4, 7, 30),
    ('alkalinity', 'Alkalinity', 'alkalinity', '', 1, 7.0, 11.0, 7, 40),
    ('calcium', 'Calcium', 'fixed', 'ppm', 0, 380, 450, 7, 50),
    ('magnesium', 'Magnesium', 'fixed', 'ppm', 0, 1250, 1400, 14, 60),
    ('nitrate', 'Nitrate', 'fixed', 'ppm', 1, 5, 50, 7, 70),
    ('phosphate', 'Phosphate', 'phosphate', '', 2, 0.06, 0.30, 7, 80),
    ('ammonia', 'Ammonia', 'fixed', 'ppm', 2, NULL, 0.10, 30, 90),
    ('nitrite', 'Nitrite', 'fixed', 'ppm', 1, NULL, 0.1, 30, 100);
