import { describe, expect, it } from 'vitest';
import type { TankParameter } from '#lib/api/params.js';
import { classifyDue, rangeState } from './status.js';

const parameter = (id: number, extra: Partial<TankParameter> = {}): TankParameter =>
  ({
    parameter_id: id,
    name: `P${id}`,
    enabled: true,
    ideal_low: null,
    ideal_high: null,
    latest: null,
    next_due: null,
    overdue: false,
    ...extra
  }) as TankParameter;

const reading = (inRange: boolean) => ({
  result_id: 1,
  value: 8,
  measured_at: '2026-03-10T10:00:00Z',
  in_range: inRange
});

describe('rangeState', () => {
  it('is none without a reading or without any limit', () => {
    expect(rangeState(parameter(1, { ideal_low: 7, ideal_high: 9 }))).toBe('none');
    expect(rangeState(parameter(1, { latest: reading(true) }))).toBe('none');
  });

  it('follows the server verdict when a limit is set', () => {
    expect(rangeState(parameter(1, { ideal_low: 7, latest: reading(true) }))).toBe('in');
    expect(rangeState(parameter(1, { ideal_high: 9, latest: reading(false) }))).toBe('out');
  });
});

describe('classifyDue', () => {
  // 2026-03-10 23:30 in Los Angeles is already the next day in UTC.
  const now = new Date('2026-03-11T06:30:00Z');

  it('lists overdue parameters, longest overdue first', () => {
    const groups = classifyDue(
      [
        parameter(1, { overdue: true, next_due: '2026-03-09T20:00:00Z' }),
        parameter(2, { overdue: true, next_due: '2026-03-01T20:00:00Z' })
      ],
      now,
      'America/Los_Angeles'
    );
    expect(groups.overdue.map((p) => p.parameter_id)).toEqual([2, 1]);
    expect(groups.today).toEqual([]);
  });

  it('counts a test due later on the same calendar day in the instance zone as due today', () => {
    const groups = classifyDue(
      [
        parameter(1, { next_due: '2026-03-11T06:55:00Z' }),
        parameter(2, { next_due: '2026-03-11T06:40:00Z' })
      ],
      now,
      'America/Los_Angeles'
    );
    expect(groups.today.map((p) => p.parameter_id)).toEqual([2, 1]);
  });

  it('uses the zone for the day boundary, not UTC', () => {
    // 07:30 UTC is 00:30 the next day in Los Angeles but the same UTC day as now.
    const due = parameter(1, { next_due: '2026-03-11T07:30:00Z' });
    expect(classifyDue([due], now, 'America/Los_Angeles').today).toHaveLength(0);
    expect(classifyDue([due], now, 'UTC').today).toHaveLength(1);
  });

  it('ignores tests due on a later day, with no schedule, or switched off', () => {
    const groups = classifyDue(
      [
        parameter(1, { next_due: '2026-03-12T20:00:00Z' }),
        parameter(2),
        parameter(3, { enabled: false, overdue: true, next_due: '2026-03-01T00:00:00Z' }),
        parameter(4, { enabled: false, next_due: '2026-03-11T07:30:00Z' })
      ],
      now,
      'America/Los_Angeles'
    );
    expect(groups).toEqual({ overdue: [], today: [] });
  });

  it('puts an overdue parameter only in the overdue list', () => {
    const p = parameter(1, { overdue: true, next_due: '2026-03-11T05:00:00Z' });
    const groups = classifyDue([p], now, 'America/Los_Angeles');
    expect(groups.overdue).toHaveLength(1);
    expect(groups.today).toHaveLength(0);
  });
});
