import { describe, expect, it } from 'vitest';
import { decimalText, fractionDigits, parseDecimal, parseWholeNumber } from './decimal.js';

describe('parseDecimal', () => {
  it('reads a decimal point and a decimal comma the same way', () => {
    expect(parseDecimal('7.9')).toBe(7.9);
    expect(parseDecimal('7,9')).toBe(7.9);
    expect(parseDecimal(' 1035,5 ')).toBe(1035.5);
  });

  it('reads whole numbers, signs and bare fractions', () => {
    expect(parseDecimal('420')).toBe(420);
    expect(parseDecimal('-1,5')).toBe(-1.5);
    expect(parseDecimal('+2')).toBe(2);
    expect(parseDecimal('−1.5')).toBe(-1.5);
    expect(parseDecimal(',5')).toBe(0.5);
    expect(parseDecimal('8.')).toBe(8);
  });

  it('refuses text that is not a plain number, including grouping separators', () => {
    for (const text of [
      '',
      ' ',
      'abc',
      '7.9.1',
      '7,9,1',
      '1,234.5',
      '1.234,5',
      '1e3',
      '--1',
      '7 9',
      '.',
      ','
    ]) {
      expect(parseDecimal(text), text).toBeNull();
    }
  });
});

describe('decimalText', () => {
  it('writes a fixed number of decimals with a point', () => {
    expect(decimalText(8.2, 2)).toBe('8.20');
    expect(decimalText(1035, 0)).toBe('1035');
  });

  it('is empty for a missing value', () => {
    expect(decimalText(null, 1)).toBe('');
    expect(decimalText(undefined, 1)).toBe('');
    expect(decimalText(Number.NaN, 1)).toBe('');
  });
});

describe('parseWholeNumber', () => {
  it('accepts digits only', () => {
    expect(parseWholeNumber('14')).toBe(14);
    expect(parseWholeNumber(' 7 ')).toBe(7);
    for (const text of ['', '-1', '1.5', '1,5', 'x', '1e3', '12345678901']) {
      expect(parseWholeNumber(text), text).toBeNull();
    }
  });
});

describe('fractionDigits', () => {
  it('counts the digits after the point without floating point noise', () => {
    expect(fractionDigits(7.9)).toBe(1);
    expect(fractionDigits(1035)).toBe(0);
    expect(fractionDigits(0.1 + 0.2)).toBe(1);
    expect(fractionDigits(1.025)).toBe(3);
  });
});
