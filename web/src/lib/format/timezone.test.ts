import { describe, expect, it, vi } from 'vitest';
import { normalizeTimeZone, timeZoneNames } from './timezone.js';

describe('normalizeTimeZone', () => {
  it('accepts IANA names and trims them', () => {
    expect(normalizeTimeZone(' Europe/Berlin ')).toBe('Europe/Berlin');
    expect(normalizeTimeZone('UTC')).toBe('UTC');
  });

  it('returns the canonical spelling of a name typed in the wrong case', () => {
    expect(normalizeTimeZone('europe/berlin')).toBe('Europe/Berlin');
  });

  it('rejects blanks, Local and unknown names', () => {
    expect(normalizeTimeZone('')).toBeUndefined();
    expect(normalizeTimeZone('   ')).toBeUndefined();
    expect(normalizeTimeZone('Local')).toBeUndefined();
    expect(normalizeTimeZone('Mars/Olympus')).toBeUndefined();
  });
});

describe('timeZoneNames', () => {
  it('lists UTC once, first', () => {
    const names = timeZoneNames();
    expect(names[0]).toBe('UTC');
    expect(names.filter((n) => n === 'UTC')).toHaveLength(1);
    expect(names).toContain('Europe/Berlin');
  });

  it('does not repeat UTC when the browser already lists it', () => {
    const supported = vi.spyOn(Intl, 'supportedValuesOf').mockReturnValue(['Europe/Berlin', 'UTC']);
    try {
      expect(timeZoneNames()).toEqual(['UTC', 'Europe/Berlin']);
    } finally {
      supported.mockRestore();
    }
  });

  it('still offers UTC when the browser cannot list zones', () => {
    const supported = vi.spyOn(Intl, 'supportedValuesOf').mockImplementation(() => {
      throw new RangeError('unsupported');
    });
    try {
      expect(timeZoneNames()).toEqual(['UTC']);
    } finally {
      supported.mockRestore();
    }
  });
});
