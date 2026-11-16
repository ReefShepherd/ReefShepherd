import { describe, expect, it } from 'vitest';
import { task } from '#lib/test/maintenance-fixtures.js';
import {
  cardRows,
  dueState,
  groupByTank,
  inSegment,
  removeTask,
  replaceTask,
  segmentCounts,
  segmentTasks,
  sortTasks
} from './grouping.js';

// It is 09:30 on the 16th in Auckland (NZDT, UTC+13) while it is still the 15th in UTC and in New York.
const auckland = 'Pacific/Auckland';
const now = new Date('2026-11-15T20:30:00Z');

describe('dueState', () => {
  it('calls this evening in the zone due today although the UTC date is already the next one', () => {
    // 2026-11-16T05:00Z is 18:00 on the 16th in Auckland and the 16th in UTC, while now is still the 15th in UTC.
    expect(dueState(task({ next_due_at: '2026-11-16T05:00:00Z' }), now, auckland)).toBe('today');
  });

  it('puts a due time after local midnight on the next day', () => {
    // 2026-11-16T14:00Z is 03:00 on the 17th in Auckland.
    expect(dueState(task({ next_due_at: '2026-11-16T14:00:00Z' }), now, auckland)).toBe('later');
  });

  it('reads the zone behind UTC as well', () => {
    const evening = new Date('2026-11-16T03:30:00Z');
    // 22:30 on the 15th in New York, the 16th in UTC: 06:00Z is 01:00 on the 16th there, a later day.
    const zone = 'America/New_York';
    expect(dueState(task({ next_due_at: '2026-11-16T04:00:00Z' }), evening, zone)).toBe('today');
    expect(dueState(task({ next_due_at: '2026-11-16T06:00:00Z' }), evening, zone)).toBe('later');
  });

  it('flags a due time that has passed, a missing one and a paused task', () => {
    expect(dueState(task({ next_due_at: '2026-11-15T20:00:00Z' }), now, auckland)).toBe('overdue');
    expect(dueState(task({ next_due_at: null }), now, auckland)).toBe('unscheduled');
    expect(
      dueState(task({ next_due_at: '2026-11-01T00:00:00Z', paused: true }), now, auckland)
    ).toBe('paused');
  });
});

describe('segments', () => {
  const overdue = task({ id: 1, name: 'A', next_due_at: '2026-11-10T00:00:00Z' });
  const today = task({ id: 2, name: 'B', next_due_at: '2026-11-16T10:00:00Z' });
  const soon = task({ id: 3, name: 'C', next_due_at: '2026-11-17T00:00:00Z' });
  const never = task({ id: 4, name: 'D', next_due_at: null });
  const paused = task({ id: 5, name: 'E', next_due_at: '2026-11-10T00:00:00Z', paused: true });
  const all = [soon, never, paused, today, overdue];

  it('lists what is due by the end of the local day, overdue included', () => {
    expect(segmentTasks(all, 'due', now, auckland).map((t) => t.id)).toEqual([1, 2]);
  });

  it('lists what is due after today, without unscheduled or paused tasks', () => {
    expect(segmentTasks(all, 'upcoming', now, auckland).map((t) => t.id)).toEqual([3]);
    expect(inSegment(never, 'upcoming', now, auckland)).toBe(false);
    expect(inSegment(paused, 'due', now, auckland)).toBe(false);
  });

  it('lists everything in all, soonest first, then unscheduled, then paused', () => {
    expect(segmentTasks(all, 'all', now, auckland).map((t) => t.id)).toEqual([1, 2, 3, 4, 5]);
  });

  it('counts each segment', () => {
    expect(segmentCounts(all, now, auckland)).toEqual({ due: 2, upcoming: 1, all: 5 });
  });

  it('keeps a task after local midnight out of due until the local day changes', () => {
    // 2026-11-16T14:00Z is 03:00 on the 17th in Auckland, while it is 23:00 on the 16th there at 10:00Z.
    const edge = task({ next_due_at: '2026-11-16T14:00:00Z' });
    expect(inSegment(edge, 'due', new Date('2026-11-16T10:00:00Z'), auckland)).toBe(false);
    expect(inSegment(edge, 'due', new Date('2026-11-16T11:30:00Z'), auckland)).toBe(true);
  });
});

describe('sortTasks', () => {
  it('orders by due time, then name, with unscheduled and paused last', () => {
    const sorted = sortTasks([
      task({ id: 1, name: 'b', next_due_at: '2026-11-20T00:00:00Z' }),
      task({ id: 2, name: 'a', next_due_at: '2026-11-20T00:00:00Z' }),
      task({ id: 3, name: 'c', next_due_at: null }),
      task({ id: 4, name: 'd', next_due_at: '2026-11-01T00:00:00Z', paused: true }),
      task({ id: 5, name: 'e', next_due_at: '2026-11-19T00:00:00Z' })
    ]);
    expect(sorted.map((t) => t.id)).toEqual([5, 2, 1, 3, 4]);
  });
});

describe('groupByTank', () => {
  it('groups by tank name with the whole system last and keeps the order inside a group', () => {
    const sections = groupByTank([
      task({ id: 1, tank_id: null, tank_name: null }),
      task({ id: 2, tank_id: 8, tank_name: 'Nano' }),
      task({ id: 3, tank_id: 3, tank_name: 'Display' }),
      task({ id: 4, tank_id: 8, tank_name: 'Nano' })
    ]);
    expect(sections.map((s) => [s.tankId, s.tasks.map((t) => t.id)])).toEqual([
      [3, [3]],
      [8, [2, 4]],
      [null, [1]]
    ]);
  });

  it('leaves out empty groups', () => {
    expect(groupByTank([])).toEqual([]);
    expect(groupByTank([task({ tank_id: null, tank_name: null })]).map((s) => s.tankId)).toEqual([
      null
    ]);
  });
});

describe('replaceTask and removeTask', () => {
  it('replaces a task in place, adds an unknown one and removes by id', () => {
    const a = task({ id: 1 });
    const b = task({ id: 2 });
    expect(replaceTask([a, b], { ...a, name: 'x' }).map((t) => t.name)).toEqual(['x', b.name]);
    expect(replaceTask([a], b).map((t) => t.id)).toEqual([1, 2]);
    expect(removeTask([a, b], 1).map((t) => t.id)).toEqual([2]);
  });
});

describe('cardRows', () => {
  it('shows at most the limit of due tasks, soonest first, and counts the rest', () => {
    const tasks = Array.from({ length: 7 }, (_, i) =>
      task({
        id: i + 1,
        name: `T${i}`,
        next_due_at: `2026-11-${String(1 + i).padStart(2, '0')}T00:00:00Z`
      })
    );
    tasks.push(task({ id: 99, next_due_at: '2026-12-30T00:00:00Z' }));
    const rows = cardRows(tasks, now, auckland);
    expect(rows.shown.map((t) => t.id)).toEqual([1, 2, 3, 4, 5]);
    expect(rows.more).toBe(2);
  });

  it('has no more when everything fits', () => {
    expect(cardRows([task({ next_due_at: '2026-11-01T00:00:00Z' })], now, auckland).more).toBe(0);
  });
});
