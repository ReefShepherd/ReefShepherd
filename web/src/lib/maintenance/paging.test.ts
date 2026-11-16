import { describe, expect, it } from 'vitest';
import { completion } from '#lib/test/maintenance-fixtures.js';
import {
  addCompletion,
  appendPage,
  completionPageSize,
  completionQuery,
  removeCompletion,
  replaceCompletion
} from './paging.js';

const a = completion({ id: 1, done_at: '2026-11-03T09:00:00Z' });
const b = completion({ id: 2, done_at: '2026-11-02T09:00:00Z' });
const c = completion({ id: 3, done_at: '2026-11-01T09:00:00Z' });

describe('completionQuery', () => {
  it('asks for the first page without a cursor and later pages with one', () => {
    expect(completionQuery(null)).toEqual({ limit: completionPageSize });
    expect(completionQuery('abc', 20)).toEqual({ limit: 20, cursor: 'abc' });
  });
});

describe('appendPage', () => {
  it('adds a page after the entries shown and skips any that are already there', () => {
    const shown = appendPage([a, b], { items: [b, c], next_cursor: null });
    expect(shown.map((x) => x.id)).toEqual([1, 2, 3]);
  });
});

describe('replaceCompletion', () => {
  it('puts the saved entry in place and restores newest first when its time moved', () => {
    const moved = { ...c, done_at: '2026-11-05T09:00:00Z', note: 'later' };
    const shown = replaceCompletion([a, b, c], moved);
    expect(shown.map((x) => x.id)).toEqual([3, 1, 2]);
    expect(shown[0].note).toBe('later');
  });

  it('breaks equal times by id', () => {
    const twin = { ...b, done_at: a.done_at };
    expect(replaceCompletion([a, b, c], twin).map((x) => x.id)).toEqual([2, 1, 3]);
  });
});

describe('removeCompletion', () => {
  it('drops the entry', () => {
    expect(removeCompletion([a, b, c], 2).map((x) => x.id)).toEqual([1, 3]);
  });
});

describe('addCompletion', () => {
  it('inserts a new entry in order and ignores one already shown', () => {
    const fresh = completion({ id: 9, done_at: '2026-11-04T09:00:00Z' });
    expect(addCompletion([a, b], fresh).map((x) => x.id)).toEqual([9, 1, 2]);
    expect(addCompletion([a, b], a).map((x) => x.id)).toEqual([1, 2]);
  });
});
