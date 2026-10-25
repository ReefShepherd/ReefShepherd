import { describe, expect, it } from 'vitest';
import { formatDateTime, relativeTime } from './datetime.js';
import { emptyValue } from './number.js';

// Newer ICU versions put a narrow no-break space before AM and PM.
const plain = (text: string) => text.replace(/\s/g, ' ');

describe('formatDateTime', () => {
  it('renders the same instant in the requested timezone', () => {
    const iso = '2026-03-08T12:00:00Z';
    expect(plain(formatDateTime(iso, 'UTC'))).toBe('Mar 8, 2026, 12:00 PM');
    expect(plain(formatDateTime(iso, 'America/New_York'))).toBe('Mar 8, 2026, 8:00 AM');
    expect(plain(formatDateTime(iso, 'Pacific/Auckland'))).toBe('Mar 9, 2026, 1:00 AM');
  });

  it('keeps a separate format for each zone', () => {
    const iso = '2026-01-01T00:00:00Z';
    expect(plain(formatDateTime(iso, 'Asia/Tokyo'))).toBe('Jan 1, 2026, 9:00 AM');
    expect(plain(formatDateTime(iso, 'UTC'))).toBe('Jan 1, 2026, 12:00 AM');
    expect(plain(formatDateTime(iso, 'Asia/Tokyo'))).toBe('Jan 1, 2026, 9:00 AM');
  });

  it('shows the empty marker for an invalid timestamp', () => {
    expect(formatDateTime('nope', 'UTC')).toBe(emptyValue);
  });
});

describe('relativeTime', () => {
  const now = Date.parse('2026-06-15T12:00:00Z');
  const at = (offsetSeconds: number) => new Date(now + offsetSeconds * 1000).toISOString();

  it('describes the past', () => {
    expect(relativeTime(at(-30), now)).toBe('30 seconds ago');
    expect(relativeTime(at(-5 * 60), now)).toBe('5 minutes ago');
    expect(relativeTime(at(-3 * 3600), now)).toBe('3 hours ago');
    expect(relativeTime(at(-86400), now)).toBe('yesterday');
    expect(relativeTime(at(-45 * 86400), now)).toBe('last month');
    expect(relativeTime(at(-800 * 86400), now)).toBe('2 years ago');
  });

  it('describes the future', () => {
    expect(relativeTime(at(2 * 86400), now)).toBe('in 2 days');
    expect(relativeTime(at(90), now)).toBe('in 1 minute');
  });

  it('rounds toward zero so 59 minutes is not an hour', () => {
    expect(relativeTime(at(-59 * 60 - 30), now)).toBe('59 minutes ago');
  });

  it('says now for the current instant', () => {
    expect(relativeTime(at(0), now)).toBe('now');
  });

  it('accepts a Date for now and shows the empty marker for bad input', () => {
    expect(relativeTime(at(-120), new Date(now))).toBe('2 minutes ago');
    expect(relativeTime('nope', now)).toBe(emptyValue);
  });
});
