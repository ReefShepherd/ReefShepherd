import type {
  NewPreset,
  NewTask,
  Preset,
  PresetChanges,
  Task,
  TaskChanges
} from '#lib/api/maintenance.js';
import type { FieldErrors } from '#lib/forms/codes.js';
import { validateMaxLength, validateName } from '#lib/forms/validation.js';
import {
  emptySchedule,
  normalizeSchedule,
  sameSchedule,
  scheduleProblems,
  type Schedule,
  type ScheduleProblem
} from '#lib/schedule/schedule.js';

export const nameMax = 100;
export const descriptionMax = 2000;

/** The tank choice that stands for a task of the whole system. */
export const wholeSystem = '';

/** What the task form holds, as typed. */
export interface TaskDraft {
  /** The id of the template the task was started from, or an empty text for none. */
  presetId: string;
  name: string;
  description: string;
  /** A tank id as text, or wholeSystem. */
  tank: string;
  schedule: Schedule;
  notify: boolean;
  paused: boolean;
  /** The date it was last done as YYYY-MM-DD, or empty. */
  lastDoneOn: string;
}

const tankChoice = (tankId: number | null): string =>
  tankId === null ? wholeSystem : String(tankId);

/** An empty form for a new task; the schedule starts at the reminder time of the instance. */
export function blankDraft(tankId: number | null, dueTime: string): TaskDraft {
  return {
    presetId: '',
    name: '',
    description: '',
    tank: tankChoice(tankId),
    schedule: emptySchedule(dueTime),
    notify: true,
    paused: false,
    lastDoneOn: ''
  };
}

/** The form with a template's name, description, tank choice and schedule filled in; a template of the whole system picks that, any other keeps the tank chosen, or moves to the fallback tank when the whole system was chosen. */
export function draftFromPreset(
  draft: TaskDraft,
  preset: Preset,
  dueTime: string,
  fallbackTank: string = wholeSystem
): TaskDraft {
  return {
    ...draft,
    presetId: String(preset.id),
    name: preset.name,
    description: preset.description,
    tank: preset.tankless ? wholeSystem : draft.tank === wholeSystem ? fallbackTank : draft.tank,
    schedule: { ...preset.schedule, due_time: preset.schedule.due_time || dueTime }
  };
}

export function draftFromTask(task: Task): TaskDraft {
  return {
    presetId: task.preset_id === null ? '' : String(task.preset_id),
    name: task.name,
    description: task.description,
    tank: tankChoice(task.tank_id),
    schedule: task.schedule,
    notify: task.notify,
    paused: task.paused,
    lastDoneOn: ''
  };
}

/** The tanks a task can be moved to: those on offer, plus its own tank when that one is no longer among them, such as an archived tank. */
export function tankChoices(
  tanks: readonly { id: number; name: string }[],
  task: Pick<Task, 'tank_id' | 'tank_name'> | undefined
): { id: number; name: string }[] {
  const out = tanks.map((t) => ({ id: t.id, name: t.name }));
  if (task?.tank_id != null && !out.some((t) => t.id === task.tank_id)) {
    out.push({ id: task.tank_id, name: task.tank_name ?? String(task.tank_id) });
  }
  return out;
}

/** The tank id a draft names, null for the whole system, or 'invalid' for a choice that is no id. */
function tankOf(draft: TaskDraft): number | null | 'invalid' {
  if (draft.tank === wholeSystem) return null;
  const id = Number(draft.tank);
  return Number.isSafeInteger(id) && id > 0 ? id : 'invalid';
}

const datePattern = /^(\d{4})-(\d{2})-(\d{2})$/;

function realDate(text: string): boolean {
  const match = datePattern.exec(text);
  if (!match) return false;
  const [year, month, day] = [Number(match[1]), Number(match[2]), Number(match[3])];
  const date = new Date(Date.UTC(year, month - 1, day));
  return (
    date.getUTCFullYear() === year && date.getUTCMonth() === month - 1 && date.getUTCDate() === day
  );
}

function textErrors(draft: TaskDraft): FieldErrors {
  const errors: FieldErrors = {};
  const name = validateName(draft.name, nameMax);
  if (name) errors.name = name;
  const description = validateMaxLength(draft.description.trim(), descriptionMax);
  if (description) errors.description = description;
  if (tankOf(draft) === 'invalid') errors.tank_id = 'invalid';
  return errors;
}

export interface NewTaskEntry {
  /** Null when the task cannot be sent. */
  body: NewTask | null;
  /** The entries that cannot be saved, keyed by the server's field names. */
  errors: FieldErrors;
  /** What is wrong with the schedule, for the schedule editor to explain. */
  schedule: ScheduleProblem[];
}

