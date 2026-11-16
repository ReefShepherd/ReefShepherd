import { describe, expect, it } from 'vitest';
import { formatBytes } from './bytes.js';
import { emptyValue } from './number.js';

const nb = (text: string) => text.replace(' ', ' ');

describe('formatBytes', () => {
  it('shows whole bytes below 1 KB', () => {
    expect(formatBytes(0)).toBe(nb('0 B'));
    expect(formatBytes(1023)).toBe(nb('1,023 B'));
  });

  it('steps up in powers of 1024 with one decimal', () => {
    expect(formatBytes(1024)).toBe(nb('1.0 KB'));
    expect(formatBytes(1536)).toBe(nb('1.5 KB'));
    expect(formatBytes(5 * 1024 * 1024)).toBe(nb('5.0 MB'));
    expect(formatBytes(3.25 * 1024 ** 3)).toBe(nb('3.3 GB'));
  });

  it('moves to the next unit instead of rounding up to 1,024', () => {
    expect(formatBytes(1024 * 1024 - 1)).toBe(nb('1.0 MB'));
  });

  it('stops at the largest unit', () => {
    expect(formatBytes(2048 * 1024 ** 4)).toBe(nb('2,048.0 TB'));
  });

  it('shows the empty value for a size that is not a usable number', () => {
    expect(formatBytes(-1)).toBe(emptyValue);
    expect(formatBytes(Number.NaN)).toBe(emptyValue);
    expect(formatBytes(Number.POSITIVE_INFINITY)).toBe(emptyValue);
  });

  it('joins the number and unit with a no-break space so they never wrap apart', () => {
    expect(formatBytes(188416)).toBe('184.0 KB');
    expect(formatBytes(188416)).not.toContain(' ');
  });
});
