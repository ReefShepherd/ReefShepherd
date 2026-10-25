import { normalizeTimeZone } from '#lib/format/timezone.js';
import { m } from '#lib/paraglide/messages.js';

/** Why a value was refused; forms turn a code into text with validationText. */
export type ValidationCode =
  | 'required'
  | 'username'
  | 'password_short'
  | 'password_long'
  | 'password_mismatch'
  | 'code'
  | 'recovery_code'
  | 'email'
  | 'timezone'
  | 'name_long';

export const passwordLimits = { min: 12, max: 256 } as const;
export const displayNameMax = 100;

const usernamePattern = /^[A-Za-z0-9._@-]{2,64}$/;
const recoveryAlphabet = /^[A-HJ-NP-Z2-9]{10}$/;

/** The authenticator code without the spaces many apps show in the middle. */
export function normalizeTotpCode(input: string): string {
  return input.replace(/\s+/g, '');
}

export function validateUsername(input: string): ValidationCode | null {
  const name = input.trim();
  if (name === '') return 'required';
  return usernamePattern.test(name) ? null : 'username';
}

/** A new password is checked by length in characters only, as the server does. */
export function validateNewPassword(input: string): ValidationCode | null {
  if (input === '') return 'required';
  const length = [...input].length;
  if (length < passwordLimits.min) return 'password_short';
  if (length > passwordLimits.max) return 'password_long';
  return null;
}

export function validatePasswordMatch(password: string, repeat: string): ValidationCode | null {
  if (repeat === '') return 'required';
  return password === repeat ? null : 'password_mismatch';
}

/** A password that is only being checked against the stored one needs to be present, not long. */
export function validateRequired(input: string): ValidationCode | null {
  return input.trim() === '' ? 'required' : null;
}

export function validateTotpCode(input: string): ValidationCode | null {
  const code = normalizeTotpCode(input);
  if (code === '') return 'required';
  return /^\d{6}$/.test(code) ? null : 'code';
}

/** Recovery codes look like ABCDE-FGHJK; case, spaces and the dash do not matter. */
export function validateRecoveryCode(input: string): ValidationCode | null {
  const code = input.replace(/[\s-]+/g, '').toUpperCase();
  if (code === '') return 'required';
  return recoveryAlphabet.test(code) ? null : 'recovery_code';
}

/** The email is optional; a non-empty one needs text on both sides of a single at sign. */
export function validateOptionalEmail(input: string): ValidationCode | null {
  const email = input.trim();
  if (email === '') return null;
  return /^[^\s@]+@[^\s@]+$/.test(email) ? null : 'email';
}

export function validateTimeZone(input: string): ValidationCode | null {
  if (input.trim() === '') return 'required';
  return normalizeTimeZone(input) === undefined ? 'timezone' : null;
}

export function validateDisplayName(input: string): ValidationCode | null {
  return [...input.trim()].length > displayNameMax ? 'name_long' : null;
}

export function validationText(code: ValidationCode): string {
  switch (code) {
    case 'required':
      return m.validation_required();
    case 'username':
      return m.validation_username();
    case 'password_short':
      return m.validation_password_short({ min: passwordLimits.min });
    case 'password_long':
      return m.validation_password_long({ max: passwordLimits.max });
    case 'password_mismatch':
      return m.validation_password_mismatch();
    case 'code':
      return m.validation_code();
    case 'recovery_code':
      return m.validation_recovery_code();
    case 'email':
      return m.validation_email();
    case 'timezone':
      return m.validation_timezone();
    case 'name_long':
      return m.validation_name_long({ max: displayNameMax });
  }
}

/** Keeps only the fields that failed, as translated text keyed by field name. */
export function collectErrors<K extends string>(
  checks: Record<K, ValidationCode | null>
): Partial<Record<K, string>> {
  const errors: Partial<Record<K, string>> = {};
  for (const field of Object.keys(checks) as K[]) {
    const code = checks[field];
    if (code !== null) errors[field] = validationText(code);
  }
  return errors;
}
