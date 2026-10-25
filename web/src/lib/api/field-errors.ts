import { m } from '#lib/paraglide/messages.js';
import { isApiError } from './client.js';

/** The translated text for a field the server rejected, or undefined when the server did not flag that field or the environment locks it (errorText explains that). */
export function fieldText(err: unknown, field: string): string | undefined {
  if (!isApiError(err) || err.code === 'env_locked') return undefined;
  const detail = err.fields[field];
  if (detail === undefined) return undefined;
  switch (detail) {
    case 'required':
      return m.validation_required();
    case 'already taken':
      return m.field_error_taken();
    case 'incorrect password':
      return m.field_error_incorrect_password();
    case 'that code is not valid':
      return m.field_error_invalid_code();
    case 'this account has no password':
      return m.field_error_no_password();
    default:
      return m.field_error_invalid();
  }
}
