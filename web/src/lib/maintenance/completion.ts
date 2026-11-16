import type {
  Completion,
  CompletionChanges,
  CompletionOutcome,
  NewCompletion
} from '#lib/api/maintenance.js';
import type { FieldErrors } from '#lib/forms/codes.js';
import { fromLocalInput, toLocalInput } from '#lib/format/local-time.js';

/** How far ahead of the clock a time may be, matching the server's allowance for a slow clock. */
const futureSlackMs = 5 * 60 * 1000;

export const outcomes: readonly CompletionOutcome[] = ['done', 'skipped'];

function tooLate(instant: string, now: Date): boolean {
  return Date.parse(instant) > now.getTime() + futureSlackMs;
}

/** What the form for marking a task done or skipped holds. */
export interface CompletionDraft {
  /** A datetime-local value in the instance timezone. */
  doneAt: string;
  note: string;
}

export function newCompletionDraft(now: Date, timeZone: string): CompletionDraft {
  return { doneAt: toLocalInput(now, timeZone), note: '' };
}

export interface CompletionEntry {
  /** Null when the entry cannot be sent. */
  body: NewCompletion | null;
  errors: FieldErrors;
}

/** The request for marking a task: a time left as it was opened is not sent, so the server stamps the moment of saving. */
export function completionBody(
  draft: CompletionDraft,
  opened: string,
  outcome: CompletionOutcome,
  timeZone: string,
  now: Date
): CompletionEntry {
  const body: NewCompletion = { outcome };
  if (draft.doneAt !== opened) {
    const instant = fromLocalInput(draft.doneAt, timeZone);
    if (instant === null) return { body: null, errors: { done_at: 'invalid' } };
    if (tooLate(instant, now)) return { body: null, errors: { done_at: 'out_of_range' } };
    body.done_at = instant;
  }
  const note = draft.note.trim();
  if (note !== '') body.note = note;
  return { body, errors: {} };
}

/** What the inline edit form of a history entry holds. */
export interface CompletionEditDraft extends CompletionDraft {
  outcome: CompletionOutcome;
}

export function draftFromCompletion(c: Completion, timeZone: string): CompletionEditDraft {
  return { doneAt: toLocalInput(c.done_at, timeZone), note: c.note, outcome: c.outcome };
}

export interface CompletionEdit {
  changes: CompletionChanges;
  errors: FieldErrors;
}

/** The fields of an entry the draft changes; fields left as they were are not sent. */
export function completionChanges(
  c: Completion,
  draft: CompletionEditDraft,
  timeZone: string,
  now: Date
): CompletionEdit {
  const changes: CompletionChanges = {};
  const errors: FieldErrors = {};
  if (draft.doneAt !== toLocalInput(c.done_at, timeZone)) {
    const instant = fromLocalInput(draft.doneAt, timeZone);
    if (instant === null) errors.done_at = 'invalid';
    else if (tooLate(instant, now)) errors.done_at = 'out_of_range';
    else changes.done_at = instant;
  }
  if (draft.outcome !== c.outcome) changes.outcome = draft.outcome;
  if (draft.note.trim() !== c.note) changes.note = draft.note.trim();
  return { changes, errors };
}
