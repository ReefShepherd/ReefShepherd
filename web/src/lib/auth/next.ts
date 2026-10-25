import { pathMatches } from '#lib/path.js';

const maxLength = 2048;
const probe = 'http://same-origin.invalid';

/** The page to open after signing in: the given path when it is a same-origin relative path beginning with a single slash, otherwise the home page. */
export function safeNext(raw: string | null | undefined): string {
  if (typeof raw !== 'string' || raw.length > maxLength) return '/';
  // The second character rules out //host and /\host, and control characters are stripped by URL parsers.
  // eslint-disable-next-line no-control-regex
  if (!/^\/(?![/\\])[^\\\u0000-\u001f\u007f]*$/.test(raw)) return '/';
  const url = new URL(raw, probe);
  if (pathMatches(url.pathname, '/login') || pathMatches(url.pathname, '/setup')) return '/';
  return raw;
}

/** The sign-in page address that returns to location afterwards; the home page needs no return address. */
export function loginUrl(location: string): string {
  const next = safeNext(location);
  return next === '/' ? '/login' : `/login?next=${encodeURIComponent(next)}`;
}
