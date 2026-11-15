export type ScheduleKind = 'none' | 'after_last' | 'every_n_days' | 'weekly' | 'monthly';

export const scheduleKinds: readonly ScheduleKind[] = [
  'none',
  'after_last',
  'every_n_days',
  'weekly',
  'monthly'
];

/** The shared schedule columns as the API sends and accepts them. */
export interface Schedule {
  kind: ScheduleKind;
  interval_days: number | null;
  /** Bitmask with Monday as 1 up to Sunday as 64. */
  weekdays: number | null;
  interval_weeks: number;
  month_day: number | null;
  /** First occurrence date as YYYY-MM-DD. */
  anchor_date: string | null;
  /** Local time of day as HH:mm. */
  due_time: string;
}

export const defaultDueTime = '09:00';
export const everyDay = 127;
export const maxIntervalDays = 36500;
export const maxIntervalWeeks = 520;

/** Monday first, matching the bit order of the weekdays mask. */
export const weekdayBits = [1, 2, 4, 8, 16, 32, 64] as const;

export function emptySchedule(dueTime: string = defaultDueTime): Schedule {
  return {
    kind: 'none',
    interval_days: null,
    weekdays: null,
    interval_weeks: 1,
    month_day: null,
    anchor_date: null,
    due_time: dueTime
  };
}

const datePattern = /^(\d{4})-(\d{2})-(\d{2})$/;

function parseDate(text: string): Date | null {
  const match = datePattern.exec(text);
  if (!match) return null;
  const [year, month, day] = [Number(match[1]), Number(match[2]), Number(match[3])];
  const date = new Date(Date.UTC(year, month - 1, day));
  const real =
    date.getUTCFullYear() === year && date.getUTCMonth() === month - 1 && date.getUTCDate() === day;
  return real ? date : null;
}

/** The weekday bit of a YYYY-MM-DD date, or null when the text is not a date. */
export function weekdayBitOf(date: string): number | null {
  const parsed = parseDate(date);
  if (!parsed) return null;
  const day = parsed.getUTCDay();
  return day === 0 ? 64 : 1 << (day - 1);
}

function dayOfMonth(date: string): number | null {
  return parseDate(date)?.getUTCDate() ?? null;
}

/** Clears the columns the kind does not use, as the server stores them. */
export function normalizeSchedule(s: Schedule): Schedule {
  const out = emptySchedule(s.due_time);
  out.kind = s.kind;
  switch (s.kind) {
    case 'after_last':
      out.interval_days = s.interval_days;
      break;
    case 'every_n_days':
      out.interval_days = s.interval_days;
      out.anchor_date = s.anchor_date;
      break;
    case 'weekly':
      out.weekdays = s.weekdays;
      out.interval_weeks = s.interval_weeks;
      if (s.interval_weeks > 1) out.anchor_date = s.anchor_date;
      break;
    case 'monthly':
      out.month_day = s.month_day;
      break;
  }
  return out;
}

/** Switches the kind and fills the columns it needs, starting from the values already entered and today's date (YYYY-MM-DD) for the rest. */
export function changeKind(s: Schedule, kind: ScheduleKind, today: string): Schedule {
  const next: Schedule = { ...s, kind };
  switch (kind) {
    case 'after_last':
      next.interval_days ??= 7;
      break;
    case 'every_n_days':
      next.interval_days ??= 7;
      next.anchor_date ??= today;
      break;
    case 'weekly':
      next.weekdays ||= weekdayBitOf(today) ?? 1;
      if (next.interval_weeks > 1) next.anchor_date ??= today;
      break;
    case 'monthly':
      next.month_day ??= dayOfMonth(today) ?? 1;
      break;
  }
  return normalizeSchedule(next);
}

/** Switches one weekday on or off; an empty selection is stored as null. */
export function toggleWeekday(s: Schedule, bit: number): Schedule {
  const mask = (s.weekdays ?? 0) ^ bit;
  return { ...s, weekdays: mask === 0 ? null : mask };
}

/** Sets the repeat interval in weeks; repeating every second week or more needs a first date, which today (YYYY-MM-DD) provides. */
export function setIntervalWeeks(s: Schedule, weeks: number, today: string): Schedule {
  const next = { ...s, interval_weeks: weeks };
  if (weeks > 1) next.anchor_date ??= today;
  return next;
}

export type ScheduleProblem =
  'interval_days' | 'weekdays' | 'interval_weeks' | 'month_day' | 'anchor_date' | 'due_time';

function inRange(value: number | null, min: number, max: number): boolean {
  return value !== null && Number.isInteger(value) && value >= min && value <= max;
}

/** The fields of a schedule the server would refuse, so a form can point at them before sending. */
export function scheduleProblems(s: Schedule): ScheduleProblem[] {
  const out: ScheduleProblem[] = [];
  if (s.kind === 'none') return out;
  if (!/^([01]\d|2[0-3]):[0-5]\d$/.test(s.due_time)) out.push('due_time');
  switch (s.kind) {
    case 'after_last':
      if (!inRange(s.interval_days, 1, maxIntervalDays)) out.push('interval_days');
      break;
    case 'every_n_days':
      if (!inRange(s.interval_days, 1, maxIntervalDays)) out.push('interval_days');
      if (s.anchor_date === null || parseDate(s.anchor_date) === null) out.push('anchor_date');
      break;
    case 'weekly':
      if (!inRange(s.weekdays, 1, everyDay)) out.push('weekdays');
      if (!inRange(s.interval_weeks, 1, maxIntervalWeeks)) out.push('interval_weeks');
      else if (
        s.interval_weeks > 1 &&
        (s.anchor_date === null || parseDate(s.anchor_date) === null)
      )
        out.push('anchor_date');
      break;
    case 'monthly':
      if (!inRange(s.month_day, 1, 31)) out.push('month_day');
      break;
  }
  return out;
}

/** True when two schedules store the same columns. */
export function sameSchedule(a: Schedule, b: Schedule): boolean {
  return JSON.stringify(normalizeSchedule(a)) === JSON.stringify(normalizeSchedule(b));
}

/** The weekday names selected in the mask, Monday first, in the given locale. */
export function weekdayNames(
  mask: number | null,
  locale: string,
  style: 'short' | 'long'
): string[] {
  const format = new Intl.DateTimeFormat(locale, { weekday: style, timeZone: 'UTC' });
  // 2024-01-01 was a Monday.
  return weekdayBits
    .map((bit, i) => ({ bit, name: format.format(new Date(Date.UTC(2024, 0, 1 + i))) }))
    .filter(({ bit }) => ((mask ?? 0) & bit) !== 0)
    .map(({ name }) => name);
}
