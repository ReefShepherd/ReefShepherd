import { describe, expect, it } from 'vitest';
import { ApiError } from '#lib/api/client.js';
import { buildEntry, rowErrors, type EntryRow } from './entry.js';

const row = (parameterId: number, value = '', note = ''): EntryRow => ({
  parameterId,
  value,
  note
});

describe('buildEntry', () => {
  it('sends the filled rows in order and skips the empty ones', () => {
    const entry = buildEntry(
      [row(1, '8.2'), row(2), row(3, '35', ' fresh salt '), row(4, '  ')],
      null
    );
    expect(entry.body).toEqual({
      results: [
        { parameter_id: 1, value: 8.2 },
        { parameter_id: 3, value: 35, note: 'fresh salt' }
      ]
    });
    expect(entry.sent).toEqual([0, 2]);
    expect(entry.problems).toEqual({});
  });

  it('reads a decimal comma', () => {
    expect(buildEntry([row(1, '1,025')], null).body?.results).toEqual([
      { parameter_id: 1, value: 1.025 }
    ]);
  });

  it('includes the time only when one is given', () => {
    expect(buildEntry([row(1, '8')], '2026-10-24T09:30:00Z').body).toEqual({
      measured_at: '2026-10-24T09:30:00Z',
      results: [{ parameter_id: 1, value: 8 }]
    });
    expect(buildEntry([row(1, '8')], null).body).not.toHaveProperty('measured_at');
  });

  it('sends nothing when every row is empty', () => {
    const entry = buildEntry([row(1), row(2, '  ', '  ')], null);
    expect(entry).toEqual({ body: null, sent: [], problems: {} });
  });

  it('flags a value that is not a number and a note without a value, and sends nothing', () => {
    const entry = buildEntry([row(1, '8.2'), row(2, 'abc'), row(3, '', 'forgot the value')], null);
    expect(entry.problems).toEqual({ 1: 'invalid_number', 2: 'value_required' });
    expect(entry.body).toBeNull();
    expect(entry.sent).toEqual([]);
  });
});

describe('rowErrors', () => {
  const error = (fields: Record<string, string>) =>
    new ApiError(422, 'validation', 'invalid', fields);

  it('maps results.N.value back to the form row that was sent as Nth', () => {
    // Rows 0 and 3 were filled, so server result 1 is form row 3.
    const errors = rowErrors(error({ 'results.1.value': 'is outside the possible range' }), [0, 3]);
    expect(Object.keys(errors)).toEqual(['3']);
    expect(errors[3].value).toBeTruthy();
    expect(errors[3].note).toBeUndefined();
  });

  it('maps note errors and a refused parameter to their rows', () => {
    const errors = rowErrors(
      error({
        'results.0.note': 'must be at most 2000 characters',
        'results.1.parameter_id': 'this parameter is archived'
      }),
      [2, 5]
    );
    expect(errors[2].note).toBeTruthy();
    expect(errors[5].value).toBeTruthy();
  });

  it('ignores errors that are not about a row and errors that are not API errors', () => {
    expect(rowErrors(error({ measured_at: 'bad' }), [0])).toEqual({});
    expect(rowErrors(new Error('x'), [0])).toEqual({});
  });
});
