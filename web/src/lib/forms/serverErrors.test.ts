import { describe, expect, it } from 'vitest';
import { ApiError } from '#lib/api/client.js';
import { errorText } from '#lib/api/error-text.js';
import { m } from '#lib/paraglide/messages.js';
import { fieldCodes, type FieldCode } from './codes.js';
import { fieldMessage, fieldTexts, moveField, serverErrors } from './serverErrors.js';

const rejected = (fields: Record<string, string>) =>
  new ApiError(422, 'validation', 'invalid', fields);

describe('fieldMessage', () => {
  it('has a message for every code on a field with no override', () => {
    const texts = fieldCodes.map((code) => fieldMessage('anything', code));
    expect(new Set(texts).size).toBe(fieldCodes.length);
    for (const text of texts) expect(text).not.toBe('');
  });

  it('keeps the specific message of each phrase the browser used to recognise', () => {
    expect(fieldMessage('anything', 'required')).toBe(m.form_err_required());
    expect(fieldMessage('username', 'taken')).toBe(m.form_err_username_taken());
    expect(fieldMessage('current_password', 'incorrect')).toBe(m.form_err_password_incorrect());
    expect(fieldMessage('password', 'incorrect')).toBe(m.form_err_password_incorrect());
    expect(fieldMessage('code', 'incorrect')).toBe(m.form_err_code_incorrect());
    expect(fieldMessage('recovery_code', 'incorrect')).toBe(m.form_err_code_incorrect());
    expect(fieldMessage('current_password', 'invalid')).toBe(m.form_err_no_password());
  });

  it('words the delete-tank confirmation and the password minimum', () => {
    expect(fieldMessage('confirm', 'incorrect')).toBe(m.form_err_confirm_name());
    expect(fieldMessage('password', 'too_short')).toBe(m.form_err_password_short({ min: 8 }));
    expect(fieldMessage('new_password', 'too_short')).toContain('8');
    expect(fieldMessage('new_password', 'too_long')).toContain('256');
  });

  it('words the format checks of the single-purpose fields', () => {
    expect(fieldMessage('username', 'invalid')).toBe(m.form_err_username());
    expect(fieldMessage('username', 'too_long')).toBe(m.form_err_username());
    expect(fieldMessage('code', 'invalid')).toBe(m.form_err_totp_code());
    expect(fieldMessage('recovery_code', 'invalid')).toBe(m.form_err_recovery_code());
    expect(fieldMessage('email', 'invalid')).toBe(m.form_err_email());
    expect(fieldMessage('timezone', 'invalid')).toBe(m.form_err_timezone());
    expect(fieldMessage('password_repeat', 'invalid')).toBe(m.form_err_password_mismatch());
    expect(fieldMessage('ideal_low', 'invalid')).toBe(m.value_invalid_number());
    expect(fieldMessage('volume', 'out_of_range')).toBe(m.form_err_volume());
    expect(fieldMessage('range', 'out_of_range')).toBe(m.params_range_order());
    expect(fieldMessage('measured_at', 'invalid')).toBe(m.tests_time_invalid());
    expect(fieldMessage('value_missing', 'required')).toBe(m.tests_value_needed());
    expect(fieldMessage('value', 'required')).toBe(m.form_err_required());
    expect(fieldMessage('parameter_id', 'invalid')).toBe(m.form_err_parameter());
    expect(fieldMessage('password', 'invalid')).toBe(m.form_err_invalid());
  });

  it('writes the limit into a too-long message and into the test interval message', () => {
    expect(fieldMessage('note', 'too_long', 100)).toBe(m.form_err_chars_max({ max: 100 }));
    expect(fieldMessage('note', 'too_long')).toBe(m.form_err_too_long());
    expect(fieldMessage('default_test_interval_days', 'out_of_range', 3650)).toBe(
      m.params_def_interval_invalid({ max: 3650 })
    );
    expect(fieldMessage('default_test_interval_days', 'out_of_range')).toBe(
      m.form_err_out_of_range()
    );
  });
});

describe('moveField', () => {
  it('renames one flagged field and leaves the others', () => {
    expect(
      moveField({ password: 'incorrect', timezone: 'invalid' }, 'password', 'current_password')
    ).toEqual({
      current_password: 'incorrect',
      timezone: 'invalid'
    });
  });

  it('changes nothing when the field is not flagged', () => {
    expect(moveField({ timezone: 'invalid' }, 'password', 'current_password')).toEqual({
      timezone: 'invalid'
    });
  });
});

describe('fieldTexts', () => {
  it('turns every refused field into text, using the limits it is given', () => {
    expect(
      fieldTexts({ name: 'too_long', display_name: 'too_long', note: 'required' }, { name: 60 })
    ).toEqual({
      name: m.form_err_chars_max({ max: 60 }),
      display_name: m.form_err_chars_max({ max: 100 }),
      note: m.form_err_required()
    });
  });

  it('gives nothing for no errors', () => {
    expect(fieldTexts({})).toEqual({});
  });
});

describe('serverErrors', () => {
  it('reads a real error body: codes by field, and no general message', () => {
    const err = rejected({
      username: 'already taken',
      password: 'must be at least 8 characters',
      'results.1.value': 'is outside the possible range',
      name: 'is required',
      timezone: 'unknown timezone "x"',
      role: 'a sentence nobody has seen'
    });
    expect(serverErrors(err)).toEqual({
      fields: {
        username: 'taken',
        password: 'too_short',
        'results.1.value': 'out_of_range',
        name: 'required',
        timezone: 'invalid',
        role: 'invalid'
      },
      message: null
    });
  });

  it('accepts the codes themselves, so the server can send them', () => {
    const fields: Record<string, FieldCode> = { name: 'too_long', email: 'taken' };
    expect(serverErrors(rejected(fields)).fields).toEqual(fields);
  });

  it('never shows a raw server string', () => {
    const { fields } = serverErrors(rejected({ name: 'must be a pretty name' }));
    expect(fieldMessage('name', fields.name ?? 'invalid')).toBe(m.form_err_invalid());
  });

  it('keeps only the fields the form shows, by name or by pattern', () => {
    const err = rejected({ name: 'required', cards: 'required', 'results.0.value': 'required' });
    expect(serverErrors(err, ['name']).fields).toEqual({ name: 'required' });
    expect(serverErrors(err, /^results\./).fields).toEqual({ 'results.0.value': 'required' });
  });

  it('explains a failure that flags no shown field with the general text', () => {
    const err = rejected({ cards: 'required' });
    expect(serverErrors(err, ['name'])).toEqual({ fields: {}, message: errorText(err) });
    expect(serverErrors(err, [])).toEqual({ fields: {}, message: errorText(err) });
  });

  it('leaves a locked field to the general text', () => {
    const locked = new ApiError(409, 'env_locked', 'set by REEFSHEPHERD_AUTH_MODE', {
      auth_mode: 'set by REEFSHEPHERD_AUTH_MODE on the server'
    });
    expect(serverErrors(locked)).toEqual({ fields: {}, message: m.api_error_env_locked() });
  });

  it('gives the general text for errors that are not API errors or flag no field', () => {
    expect(serverErrors(new Error('x'))).toEqual({ fields: {}, message: m.api_error_unknown() });
    expect(serverErrors(new ApiError(403, 'forbidden', 'no'))).toEqual({
      fields: {},
      message: m.api_error_forbidden()
    });
  });
});
