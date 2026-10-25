import { describe, expect, it } from 'vitest';
import type { TankParameter } from '#lib/api/params.js';
import { emptySchedule, type Schedule } from '#lib/schedule/schedule.js';
import {
  draftFromParameter,
  orderParameters,
  parameterChanges,
  reorderChanges
} from './parameter-settings.js';

const after7: Schedule = { ...emptySchedule(), kind: 'after_last', interval_days: 7 };

function parameter(patch: Partial<TankParameter> = {}): TankParameter {
  return {
    parameter_id: 1,
    preset_key: 'ph',
    name: 'pH',
    unit_family: 'fixed',
    unit: '',
    unit_label: '',
    decimals: 1,
    configured: true,
    enabled: true,
    ideal_low: 7.9,
    ideal_high: 8.4,
    schedule: after7,
    sort_order: 30,
    latest: null,
    next_due: null,
    overdue: false,
    ...patch
  };
}

const slot = (parameter_id: number, sort_order: number) => ({ parameter_id, sort_order });

describe('orderParameters', () => {
  it('sorts by sort order and breaks ties by id', () => {
    const list = [slot(3, 20), slot(2, 20), slot(1, 30)];
    expect(orderParameters(list).map((p) => p.parameter_id)).toEqual([2, 3, 1]);
  });
});

describe('reorderChanges', () => {
  it("swaps two neighbours onto each other's slots", () => {
    const ordered = [slot(1, 10), slot(2, 20), slot(3, 30)];
    expect(reorderChanges(ordered, 2, 1)).toEqual([
      { parameterId: 3, sortOrder: 20 },
      { parameterId: 2, sortOrder: 30 }
    ]);
  });

  it('gives tied parameters distinct slots so a move always shows', () => {
    const ordered = [slot(1, 10), slot(2, 10), slot(3, 10)];
    expect(reorderChanges(ordered, 0, 1)).toEqual([
      { parameterId: 1, sortOrder: 20 },
      { parameterId: 3, sortOrder: 30 }
    ]);
  });

  it('does nothing for a move that leaves the list or stays in place', () => {
    const ordered = [slot(1, 10), slot(2, 20)];
    expect(reorderChanges(ordered, 0, -1)).toEqual([]);
    expect(reorderChanges(ordered, 1, 2)).toEqual([]);
    expect(reorderChanges(ordered, 1, 1)).toEqual([]);
  });
});

describe('parameterChanges', () => {
  it('is empty for an untouched draft', () => {
    const p = parameter();
    expect(parameterChanges(p, draftFromParameter(p))).toEqual({
      changes: {},
      errors: {},
      schedule: []
    });
  });

  it('sends changed limits as numbers, reading a decimal comma, and clears an emptied one', () => {
    const p = parameter();
    const draft = { ...draftFromParameter(p), low: '7,8', high: '' };
    expect(parameterChanges(p, draft).changes).toEqual({ ideal_low: 7.8, ideal_high: null });
  });

  it('sends the whole schedule, with unused columns cleared, only when it changed', () => {
    const p = parameter();
    const draft = {
      ...draftFromParameter(p),
      schedule: { ...after7, interval_days: 14, month_day: 5 }
    };
    expect(parameterChanges(p, draft).changes).toEqual({
      schedule: { ...after7, interval_days: 14 }
    });
  });

  it('reports limits that are not numbers, a reversed range and an incomplete schedule', () => {
    const p = parameter();
    expect(parameterChanges(p, { ...draftFromParameter(p), low: 'x' }).errors).toEqual({
      ideal_low: 'invalid'
    });
    expect(parameterChanges(p, { ...draftFromParameter(p), high: '1,x' }).errors).toEqual({
      ideal_high: 'invalid'
    });
    expect(parameterChanges(p, { ...draftFromParameter(p), low: '9' }).errors).toEqual({
      range: 'out_of_range'
    });
    const schedule = { ...after7, interval_days: null };
    const edit = parameterChanges(p, { ...draftFromParameter(p), schedule });
    expect(edit.schedule).toEqual(['interval_days']);
    expect(edit.changes).toEqual({});
  });
});
