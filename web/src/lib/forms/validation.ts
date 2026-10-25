import { normalizeTimeZone } from '#lib/format/timezone.js';
import { parseDecimal } from '#lib/format/decimal.js';
import type { FieldCode, FieldErrors } from './codes.js';

export const passwordLimits = { min: 8, max: 256 } as const;
export const displayNameMax = 100;

const usernamePattern = /^[A-Za-z0-9._@-]{2,64}$/;
const recoveryAlphabet = /^[A-HJ-NP-Z2-9]{10}$/;

const length = (text: string) => [...text].length;

/** The authenticator code without the spaces many apps show in the middle. */
export function normalizeTotpCode(input: string): string {
  return input.replace(/\s+/g, '');
}

export function validateUsername(input: string): FieldCode | null {
  const name = input.trim();
  if (name === '') return 'required';
  return usernamePattern.test(name) ? null : 'invalid';
}

/** A new password is checked by length in characters only, as the server does. */
export function validateNewPassword(input: string): FieldCode | null {
  if (input === '') return 'required';
  const size = length(input);
  if (size < passwordLimits.min) return 'too_short';
  if (size > passwordLimits.max) return 'too_long';
  return null;
}

export function validatePasswordMatch(password: string, repeat: string): FieldCode | null {
  if (repeat === '') return 'required';
  return password === repeat ? null : 'invalid';
}

/** A password that is only being checked against the stored one needs to be present, not long. */
export function validateRequired(input: string): FieldCode | null {
  return input.trim() === '' ? 'required' : null;
}

/** A required text of at most max characters. */
export function validateName(input: string, max: number): FieldCode | null {
  return validateRequired(input) ?? validateMaxLength(input.trim(), max);
}

/** Text of at most max characters, counted as characters rather than UTF-16 units. */
export function validateMaxLength(input: string, max: number): FieldCode | null {
  return length(input) > max ? 'too_long' : null;
}

/** A number that may be left blank; the decimal point or comma are both read. */
export function validateOptionalNumber(input: string): FieldCode | null {
  return input.trim() === '' || parseDecimal(input) !== null ? null : 'invalid';
}

/** Two optional limits typed as text: each must be a number, and the low one must not be above the high one (reported as `range`). */
export function validateLimitPair(
  low: string,
  high: string,
  fields: { low: string; high: string }
): FieldErrors {
  const lowValue = parseDecimal(low);
  const highValue = parseDecimal(high);
  const ordered = lowValue === null || highValue === null || lowValue <= highValue;
  return collectErrors({
    [fields.low]: validateOptionalNumber(low),
    [fields.high]: validateOptionalNumber(high),
    range: ordered ? null : 'out_of_range'
  });
}

export function validateTotpCode(input: string): FieldCode | null {
  const code = normalizeTotpCode(input);
  if (code === '') return 'required';
  return /^\d{6}$/.test(code) ? null : 'invalid';
}

/** Recovery codes look like ABCDE-FGHJK; case, spaces and the dash do not matter. */
export function validateRecoveryCode(input: string): FieldCode | null {
  const code = input.replace(/[\s-]+/g, '').toUpperCase();
  if (code === '') return 'required';
  return recoveryAlphabet.test(code) ? null : 'invalid';
}

/** The email is optional; a non-empty one needs text on both sides of a single at sign. */
export function validateOptionalEmail(input: string): FieldCode | null {
  const email = input.trim();
  if (email === '') return null;
  return /^[^\s@]+@[^\s@]+$/.test(email) ? null : 'invalid';
}

export function validateTimeZone(input: string): FieldCode | null {
  if (input.trim() === '') return 'required';
  return normalizeTimeZone(input) === undefined ? 'invalid' : null;
}

export function validateDisplayName(input: string): FieldCode | null {
  return validateMaxLength(input.trim(), displayNameMax);
}

/** Keeps only the checks that failed, keyed by the server's field names. */
export function collectErrors(checks: Record<string, FieldCode | null>): FieldErrors {
  const errors: FieldErrors = {};
  for (const [field, code] of Object.entries(checks)) {
    if (code !== null) errors[field] = code;
  }
  return errors;
}
