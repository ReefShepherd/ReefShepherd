import { describe, expect, it } from 'vitest';
import { base32Decode, totpCode } from '../tests/e2e/totp.js';

// RFC 6238 appendix B secret "12345678901234567890"; the 6-digit codes are the last six digits of the published 8-digit ones.
const secret = 'GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ';

describe('base32Decode', () => {
  it('decodes RFC 4648 vectors', () => {
    expect(base32Decode('MZXW6YTBOI======').toString()).toBe('foobar');
    expect(base32Decode('mzxw6 ytb oi').toString()).toBe('foobar');
    expect(base32Decode(secret).toString()).toBe('12345678901234567890');
  });

  it('rejects characters outside the alphabet', () => {
    expect(() => base32Decode('MZXW1')).toThrow();
  });
});

describe('totpCode', () => {
  it.each([
    [59, '287082'],
    [1111111109, '081804'],
    [1111111111, '050471'],
    [1234567890, '005924'],
    [2000000000, '279037']
  ])('matches the RFC 6238 value at %i seconds', (seconds, code) => {
    expect(totpCode(secret, 0, seconds * 1000)).toBe(code);
  });

  it('moves by whole 30-second steps with stepOffset', () => {
    expect(totpCode(secret, 1, 59_000)).toBe(totpCode(secret, 0, 89_000));
    expect(totpCode(secret, -1, 59_000)).toBe(totpCode(secret, 0, 29_000));
  });
});
