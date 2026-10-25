import { describe, expect, it } from 'vitest';
import { m } from '#lib/paraglide/messages.js';
import {
  collectErrors,
  normalizeTotpCode,
  validateDisplayName,
  validateNewPassword,
  validateOptionalEmail,
  validatePasswordMatch,
  validateRecoveryCode,
  validateRequired,
  validateTimeZone,
  validateTotpCode,
  validateUsername,
  validationText
} from './validation.js';

describe('validateUsername', () => {
  it('accepts the characters the server accepts, trimmed', () => {
    expect(validateUsername(' reef.keeper-1@home_lab ')).toBeNull();
    expect(validateUsername('ab')).toBeNull();
    expect(validateUsername('a'.repeat(64))).toBeNull();
  });

  it('refuses blank, too short, too long and other characters', () => {
    expect(validateUsername('  ')).toBe('required');
    expect(validateUsername('a')).toBe('username');
    expect(validateUsername('a'.repeat(65))).toBe('username');
    expect(validateUsername('has space')).toBe('username');
    expect(validateUsername('ünï')).toBe('username');
  });
});

describe('validateNewPassword', () => {
  it('requires 8 to 256 characters', () => {
    expect(validateNewPassword('')).toBe('required');
    expect(validateNewPassword('a'.repeat(7))).toBe('password_short');
    expect(validateNewPassword('a'.repeat(8))).toBeNull();
    expect(validateNewPassword('a'.repeat(256))).toBeNull();
    expect(validateNewPassword('a'.repeat(257))).toBe('password_long');
  });

  it('counts characters, not UTF-16 units', () => {
    expect(validateNewPassword('🐠'.repeat(8))).toBeNull();
    expect(validateNewPassword('🐠'.repeat(7))).toBe('password_short');
  });

  it('keeps spaces, since passphrases have them', () => {
    expect(validateNewPassword('correct horse battery')).toBeNull();
  });
});

describe('validatePasswordMatch', () => {
  it('flags an empty or different repeat', () => {
    expect(validatePasswordMatch('abc', '')).toBe('required');
    expect(validatePasswordMatch('abc', 'abd')).toBe('password_mismatch');
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
    expect(validateTotpCode('12345')).toBe('code');
    expect(validateTotpCode('1234567')).toBe('code');
    expect(validateTotpCode('12345a')).toBe('code');
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
    expect(validateRecoveryCode('ABCDE-FGHJI')).toBe('recovery_code');
    expect(validateRecoveryCode('ABCDE-FGHJ0')).toBe('recovery_code');
    expect(validateRecoveryCode('ABCDE-FGHJ')).toBe('recovery_code');
    expect(validateRecoveryCode('ABCDE-FGHJKL')).toBe('recovery_code');
  });
});

describe('validateOptionalEmail and validateDisplayName', () => {
  it('lets the email be blank but not malformed', () => {
    expect(validateOptionalEmail('')).toBeNull();
    expect(validateOptionalEmail('a@b.c')).toBeNull();
    expect(validateOptionalEmail('no-at-sign')).toBe('email');
    expect(validateOptionalEmail('a@b@c')).toBe('email');
  });

  it('limits the display name to 100 characters', () => {
    expect(validateDisplayName('x'.repeat(100))).toBeNull();
    expect(validateDisplayName('x'.repeat(101))).toBe('name_long');
  });
});

describe('validateTimeZone', () => {
  it('accepts known zones and refuses blanks and made-up names', () => {
    expect(validateTimeZone('Europe/Berlin')).toBeNull();
    expect(validateTimeZone('')).toBe('required');
    expect(validateTimeZone('Nowhere/Land')).toBe('timezone');
  });
});

describe('collectErrors', () => {
  it('keeps only failed fields, as text', () => {
    expect(collectErrors({ a: null, b: 'required', c: 'username' })).toEqual({
      b: m.validation_required(),
      c: m.validation_username()
    });
  });

  it('writes the limits into the password messages', () => {
    expect(validationText('password_short')).toContain('8');
    expect(validationText('password_long')).toContain('256');
  });
});