/** The request that creates a task from the form; today is the current date in the instance timezone as YYYY-MM-DD. */
export function newTask(draft: TaskDraft, today: string): NewTaskEntry {
  const errors = textErrors(draft);
  if (draft.lastDoneOn !== '') {
    if (!realDate(draft.lastDoneOn)) errors.last_done_on = 'invalid';
    else if (draft.lastDoneOn > today) errors.last_done_on = 'out_of_range';
  }
  const schedule = scheduleProblems(draft.schedule);
  const tank = tankOf(draft);
  if (Object.keys(errors).length > 0 || schedule.length > 0 || tank === 'invalid') {
    return { body: null, errors, schedule };
  }
  const body: NewTask = {
    tank_id: tank,
    preset_id: draft.presetId === '' ? null : Number(draft.presetId),
    name: draft.name.trim(),
    description: draft.description.trim(),
    schedule: normalizeSchedule(draft.schedule),
    notify: draft.notify
  };
  if (draft.lastDoneOn !== '') body.last_done_on = draft.lastDoneOn;
  return { body, errors, schedule };
}

export interface TaskEdit {
  changes: TaskChanges;
  errors: FieldErrors;
  schedule: ScheduleProblem[];
}

/** The fields of a task the draft changes; fields left as they were are not sent. */
export function taskChanges(task: Task, draft: TaskDraft): TaskEdit {
  const errors = textErrors(draft);
  const schedule = scheduleProblems(draft.schedule);
  const changes: TaskChanges = {};
  if (draft.name.trim() !== task.name) changes.name = draft.name.trim();
  if (draft.description.trim() !== task.description) changes.description = draft.description.trim();
  const tank = tankOf(draft);
  if (tank !== 'invalid' && tank !== task.tank_id) changes.tank_id = tank;
  if (schedule.length === 0 && !sameSchedule(draft.schedule, task.schedule)) {
    changes.schedule = normalizeSchedule(draft.schedule);
  }
  if (draft.notify !== task.notify) changes.notify = draft.notify;
  if (draft.paused !== task.paused) changes.paused = draft.paused;
  return { changes, errors, schedule };
}

/** What the template form holds, as typed. */
export interface PresetDraft {
  name: string;
  description: string;
  tankless: boolean;
  schedule: Schedule;
}

export function blankPresetDraft(dueTime: string): PresetDraft {
  return { name: '', description: '', tankless: false, schedule: emptySchedule(dueTime) };
}

export function draftFromPresetRecord(preset: Preset): PresetDraft {
  return {
    name: preset.name,
    description: preset.description,
    tankless: preset.tankless,
    schedule: preset.schedule
  };
}

function presetErrors(draft: PresetDraft): FieldErrors {
  const errors: FieldErrors = {};
  const name = validateName(draft.name, nameMax);
  if (name) errors.name = name;
  const description = validateMaxLength(draft.description.trim(), descriptionMax);
  if (description) errors.description = description;
  return errors;
}

export interface NewPresetEntry {
  body: NewPreset | null;
  errors: FieldErrors;
  schedule: ScheduleProblem[];
}

export function newPreset(draft: PresetDraft): NewPresetEntry {
  const errors = presetErrors(draft);
  const schedule = scheduleProblems(draft.schedule);
  if (Object.keys(errors).length > 0 || schedule.length > 0)
    return { body: null, errors, schedule };
  return {
    body: {
      name: draft.name.trim(),
      description: draft.description.trim(),
      tankless: draft.tankless,
      schedule: normalizeSchedule(draft.schedule)
    },
    errors,
    schedule
  };
}

export interface PresetEdit {
  changes: PresetChanges;
  errors: FieldErrors;
  schedule: ScheduleProblem[];
}

export function presetChanges(preset: Preset, draft: PresetDraft): PresetEdit {
  const errors = presetErrors(draft);
  const schedule = scheduleProblems(draft.schedule);
  const changes: PresetChanges = {};
  if (draft.name.trim() !== preset.name) changes.name = draft.name.trim();
  if (draft.description.trim() !== preset.description) {
    changes.description = draft.description.trim();
  }
  if (draft.tankless !== preset.tankless) changes.tankless = draft.tankless;
  if (schedule.length === 0 && !sameSchedule(draft.schedule, preset.schedule)) {
    changes.schedule = normalizeSchedule(draft.schedule);
  }
  return { changes, errors, schedule };
}
