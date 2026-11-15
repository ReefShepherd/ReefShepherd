import type { NewResults, NewResultEntry } from '#lib/api/params.js';
import { parseDecimal } from '#lib/format/decimal.js';
import type { FieldErrors } from '#lib/forms/codes.js';

/** One line of the test entry form, with the value as typed. */
export interface EntryRow {
  parameterId: number;
  value: string;
  note: string;
}

export interface BuiltEntry {
  /** The request, or null when there is nothing to send or a row has a problem. */
  body: NewResults | null;
  /** The index in rows of each result in the request, in order. */
  sent: number[];
  /** Why a row cannot be sent: its `value` is not a number, or it has a note but no value (`value_missing`). */
  problems: Record<number, FieldErrors>;
}

/** Turns the form rows into the request: rows with neither value nor note are skipped, and measuredAt (an RFC 3339 instant) is left out to mean now. */
export function buildEntry(rows: readonly EntryRow[], measuredAt: string | null): BuiltEntry {
  const results: NewResultEntry[] = [];
  const sent: number[] = [];
  const problems: Record<number, FieldErrors> = {};
  rows.forEach((row, index) => {
    const text = row.value.trim();
    const note = row.note.trim();
    if (text === '' && note === '') return;
    if (text === '') {
      problems[index] = { value_missing: 'required' };
      return;
    }
    const value = parseDecimal(text);
    if (value === null) {
      problems[index] = { value: 'invalid' };
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

/** Maps the server's results.N.value, results.N.parameter_id and results.N.note errors back to the form rows that were sent. */
export function rowErrors(
  fields: FieldErrors,
  sent: readonly number[]
): Record<number, FieldErrors> {
  const out: Record<number, FieldErrors> = {};
  sent.forEach((rowIndex, n) => {
    for (const name of ['value', 'parameter_id', 'note']) {
      const code = fields[`results.${n}.${name}`];
      if (code !== undefined) out[rowIndex] = { ...out[rowIndex], [name]: code };
    }
  });
  return out;
}
