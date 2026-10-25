import { describe, expect, it } from 'vitest';
import type { TestResult } from '#lib/api/params.js';
import { addResults, appendPage, removeResult, replaceResult, resultQuery } from './paging.js';

function result(id: number, measuredAt: string, parameterId = 1): TestResult {
  return {
    id,
    tank_id: 1,
    parameter_id: parameterId,
    parameter_name: 'pH',
    measured_at: measuredAt,
    value: 8,
    unit: '',
    unit_label: '',
    in_range: true,
    note: '',
    source: 'manual',
    created_by: 1,
    created_at: measuredAt,
    updated_at: measuredAt
  };
}

const ids = (list: TestResult[]) => list.map((r) => r.id);

describe('resultQuery', () => {
  it('asks for the first page of everything', () => {
    expect(resultQuery(null, null)).toEqual({ limit: 50 });
  });

  it('adds the parameter filter and the cursor when set', () => {
    expect(resultQuery(7, 'abc', 20)).toEqual({ limit: 20, parameter_id: 7, cursor: 'abc' });
  });
});

describe('appendPage', () => {
  it('adds a page after the rows shown and skips rows already there', () => {
    const shown = [result(5, '2026-10-05T10:00:00Z'), result(4, '2026-10-04T10:00:00Z')];
    const page = {
      results: [result(4, '2026-10-04T10:00:00Z'), result(3, '2026-10-03T10:00:00Z')],
      next_cursor: null
    };
    expect(ids(appendPage(shown, page))).toEqual([5, 4, 3]);
  });
});

describe('replaceResult', () => {
  it('swaps in the saved copy and restores newest-first order after a time change', () => {
    const shown = [
      result(3, '2026-10-03T10:00:00Z'),
      result(2, '2026-10-02T10:00:00Z'),
      result(1, '2026-10-01T10:00:00Z')
    ];
    const moved = { ...result(1, '2026-10-04T10:00:00Z'), value: 7.5 };
    const next = replaceResult(shown, moved);
    expect(ids(next)).toEqual([1, 3, 2]);
    expect(next[0].value).toBe(7.5);
  });

  it('orders results with the same time by id, newest id first', () => {
    const shown = [result(2, '2026-10-02T10:00:00Z'), result(1, '2026-10-01T10:00:00Z')];
    expect(ids(replaceResult(shown, result(1, '2026-10-02T10:00:00Z')))).toEqual([2, 1]);
  });
});

describe('removeResult', () => {
  it('drops one result', () => {
    const shown = [result(2, '2026-10-02T10:00:00Z'), result(1, '2026-10-01T10:00:00Z')];
    expect(ids(removeResult(shown, 2))).toEqual([1]);
  });
});

describe('addResults', () => {
  const shown = [result(2, '2026-10-02T10:00:00Z', 1), result(1, '2026-10-01T10:00:00Z', 2)];
  const added = [result(4, '2026-10-05T10:00:00Z', 1), result(5, '2026-10-05T10:00:00Z', 2)];

  it('puts new results on top when no filter is set', () => {
    expect(ids(addResults(shown, added, null))).toEqual([5, 4, 2, 1]);
  });

  it('keeps only the filtered parameter when a filter is set', () => {
    expect(ids(addResults(shown, added, 2))).toEqual([5, 2, 1]);
  });

  it('does not repeat a result that is already shown', () => {
    expect(ids(addResults(shown, [shown[0]], null))).toEqual([2, 1]);
  });
});
