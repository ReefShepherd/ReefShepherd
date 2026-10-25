import { isApiError } from '#lib/api/client.js';
import { errorText } from '#lib/api/error-text.js';
import { m } from '#lib/paraglide/messages.js';
import { fieldCodeOf, hasErrors, type FieldCode, type FieldErrors } from './codes.js';
import { displayNameMax, passwordLimits } from './validation.js';

/** Which of the fields an error flagged a form has an input for: their names, or a pattern that matches them. */
export type ShownFields = readonly string[] | RegExp;

const isShown = (field: string, shown: ShownFields | undefined) =>
  shown === undefined || (shown instanceof RegExp ? shown.test(field) : shown.includes(field));

/** The fields a failed API call flagged, as codes, and the general message to show when none of them explains the failure. */
export function serverErrors(
  err: unknown,
  shown?: ShownFields
): { fields: FieldErrors; message: string | null } {
  const fields: FieldErrors = {};
  // A field the environment locks is explained by errorText, so it is not reported as a field.
  if (isApiError(err) && err.code !== 'env_locked') {
    for (const [field, detail] of Object.entries(err.fields)) {
      if (isShown(field, shown)) fields[field] = fieldCodeOf(detail);
    }
  }
  return { fields, message: hasErrors(fields) ? null : errorText(err) };
}

/** Moves a flagged field to another name, for a form whose input is called something else than the server's field. */
export function moveField(errors: FieldErrors, from: string, to: string): FieldErrors {
  const { [from]: code, ...rest } = errors;
  return code === undefined ? rest : { ...rest, [to]: code };
}

const generic: Record<FieldCode, () => string> = {
  required: () => m.form_err_required(),
  invalid: () => m.form_err_invalid(),
  too_short: () => m.form_err_too_short(),
  too_long: () => m.form_err_too_long(),
  out_of_range: () => m.form_err_out_of_range(),
  taken: () => m.form_err_taken(),
  incorrect: () => m.form_err_incorrect(),
  locked: () => m.api_error_env_locked()
};

// A field-specific text is told the field's length limit when the form knows one, and may decline by returning undefined.
type Text = (limit?: number) => string | undefined;

const specific = new Map<string, Text>();

function define(fields: readonly string[], codes: readonly FieldCode[], text: Text) {
  for (const field of fields) for (const code of codes) specific.set(`${field}:${code}`, text);
}

const numberFields = ['value', 'volume', 'default_low', 'default_high', 'ideal_low', 'ideal_high'];

define(['username'], ['invalid', 'too_short', 'too_long'], () => m.form_err_username());
define(['username'], ['taken'], () => m.form_err_username_taken());
define(['password', 'new_password'], ['too_short'], () =>
  m.form_err_password_short({ min: passwordLimits.min })
);
define(['password', 'new_password'], ['too_long'], () =>
  m.form_err_password_long({ max: passwordLimits.max })
);
define(['password', 'current_password'], ['incorrect'], () => m.form_err_password_incorrect());
define(['current_password'], ['invalid'], () => m.form_err_no_password());
define(['password_repeat'], ['invalid'], () => m.form_err_password_mismatch());
define(['code'], ['invalid'], () => m.form_err_totp_code());
define(['recovery_code'], ['invalid'], () => m.form_err_recovery_code());
define(['code', 'recovery_code'], ['incorrect'], () => m.form_err_code_incorrect());
define(['email'], ['invalid'], () => m.form_err_email());
define(['timezone'], ['invalid'], () => m.form_err_timezone());
define(['confirm'], ['incorrect'], () => m.form_err_confirm_name());
define(numberFields, ['invalid'], () => m.value_invalid_number());
define(['volume'], ['out_of_range'], () => m.form_err_volume());
define(['range'], ['out_of_range'], () => m.params_range_order());
define(['default_test_interval_days'], ['invalid', 'out_of_range'], (max) =>
  max === undefined ? undefined : m.params_def_interval_invalid({ max })
);
define(['value_missing'], ['required'], () => m.tests_value_needed());
define(['parameter_id'], ['invalid', 'required'], () => m.form_err_parameter());
define(['measured_at'], ['invalid'], () => m.tests_time_invalid());

/** The text for a field the server or the browser refused; limit is the field's maximum length (or other bound) when the form knows it. */
export function fieldMessage(field: string, code: FieldCode, limit?: number): string {
  const text = specific.get(`${field}:${code}`)?.(limit);
  if (text !== undefined) return text;
  if (code === 'too_long' && limit !== undefined) return m.form_err_chars_max({ max: limit });
  return generic[code]();
}

const defaultLimits: Readonly<Record<string, number>> = { display_name: displayNameMax };

/** The text for every refused field, keyed by field name; limits gives the maximum lengths this form's fields use. */
export function fieldTexts(
  errors: FieldErrors,
  limits: Readonly<Record<string, number>> = {}
): Partial<Record<string, string>> {
  const all = { ...defaultLimits, ...limits };
  const texts: Partial<Record<string, string>> = {};
  for (const [field, code] of Object.entries(errors)) {
    if (code !== undefined) texts[field] = fieldMessage(field, code, all[field]);
  }
  return texts;
}
