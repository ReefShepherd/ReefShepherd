import { describe, expect, it } from 'vitest';
import {
  collectErrors,
  normalizeTotpCode,
  validateLimitPair,
  validateMaxLength,
  validateName,
  validateOptionalNumber,
  parseEmailList,
  validateDisplayName,
  validateEmailList,
  validateHttpUrl,
  validatePort,
  validateTime,
  validateWholeNumber,
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

describe('validateHttpUrl', () => {
  it('accepts http and https addresses with a host, including local ones', () => {
    expect(validateHttpUrl('https://discord.com/api/webhooks/1/abc')).toBeNull();
    expect(validateHttpUrl(' http://192.168.1.20:8080/hook ')).toBeNull();
    expect(validateHttpUrl('http://localhost/x')).toBeNull();
  });

  it('requires text and refuses other schemes and non-addresses', () => {
    expect(validateHttpUrl('  ')).toBe('required');
    expect(validateHttpUrl('ftp://example.com')).toBe('invalid');
    expect(validateHttpUrl('javascript:alert(1)')).toBe('invalid');
    expect(validateHttpUrl('example.com/hook')).toBe('invalid');
    expect(validateHttpUrl('https://')).toBe('invalid');
  });
});

describe('validateTime', () => {
  it('accepts a 24-hour HH:MM time', () => {
    expect(validateTime('00:00')).toBeNull();
    expect(validateTime('09:05')).toBeNull();
    expect(validateTime(' 23:59 ')).toBeNull();
  });

  it('requires text and refuses everything else', () => {
    expect(validateTime('')).toBe('required');
    expect(validateTime('24:00')).toBe('invalid');
    expect(validateTime('9:00')).toBe('invalid');
    expect(validateTime('09:60')).toBe('invalid');
    expect(validateTime('09:00:30')).toBe('invalid');
    expect(validateTime('noon')).toBe('invalid');
  });
});

describe('validateWholeNumber', () => {
  it('accepts a whole number within the bounds, ends included', () => {
    expect(validateWholeNumber('1', 1, 365)).toBeNull();
    expect(validateWholeNumber(' 365 ', 1, 365)).toBeNull();
  });

  it('separates a missing value, a non-number and a number outside the bounds', () => {
    expect(validateWholeNumber('', 1, 365)).toBe('required');
    expect(validateWholeNumber('7.5', 1, 365)).toBe('invalid');
    expect(validateWholeNumber('1e2', 1, 365)).toBe('invalid');
    expect(validateWholeNumber('abc', 1, 365)).toBe('invalid');
    expect(validateWholeNumber('0', 1, 365)).toBe('out_of_range');
    expect(validateWholeNumber('366', 1, 365)).toBe('out_of_range');
    expect(validateWholeNumber('-3', 1, 365)).toBe('out_of_range');
  });
});

describe('validatePort', () => {
  it('accepts 1 to 65535', () => {
    expect(validatePort('1')).toBeNull();
    expect(validatePort('587')).toBeNull();
    expect(validatePort('65535')).toBeNull();
  });

  it('refuses a missing, malformed or out-of-range port', () => {
    expect(validatePort('')).toBe('required');
    expect(validatePort('smtp')).toBe('invalid');
    expect(validatePort('0')).toBe('out_of_range');
    expect(validatePort('65536')).toBe('out_of_range');
  });
});

describe('validateEmailList', () => {
  it('accepts one or several addresses and ignores spaces and empty entries', () => {
    expect(validateEmailList('a@example.com')).toBeNull();
    expect(validateEmailList(' a@example.com , b@example.org,, ')).toBeNull();
  });

  it('requires an address and refuses any malformed one', () => {
    expect(validateEmailList('')).toBe('required');
    expect(validateEmailList(' , ')).toBe('required');
    expect(validateEmailList('a@example.com, nobody')).toBe('invalid');
    expect(validateEmailList('a@example.com b@example.com')).toBe('invalid');
  });

  it('reads the list into trimmed addresses', () => {
    expect(parseEmailList(' a@example.com , b@example.org,, ')).toEqual([
      'a@example.com',
      'b@example.org'
    ]);
  });
});
