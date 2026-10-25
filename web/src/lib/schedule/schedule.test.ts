import { describe, expect, it } from 'vitest';
import { describeSchedule } from './describe.js';
import {
  changeKind,
  emptySchedule,
  normalizeSchedule,
  sameSchedule,
  scheduleProblems,
  setIntervalWeeks,
  toggleWeekday,
  weekdayBitOf,
  weekdayNames,
  type Schedule
} from './schedule.js';

// 2026-10-24 is a Saturday.
const today = '2026-10-24';

const schedule = (patch: Partial<Schedule>): Schedule => ({ ...emptySchedule(), ...patch });

describe('weekdayBitOf', () => {
  it('uses Monday as 1 up to Sunday as 64', () => {
    expect(weekdayBitOf('2026-10-19')).toBe(1);
    expect(weekdayBitOf('2026-10-24')).toBe(32);
    expect(weekdayBitOf('2026-10-25')).toBe(64);
  });

  it('is null for text that is not a date', () => {
    expect(weekdayBitOf('2026-02-30')).toBeNull();
    expect(weekdayBitOf('soon')).toBeNull();
  });
});

describe('changeKind', () => {
  it('fills the columns the new kind needs and clears the rest', () => {
    expect(changeKind(emptySchedule(), 'after_last', today)).toEqual(
      schedule({ kind: 'after_last', interval_days: 7 })
    );
    expect(changeKind(emptySchedule(), 'every_n_days', today)).toEqual(
      schedule({ kind: 'every_n_days', interval_days: 7, anchor_date: today })
    );
    expect(changeKind(emptySchedule(), 'weekly', today)).toEqual(
      schedule({ kind: 'weekly', weekdays: 32 })
    );
    expect(changeKind(emptySchedule(), 'monthly', today)).toEqual(
      schedule({ kind: 'monthly', month_day: 24 })
    );
    expect(changeKind(schedule({ kind: 'monthly', month_day: 3 }), 'none', today)).toEqual(
      emptySchedule()
    );
  });

  it('keeps an interval already entered and the due time', () => {
    const from = schedule({ kind: 'after_last', interval_days: 14, due_time: '18:30' });
    expect(changeKind(from, 'every_n_days', today)).toEqual(
      schedule({ kind: 'every_n_days', interval_days: 14, anchor_date: today, due_time: '18:30' })
    );
  });
});

describe('toggleWeekday', () => {
  it('flips one bit and stores an empty selection as null', () => {
    const base = schedule({ kind: 'weekly', weekdays: 5 });
    expect(toggleWeekday(base, 2).weekdays).toBe(7);
    expect(toggleWeekday(base, 4).weekdays).toBe(1);
    expect(toggleWeekday(schedule({ kind: 'weekly', weekdays: 1 }), 1).weekdays).toBeNull();
  });
});

describe('setIntervalWeeks', () => {
  it('asks for a start date once the repeat is longer than a week', () => {
    const base = schedule({ kind: 'weekly', weekdays: 32 });
    expect(setIntervalWeeks(base, 2, today)).toMatchObject({
      interval_weeks: 2,
      anchor_date: today
    });
    expect(setIntervalWeeks(base, 1, today).anchor_date).toBeNull();
  });
});

describe('normalizeSchedule', () => {
  it('drops the columns the kind ignores, as the server stores it', () => {
    const messy = schedule({
      kind: 'weekly',
      interval_days: 9,
      weekdays: 5,
      interval_weeks: 1,
      month_day: 4,
      anchor_date: today,
      due_time: '08:00'
    });
    expect(normalizeSchedule(messy)).toEqual(
      schedule({ kind: 'weekly', weekdays: 5, due_time: '08:00' })
    );
    expect(normalizeSchedule({ ...messy, interval_weeks: 3 }).anchor_date).toBe(today);
  });
});

describe('scheduleProblems', () => {
  it('accepts a none schedule whatever else it holds', () => {
    expect(scheduleProblems(schedule({ due_time: 'nope' }))).toEqual([]);
  });

  it('flags what each kind needs', () => {
    expect(scheduleProblems(schedule({ kind: 'after_last' }))).toEqual(['interval_days']);
    expect(scheduleProblems(schedule({ kind: 'after_last', interval_days: 0 }))).toEqual([
      'interval_days'
    ]);
    expect(scheduleProblems(schedule({ kind: 'after_last', interval_days: 7 }))).toEqual([]);
    expect(scheduleProblems(schedule({ kind: 'every_n_days', interval_days: 3 }))).toEqual([
      'anchor_date'
    ]);
    expect(scheduleProblems(schedule({ kind: 'weekly' }))).toEqual(['weekdays']);
    expect(scheduleProblems(schedule({ kind: 'weekly', weekdays: 1, interval_weeks: 2 }))).toEqual([
      'anchor_date'
    ]);
    expect(
      scheduleProblems(
        schedule({ kind: 'weekly', weekdays: 1, interval_weeks: 2, anchor_date: today })
      )
    ).toEqual([]);
    expect(scheduleProblems(schedule({ kind: 'monthly', month_day: 32 }))).toEqual(['month_day']);
    expect(scheduleProblems(schedule({ kind: 'monthly', month_day: 31 }))).toEqual([]);
    expect(
      scheduleProblems(schedule({ kind: 'monthly', month_day: 1, due_time: '25:00' }))
    ).toEqual(['due_time']);
  });
});

describe('sameSchedule', () => {
  it('ignores columns the kind does not use', () => {
    const a = schedule({ kind: 'after_last', interval_days: 7 });
    expect(sameSchedule(a, { ...a, month_day: 12 })).toBe(true);
    expect(sameSchedule(a, { ...a, interval_days: 8 })).toBe(false);
  });
});

describe('weekdayNames', () => {
  it('lists the chosen days Monday first', () => {
    expect(weekdayNames(1 | 4 | 64, 'en', 'short')).toEqual(['Mon', 'Wed', 'Sun']);
    expect(weekdayNames(null, 'en', 'short')).toEqual([]);
  });
});

describe('describeSchedule', () => {
  it('summarises each kind in words', () => {
    expect(describeSchedule(emptySchedule())).toBe('No schedule');
    expect(describeSchedule(schedule({ kind: 'after_last', interval_days: 1 }))).toBe(
      '1 day after the last time at 09:00'
    );
    expect(describeSchedule(schedule({ kind: 'after_last', interval_days: 7 }))).toBe(
      '7 days after the last time at 09:00'
    );
    expect(
      describeSchedule(schedule({ kind: 'every_n_days', interval_days: 3, anchor_date: today }))
    ).toBe('Every 3 days from 2026-10-24 at 09:00');
    expect(describeSchedule(schedule({ kind: 'weekly', weekdays: 5, due_time: '18:00' }))).toBe(
      'Every week on Mon, Wed at 18:00'
    );
    expect(
      describeSchedule(
        schedule({ kind: 'weekly', weekdays: 64, interval_weeks: 2, anchor_date: today })
      )
    ).toBe('Every 2 weeks on Sun, starting 2026-10-24 at 09:00');
    expect(describeSchedule(schedule({ kind: 'monthly', month_day: 1 }))).toBe(
      'On day 1 of each month at 09:00'
    );
  });
});
