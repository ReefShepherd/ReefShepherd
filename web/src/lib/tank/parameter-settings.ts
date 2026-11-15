import type { TankParameter, TankParameterChanges } from '#lib/api/params.js';
import { decimalText, parseDecimal } from '#lib/format/decimal.js';
import type { FieldErrors } from '#lib/forms/codes.js';
import { validateLimitPair } from '#lib/forms/validation.js';
import {
  normalizeSchedule,
  sameSchedule,
  scheduleProblems,
  type Schedule,
  type ScheduleProblem
} from '#lib/schedule/schedule.js';

const orderStep = 10;

/** The tank's parameters in the order they are shown: by sort order, then by id. */
export function orderParameters<T extends Pick<TankParameter, 'sort_order' | 'parameter_id'>>(
  parameters: readonly T[]
): T[] {
  return [...parameters].sort(
    (a, b) => a.sort_order - b.sort_order || a.parameter_id - b.parameter_id
  );
}

export interface OrderChange {
  parameterId: number;
  sortOrder: number;
}

/** The sort orders to save after moving the parameter at index from to index to; every parameter ends up on a distinct multiple of ten, and only those that moved are listed. */
export function reorderChanges(
  ordered: readonly Pick<TankParameter, 'sort_order' | 'parameter_id'>[],
  from: number,
  to: number
): OrderChange[] {
  if (from === to || from < 0 || to < 0 || from >= ordered.length || to >= ordered.length)
    return [];
  const next = [...ordered];
  const [moved] = next.splice(from, 1);
  next.splice(to, 0, moved);
  const changes: OrderChange[] = [];
  next.forEach((p, i) => {
    const sortOrder = (i + 1) * orderStep;
    if (p.sort_order !== sortOrder) changes.push({ parameterId: p.parameter_id, sortOrder });
  });
  return changes;
}

/** What the settings form of one parameter holds. */
export interface ParameterDraft {
  low: string;
  high: string;
  schedule: Schedule;
}

export function draftFromParameter(p: TankParameter): ParameterDraft {
  return {
    low: decimalText(p.ideal_low, p.decimals),
    high: decimalText(p.ideal_high, p.decimals),
    schedule: p.schedule
  };
}

export interface ParameterEdit {
  changes: TankParameterChanges;
  /** The limits that cannot be saved, keyed by the server's field names; `range` flags a low limit above the high one. */
  errors: FieldErrors;
  /** What is wrong with the schedule, for the schedule editor to explain. */
  schedule: ScheduleProblem[];
}

function limit(text: string): number | null | 'invalid' {
  if (text.trim() === '') return null;
  return parseDecimal(text) ?? 'invalid';
}

/** The settings the draft changes, and the entries that cannot be saved; a blank limit clears it. */
export function parameterChanges(p: TankParameter, draft: ParameterDraft): ParameterEdit {
  const changes: TankParameterChanges = {};
  const low = limit(draft.low);
  const high = limit(draft.high);
  if (low !== 'invalid' && low !== p.ideal_low) changes.ideal_low = low;
  if (high !== 'invalid' && high !== p.ideal_high) changes.ideal_high = high;

  const schedule = scheduleProblems(draft.schedule);
  if (schedule.length === 0 && !sameSchedule(draft.schedule, p.schedule))
    changes.schedule = normalizeSchedule(draft.schedule);
  const errors = validateLimitPair(draft.low, draft.high, { low: 'ideal_low', high: 'ideal_high' });
  return { changes, errors, schedule };
}
