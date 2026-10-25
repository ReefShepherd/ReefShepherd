import { describe, expect, it } from 'vitest';
import { m } from '#lib/paraglide/messages.js';
import { ApiError } from './client.js';
import { fieldText } from './field-errors.js';

const rejected = (fields: Record<string, string>) =>
  new ApiError(422, 'validation', 'invalid', fields);

describe('fieldText', () => {
  it('translates the server messages the forms can run into', () => {
    expect(fieldText(rejected({ username: 'already taken' }), 'username')).toBe(
      m.field_error_taken()
    );
    expect(
      fieldText(rejected({ current_password: 'incorrect password' }), 'current_password')
    ).toBe(m.field_error_incorrect_password());
    expect(fieldText(rejected({ code: 'that code is not valid' }), 'code')).toBe(
      m.field_error_invalid_code()
    );
    expect(fieldText(rejected({ password: 'required' }), 'password')).toBe(m.validation_required());
  });

  it('falls back to a generic text for any other server message', () => {
    expect(fieldText(rejected({ timezone: 'unknown timezone "x"' }), 'timezone')).toBe(
      m.field_error_invalid()
    );
  });

  it('returns undefined for a field the server did not flag or an error that is not an API error', () => {
    expect(fieldText(rejected({ username: 'already taken' }), 'email')).toBeUndefined();
    expect(fieldText(new Error('x'), 'username')).toBeUndefined();
  });

  it('leaves a field the environment locks to errorText', () => {
    const locked = new ApiError(409, 'env_locked', 'set by REEFSHEPHERD_AUTH_MODE', {
      auth_mode: 'set by REEFSHEPHERD_AUTH_MODE on the server'
    });
    expect(fieldText(locked, 'auth_mode')).toBeUndefined();
  });
});
