import type { NewResults, NewResultEntry } from '#lib/api/params.js';
import { fieldText } from '#lib/api/field-errors.js';
import { isApiError } from '#lib/api/client.js';
import { parseDecimal } from '#lib/format/decimal.js';

/** One line of the test entry form, with the value as typed. */
export interface EntryRow {
  parameterId: number;
  value: string;
  note: string;
}

/** Why a row cannot be sent: its value is not a number, or it has a note but no value. */
export type RowProblem = 'invalid_number' | 'value_required';

export interface BuiltEntry {
  /** The request, or null when there is nothing to send or a row has a problem. */
  body: NewResults | null;
  /** The index in rows of each result in the request, in order. */
  sent: number[];
  problems: Record<number, RowProblem>;
}

/** Turns the form rows into the request: rows with neither value nor note are skipped, and measuredAt (an RFC 3339 instant) is left out to mean now. */
export function buildEntry(rows: readonly EntryRow[], measuredAt: string | null): BuiltEntry {
  const results: NewResultEntry[] = [];
  const sent: number[] = [];
  const problems: Record<number, RowProblem> = {};
  rows.forEach((row, index) => {
    const text = row.value.trim();
    const note = row.note.trim();
    if (text === '' && note === '') return;
    if (text === '') {
      problems[index] = 'value_required';
      return;
    }
    const value = parseDecimal(text);
    if (value === null) {
      problems[index] = 'invalid_number';
      return;
    }
    results.push(
      note === ''
        ? { parameter_id: row.parameterId, value }
        : { parameter_id: row.parameterId, value, note }
    );
    sent.push(index);
  });
  const body: NewResults | null =
    results.length > 0 && Object.keys(problems).length === 0
      ? measuredAt === null
        ? { results }
        : { measured_at: measuredAt, results }
      : null;
  return { body, sent: body ? sent : [], problems };
}

export interface RowErrors {
  value?: string;
  note?: string;
}

/** Maps the server's results.N.value and results.N.note errors back to the form rows that were sent. */
export function rowErrors(err: unknown, sent: readonly number[]): Record<number, RowErrors> {
  const out: Record<number, RowErrors> = {};
  if (!isApiError(err)) return out;
  sent.forEach((rowIndex, n) => {
    const value =
      fieldText(err, `results.${n}.value`) ?? fieldText(err, `results.${n}.parameter_id`);
    const note = fieldText(err, `results.${n}.note`);
    if (value !== undefined || note !== undefined) out[rowIndex] = { value, note };
  });
  return out;
}
