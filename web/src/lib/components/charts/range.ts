import { fromLocalInput, todayIn } from '#lib/format/local-time.js';

export type RangeId = '30d' | '90d' | '1y' | 'all';

export const rangeIds: readonly RangeId[] = ['30d', '90d', '1y', 'all'];

function daysInMonth(year: number, month: number): number {
  return new Date(Date.UTC(year, month, 0)).getUTCDate();
}

/**
 * Where a range button starts: 30 d, 90 d and 1 y begin at the start of that calendar day in the zone, and all has no start.
 */
export function rangeStart(range: RangeId, now: Date, timeZone: string): Date | null {
  if (range === 'all') return null;
  const [year, month, day] = todayIn(timeZone, now).split('-').map(Number);
  let date: Date;
  if (range === '1y') {
    // Feb 29 has no counterpart a year earlier, so it falls back to Feb 28.
    date = new Date(Date.UTC(year - 1, month - 1, Math.min(day, daysInMonth(year - 1, month))));
  } else {
    date = new Date(Date.UTC(year, month - 1, day - (range === '30d' ? 30 : 90)));
  }
  const start = fromLocalInput(`${date.toISOString().slice(0, 10)}T00:00`, timeZone);
  return start === null ? null : new Date(start);
}
