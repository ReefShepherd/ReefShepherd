import { describe, expect, it } from 'vitest';
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
    expect(entry.problems).toEqual({ 1: { value: 'invalid' }, 2: { value_missing: 'required' } });
    expect(entry.body).toBeNull();
    expect(entry.sent).toEqual([]);
  });
});

describe('rowErrors', () => {
  it('maps results.N.value back to the form row that was sent as Nth', () => {
    // Rows 0 and 3 were filled, so server result 1 is form row 3.
    expect(rowErrors({ 'results.1.value': 'out_of_range' }, [0, 3])).toEqual({
      3: { value: 'out_of_range' }
    });
  });

  it('maps note errors and a refused parameter to their rows', () => {
    expect(
      rowErrors({ 'results.0.note': 'too_long', 'results.1.parameter_id': 'invalid' }, [2, 5])
    ).toEqual({ 2: { note: 'too_long' }, 5: { parameter_id: 'invalid' } });
  });

  it('ignores errors that are not about a row', () => {
    expect(rowErrors({ measured_at: 'invalid', results: 'required' }, [0])).toEqual({});
  });
});
