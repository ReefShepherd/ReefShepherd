import { describe, expect, it } from 'vitest';
import { formatWait } from './duration.js';

describe('formatWait', () => {
  it('shows short waits in seconds', () => {
    expect(formatWait(30)).toBe('30 seconds');
    expect(formatWait(1)).toBe('1 second');
  });

  it('rounds fractions of a second up', () => {
    expect(formatWait(30.4)).toBe('31 seconds');
  });

  it('rounds up and never shows zero', () => {
    expect(formatWait(0.2)).toBe('1 second');
    expect(formatWait(0)).toBe('1 second');
  });

  it('switches to whole minutes, rounded up, from two minutes', () => {
    expect(formatWait(119)).toBe('119 seconds');
    expect(formatWait(120)).toBe('2 minutes');
    expect(formatWait(121)).toBe('3 minutes');
    expect(formatWait(900)).toBe('15 minutes');
  });
});
