import { describe, expect, it } from 'vitest';
import { fromLocalInput, todayIn, toLocalInput } from './local-time.js';

describe('toLocalInput', () => {
  it('writes the wall clock of the zone', () => {
    expect(toLocalInput('2026-07-01T10:30:00Z', 'Europe/Berlin')).toBe('2026-07-01T12:30');
    expect(toLocalInput('2026-01-01T10:30:00Z', 'Europe/Berlin')).toBe('2026-01-01T11:30');
    expect(toLocalInput('2026-01-01T03:00:00Z', 'America/New_York')).toBe('2025-12-31T22:00');
  });

  it('shows midnight as 00 and not 24', () => {
    expect(toLocalInput('2026-01-01T00:00:00Z', 'UTC')).toBe('2026-01-01T00:00');
  });

  it('is empty for an unreadable time', () => {
    expect(toLocalInput('nonsense', 'UTC')).toBe('');
  });
});

describe('todayIn', () => {
  it('uses the date in the zone, which can differ from the UTC date', () => {
    const instant = Date.UTC(2026, 9, 24, 23, 30);
    expect(todayIn('UTC', instant)).toBe('2026-10-24');
    expect(todayIn('Asia/Tokyo', instant)).toBe('2026-10-25');
    expect(todayIn('America/Los_Angeles', instant)).toBe('2026-10-24');
  });
});

describe('fromLocalInput', () => {
  it('converts a wall clock time to a UTC instant', () => {
    expect(fromLocalInput('2026-07-01T12:30', 'Europe/Berlin')).toBe('2026-07-01T10:30:00Z');
    expect(fromLocalInput('2026-01-01T11:30', 'Europe/Berlin')).toBe('2026-01-01T10:30:00Z');
    expect(fromLocalInput('2025-12-31T22:00', 'America/New_York')).toBe('2026-01-01T03:00:00Z');
    expect(fromLocalInput('2026-01-01T00:00', 'UTC')).toBe('2026-01-01T00:00:00Z');
  });

  it('accepts seconds', () => {
    expect(fromLocalInput('2026-01-01T11:30:15', 'Europe/Berlin')).toBe('2026-01-01T10:30:15Z');
  });

  it('round trips through toLocalInput on either side of a clock change', () => {
    for (const local of [
      '2026-03-29T01:59',
      '2026-03-29T03:00',
      '2026-10-25T01:59',
      '2026-10-25T03:00'
    ]) {
      const instant = fromLocalInput(local, 'Europe/Berlin');
      expect(instant, local).not.toBeNull();
      expect(toLocalInput(instant ?? '', 'Europe/Berlin'), local).toBe(local);
    }
  });

  it('moves a time skipped by the spring clock change forward', () => {
    // 02:30 does not exist in Berlin on 2026-03-29.
    expect(fromLocalInput('2026-03-29T02:30', 'Europe/Berlin')).toBe('2026-03-29T01:30:00Z');
  });

  it('refuses text that is not a real date and time', () => {
    for (const text of [
      '',
      'today',
      '2026-02-30T10:00',
      '2026-13-01T10:00',
      '2026-01-01T24:00',
      '2026-01-01',
      '2026-01-01T10:60'
    ]) {
      expect(fromLocalInput(text, 'UTC'), text).toBeNull();
    }
  });
});
