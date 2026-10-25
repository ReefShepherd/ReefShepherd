import { describe, expect, it } from 'vitest';
import { emptyValue, formatNumber, formatValue } from './number.js';

describe('formatNumber', () => {
  it('keeps exactly the requested decimals', () => {
    expect(formatNumber(26, 1)).toBe('26.0');
    expect(formatNumber(8.2, 2)).toBe('8.20');
    expect(formatNumber(1.02637, 4)).toBe('1.0264');
    expect(formatNumber(7.56, 0)).toBe('8');
  });

  it('groups thousands', () => {
    expect(formatNumber(1234.5, 1)).toBe('1,234.5');
  });

  it('never shows a minus sign for a value that rounds to zero', () => {
    expect(formatNumber(-0.04, 1)).toBe('0.0');
    expect(formatNumber(-0.6, 1)).toBe('-0.6');
  });

  it('clamps unreasonable decimals', () => {
    expect(formatNumber(1.5, -3)).toBe('2');
    expect(() => formatNumber(1.5, 500)).not.toThrow();
  });

  it('shows the empty marker for non-finite numbers', () => {
    expect(formatNumber(Number.NaN, 1)).toBe(emptyValue);
    expect(formatNumber(Infinity, 1)).toBe(emptyValue);
  });
});

describe('formatValue', () => {
  it('puts the unit after a no-break space', () => {
    expect(formatValue(25.5, '°C', 1)).toBe('25.5\u00a0°C');
    expect(formatValue(420, 'ppm', 0)).toBe('420\u00a0ppm');
  });

  it('shows a bare number when there is no unit', () => {
    expect(formatValue(8.2, '', 2)).toBe('8.20');
    expect(formatValue(8.2, null, 2)).toBe('8.20');
    expect(formatValue(8.2, undefined, 2)).toBe('8.20');
  });

  it('shows the empty marker, without a unit, for a missing value', () => {
    expect(formatValue(null, '°C', 1)).toBe(emptyValue);
    expect(formatValue(undefined, '°C', 1)).toBe(emptyValue);
    expect(formatValue(Number.NaN, '°C', 1)).toBe(emptyValue);
  });
});
