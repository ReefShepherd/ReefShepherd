import { describe, expect, it } from 'vitest';
import {
  collectErrors,
  normalizeTotpCode,
  validateLimitPair,
  validateMaxLength,
  validateName,
  validateOptionalNumber,
  validateDisplayName,
  validateNewPassword,
  validateOptionalEmail,
  validatePasswordMatch,
  validateRecoveryCode,
  validateRequired,
  validateTimeZone,
  validateTotpCode,
  validateUsername
} from './validation.js';

describe('validateUsername', () => {
  it('accepts the characters the server accepts, trimmed', () => {
    expect(validateUsername(' reef.keeper-1@home_lab ')).toBeNull();
    expect(validateUsername('ab')).toBeNull();
    expect(validateUsername('a'.repeat(64))).toBeNull();
  });

  it('refuses blank, too short, too long and other characters', () => {
    expect(validateUsername('  ')).toBe('required');
    expect(validateUsername('a')).toBe('invalid');
    expect(validateUsername('a'.repeat(65))).toBe('invalid');
    expect(validateUsername('has space')).toBe('invalid');
    expect(validateUsername('ünï')).toBe('invalid');
  });
});

describe('validateNewPassword', () => {
  it('requires 8 to 256 characters', () => {
    expect(validateNewPassword('')).toBe('required');
    expect(validateNewPassword('a'.repeat(7))).toBe('too_short');
    expect(validateNewPassword('a'.repeat(8))).toBeNull();
    expect(validateNewPassword('a'.repeat(256))).toBeNull();
    expect(validateNewPassword('a'.repeat(257))).toBe('too_long');
  });

  it('counts characters, not UTF-16 units', () => {
    expect(validateNewPassword('🐠'.repeat(8))).toBeNull();
    expect(validateNewPassword('🐠'.repeat(7))).toBe('too_short');
  });

  it('keeps spaces, since passphrases have them', () => {
    expect(validateNewPassword('correct horse battery')).toBeNull();
  });
});

describe('validatePasswordMatch', () => {
  it('flags an empty or different repeat', () => {
    expect(validatePasswordMatch('abc', '')).toBe('required');
    expect(validatePasswordMatch('abc', 'abd')).toBe('invalid');
    expect(validatePasswordMatch('abc', 'abc')).toBeNull();
  });
});

describe('validateRequired', () => {
  it('only checks that something was typed', () => {
    expect(validateRequired('')).toBe('required');
    expect(validateRequired('  ')).toBe('required');
    expect(validateRequired('x')).toBeNull();
  });
});

describe('TOTP codes', () => {
  it('strips spaces before checking six digits', () => {
    expect(normalizeTotpCode('123 456')).toBe('123456');
    expect(validateTotpCode('123 456')).toBeNull();
    expect(validateTotpCode('123456')).toBeNull();
  });

  it('refuses blank, short, long and non-digit codes', () => {
    expect(validateTotpCode('')).toBe('required');
    expect(validateTotpCode('12345')).toBe('invalid');
    expect(validateTotpCode('1234567')).toBe('invalid');
    expect(validateTotpCode('12345a')).toBe('invalid');
  });
});

describe('validateRecoveryCode', () => {
  it('accepts the server alphabet in any case, with or without the dash', () => {
    expect(validateRecoveryCode('ABCDE-FGHJK')).toBeNull();
    expect(validateRecoveryCode('abcde-fghjk')).toBeNull();
    expect(validateRecoveryCode('abcde fghjk')).toBeNull();
    expect(validateRecoveryCode('ABCDEFGHJK')).toBeNull();
  });

  it('refuses letters the server never issues, wrong lengths and blanks', () => {
    expect(validateRecoveryCode('')).toBe('required');
    expect(validateRecoveryCode('ABCDE-FGHJI')).toBe('invalid');
    expect(validateRecoveryCode('ABCDE-FGHJ0')).toBe('invalid');
    expect(validateRecoveryCode('ABCDE-FGHJ')).toBe('invalid');
    expect(validateRecoveryCode('ABCDE-FGHJKL')).toBe('invalid');
  });
});

describe('validateOptionalEmail and validateDisplayName', () => {
  it('lets the email be blank but not malformed', () => {
    expect(validateOptionalEmail('')).toBeNull();
    expect(validateOptionalEmail('a@b.c')).toBeNull();
    expect(validateOptionalEmail('no-at-sign')).toBe('invalid');
    expect(validateOptionalEmail('a@b@c')).toBe('invalid');
  });

  it('limits the display name to 100 characters', () => {
    expect(validateDisplayName('x'.repeat(100))).toBeNull();
    expect(validateDisplayName('x'.repeat(101))).toBe('too_long');
  });
});

describe('validateTimeZone', () => {
  it('accepts known zones and refuses blanks and made-up names', () => {
    expect(validateTimeZone('Europe/Berlin')).toBeNull();
    expect(validateTimeZone('')).toBe('required');
    expect(validateTimeZone('Nowhere/Land')).toBe('invalid');
  });
});

describe('validateName and validateMaxLength', () => {
  it('requires a name of at most max characters, trimmed', () => {
    expect(validateName('  ', 5)).toBe('required');
    expect(validateName(' abcde ', 5)).toBeNull();
    expect(validateName('abcdef', 5)).toBe('too_long');
  });

  it('counts characters, not UTF-16 units', () => {
    expect(validateMaxLength('🐠'.repeat(3), 3)).toBeNull();
    expect(validateMaxLength('🐠'.repeat(4), 3)).toBe('too_long');
  });
});

describe('validateOptionalNumber', () => {
  it('lets the text be blank, a decimal point or a decimal comma, but nothing else', () => {
    expect(validateOptionalNumber('')).toBeNull();
    expect(validateOptionalNumber(' 7,9 ')).toBeNull();
    expect(validateOptionalNumber('7.9')).toBeNull();
    expect(validateOptionalNumber('high')).toBe('invalid');
  });
});

describe('validateLimitPair', () => {
  const keys = { low: 'ideal_low', high: 'ideal_high' };

  it('accepts blank limits and a low limit at or below the high one', () => {
    expect(validateLimitPair('', '', keys)).toEqual({});
    expect(validateLimitPair('7,9', '', keys)).toEqual({});
    expect(validateLimitPair('8', '8', keys)).toEqual({});
  });

  it('flags a limit that is not a number under its own field', () => {
    expect(validateLimitPair('x', '8', keys)).toEqual({ ideal_low: 'invalid' });
    expect(validateLimitPair('7', '1,x', keys)).toEqual({ ideal_high: 'invalid' });
  });

  it('flags a low limit above the high one as range', () => {
    expect(validateLimitPair('9', '8', keys)).toEqual({ range: 'out_of_range' });
  });
});

describe('collectErrors', () => {
  it('keeps only failed fields, as codes', () => {
    expect(collectErrors({ a: null, b: 'required', c: 'invalid' })).toEqual({
      b: 'required',
      c: 'invalid'
    });
  });
});
