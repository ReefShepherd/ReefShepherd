import { getLocale } from '#lib/paraglide/runtime.js';
import { emptyValue } from './number.js';

const dateTimeFormats = new Map<string, Intl.DateTimeFormat>();

/** Formats an RFC 3339 timestamp as a short date and time in the given IANA timezone. */
export function formatDateTime(iso: string, timeZone: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return emptyValue;
  const locale = getLocale();
  const key = `${locale}|${timeZone}`;
  let format = dateTimeFormats.get(key);
  if (!format) {
    format = new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short', timeZone });
    dateTimeFormats.set(key, format);
  }
  return format.format(date);
}

const relativeUnits: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 365 * 86400],
  ['month', 30 * 86400],
  ['day', 86400],
  ['hour', 3600],
  ['minute', 60]
];

/** Describes a timestamp relative to now, such as "3 hours ago" or "in 2 days". */
export function relativeTime(iso: string, now: Date | number = Date.now()): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return emptyValue;
  const seconds = (then - new Date(now).getTime()) / 1000;
  const format = new Intl.RelativeTimeFormat(getLocale(), { numeric: 'auto' });
  for (const [unit, size] of relativeUnits) {
    if (Math.abs(seconds) >= size) return format.format(Math.trunc(seconds / size), unit);
  }
  return format.format(Math.trunc(seconds), 'second');
}
