import type { ResultChanges, TestResult } from '#lib/api/params.js';
import { decimalText, parseDecimal } from '#lib/format/decimal.js';
import type { FieldErrors } from '#lib/forms/codes.js';
import { fromLocalInput, toLocalInput } from '#lib/format/local-time.js';

/** What the inline edit form holds for a result, as typed. */
export interface ResultDraft {
  value: string;
  note: string;
  /** A datetime-local value in the instance timezone. */
  measuredAt: string;
}

export function draftFromResult(
  result: TestResult,
  decimals: number,
  timeZone: string
): ResultDraft {
  return {
    value: decimalText(result.value, decimals),
    note: result.note,
    measuredAt: toLocalInput(result.measured_at, timeZone)
  };
}

export interface ResultEdit {
  changes: ResultChanges;
  /** The fields that cannot be saved, keyed by the server's field names. */
  errors: FieldErrors;
}

/** The fields of a result the draft changes; fields left as they were are not sent. */
export function resultChanges(
  result: TestResult,
  draft: ResultDraft,
  timeZone: string
): ResultEdit {
  const changes: ResultChanges = {};
  const errors: FieldErrors = {};

  const value = parseDecimal(draft.value);
  if (value === null) errors.value = 'invalid';
  else if (value !== result.value) changes.value = value;

  if (draft.measuredAt !== toLocalInput(result.measured_at, timeZone)) {
    const instant = fromLocalInput(draft.measuredAt, timeZone);
    if (instant === null) errors.measured_at = 'invalid';
    else changes.measured_at = instant;
  }

  if (draft.note.trim() !== result.note) changes.note = draft.note.trim();
  return { changes, errors };
}
