import { describe, expect, it } from 'vitest';
import { m } from '#lib/paraglide/messages.js';
import { ApiError } from './client.js';
import { errorText } from './error-text.js';

const cases: [string, () => string][] = [
  ['network', m.api_error_network],
  ['http_error', m.api_error_http],
  ['bad_response', m.api_error_bad_response],
  ['validation', m.api_error_validation],
  ['unauthorized', m.api_error_unauthorized],
  ['forbidden', m.api_error_forbidden],
  ['not_found', m.api_error_not_found],
  ['conflict', m.api_error_conflict],
  ['setup_required', m.api_error_setup_required]
];

describe('errorText', () => {
  it.each(cases)('maps the %s code to its message', (code, message) => {
    expect(errorText(new ApiError(400, code, 'server words'))).toBe(message());
  });

  it('gives each known code its own text', () => {
    const texts = cases.map(([, message]) => message());
    expect(new Set(texts).size).toBe(texts.length);
    expect(texts).not.toContain(m.api_error_unknown());
  });

  it('uses the default text for an unknown code', () => {
    expect(errorText(new ApiError(500, 'internal', 'boom'))).toBe(m.api_error_unknown());
  });

  it('uses the default text for values that are not API errors', () => {
    expect(errorText(new Error('boom'))).toBe(m.api_error_unknown());
    expect(errorText(undefined)).toBe(m.api_error_unknown());
  });

  it('never shows the error message or the status', () => {
    const text = errorText(new ApiError(502, 'http_error', 'HTTP 502'));
    expect(text).not.toContain('502');
  });
});
