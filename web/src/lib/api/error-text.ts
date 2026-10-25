import { m } from '#lib/paraglide/messages.js';
import { formatWait } from '#lib/format/duration.js';
import { isApiError } from './client.js';

/** The translated text to show for a failed API call; the error's own message is for logs only. */
export function errorText(err: unknown): string {
  if (!isApiError(err)) return m.api_error_unknown();
  switch (err.code) {
    case 'network':
      return m.api_error_network();
    case 'http_error':
      return m.api_error_http();
    case 'bad_response':
      return m.api_error_bad_response();
    case 'validation':
      return m.api_error_validation();
    case 'unauthorized':
      return m.api_error_unauthorized();
    case 'forbidden':
      return m.api_error_forbidden();
    case 'not_found':
      return m.api_error_not_found();
    case 'conflict':
      return m.api_error_conflict();
    case 'setup_required':
      return m.api_error_setup_required();
    case 'invalid_credentials':
      return m.api_error_invalid_credentials();
    case 'invalid_code':
      return m.api_error_invalid_code();
    case 'env_locked':
      return m.api_error_env_locked();
    case 'too_many_requests':
      return err.retryAfter === undefined
        ? m.api_error_rate_limited_soon()
        : m.api_error_rate_limited({ wait: formatWait(err.retryAfter) });
    default:
      return m.api_error_unknown();
  }
}
