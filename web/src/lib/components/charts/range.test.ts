import { describe, expect, it } from 'vitest';
import { rangeIds, rangeStart } from './range.js';

const iso = (d: Date | null) => d?.toISOString();

describe('rangeStart', () => {
  it('offers the four buttons in order', () => {
    expect(rangeIds).toEqual(['30d', '90d', '1y', 'all']);
  });

  it('has no start for all', () => {
    expect(rangeStart('all', new Date(), 'America/New_York')).toBeNull();
  });

  it('starts at local midnight of the calendar day, across a daylight saving change', () => {
    // 23:30 on Mar 9 in New York, a day after the clocks went forward.
    const now = new Date('2026-03-10T03:30:00Z');
    expect(iso(rangeStart('30d', now, 'America/New_York'))).toBe('2026-02-07T05:00:00.000Z');
    expect(iso(rangeStart('90d', now, 'America/New_York'))).toBe('2025-12-09T05:00:00.000Z');
    expect(iso(rangeStart('1y', now, 'America/New_York'))).toBe('2025-03-09T05:00:00.000Z');
  });

  it('uses the zone calendar day when it differs from the UTC day', () => {
    // 01:00 on Mar 10 in Auckland is still Mar 9 in UTC.
    const now = new Date('2026-03-09T12:00:00Z');
    expect(iso(rangeStart('30d', now, 'Pacific/Auckland'))).toBe('2026-02-07T11:00:00.000Z');
    // 20:00 on Mar 9 in Los Angeles is already Mar 10 in UTC.
    expect(iso(rangeStart('30d', new Date('2026-03-10T03:00:00Z'), 'America/Los_Angeles'))).toBe(
      '2026-02-07T08:00:00.000Z'
    );
  });

  it('finds local midnight on the day daylight saving ends, where UTC midnight is already past the change', () => {
    // Auckland leaves daylight saving at 03:00 on Apr 5, so that midnight is still +13 but 00:00 UTC is already +12.
    const now = new Date('2026-05-05T00:00:00Z');
    expect(iso(rangeStart('30d', now, 'Pacific/Auckland'))).toBe('2026-04-04T11:00:00.000Z');
  });

  it('handles a zone with a half-hour offset', () => {
    const now = new Date('2026-06-15T20:00:00Z');
    // 01:30 on Jun 16 in Kolkata, so 30 days back is May 17.
    expect(iso(rangeStart('30d', now, 'Asia/Kolkata'))).toBe('2026-05-16T18:30:00.000Z');
  });

  it('falls back to Feb 28 when a year before Feb 29 does not exist', () => {
    const now = new Date('2028-02-29T15:00:00Z');
    expect(iso(rangeStart('1y', now, 'Europe/Berlin'))).toBe('2027-02-27T23:00:00.000Z');
  });

  it('matches plain UTC date math when the zone is UTC', () => {
    const now = new Date('2026-01-31T10:00:00Z');
    expect(iso(rangeStart('30d', now, 'UTC'))).toBe('2026-01-01T00:00:00.000Z');
  });
});
