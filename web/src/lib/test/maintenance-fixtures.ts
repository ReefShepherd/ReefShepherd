import type { Completion, Preset, Task } from '#lib/api/maintenance.js';
import { emptySchedule } from '#lib/schedule/schedule.js';

/** A task with every field set; the fields a test cares about are passed in. */
export function task(over: Partial<Task> = {}): Task {
  return {
    id: 1,
    tank_id: 3,
    tank_name: 'Display',
    preset_id: null,
    name: 'Water change',
    description: '',
    schedule: { ...emptySchedule('09:00'), kind: 'after_last', interval_days: 7 },
    notify: true,
    paused: false,
    last_done_at: null,
    last_outcome: null,
    next_due_at: null,
    overdue: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...over
  };
}

export function completion(over: Partial<Completion> = {}): Completion {
  return {
    id: 1,
    task_id: 1,
    done_at: '2026-11-10T09:00:00Z',
    outcome: 'done',
    note: '',
    done_by_name: null,
    created_at: '2026-11-10T09:00:00Z',
    ...over
  };
}

export function preset(over: Partial<Preset> = {}): Preset {
  return {
    id: 7,
    preset_key: 'water_change',
    name: 'Water change',
    description: 'Change a tenth of the water.',
    tankless: false,
    schedule: { ...emptySchedule('09:00'), kind: 'weekly', weekdays: 64 },
    sort_order: 10,
    archived_at: null,
    ...over
  };
}
