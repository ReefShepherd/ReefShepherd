import { loginUrl } from '#lib/auth/next.js';
import { pathMatches } from '#lib/path.js';

const base = '/api/v1';

/** Per-field validation messages keyed by field name, as sent by the server. */
export type FieldErrors = Record<string, string>;

/** Any failed API call: an error body from the server, a non-JSON reply, or a network failure (status 0); message is for logs, so show errorText(err) instead. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly fields: FieldErrors;
  /** Seconds the server asked the client to wait, from the Retry-After header; undefined when absent. */
  readonly retryAfter: number | undefined;

  constructor(
    status: number,
    code: string,
    message: string,
    fields: FieldErrors = {},
    retryAfter?: number
  ) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.fields = fields;
    this.retryAfter = retryAfter;
  }
}

export function isApiError(value: unknown): value is ApiError {
  return value instanceof ApiError;
}

type QueryValue = string | number | boolean | null | undefined;

export interface RequestOptions {
  signal?: AbortSignal;
  /** Appended as a query string; null and undefined entries are skipped. */
  query?: Record<string, QueryValue>;
}

export interface Api {
  get<T>(path: string, options?: RequestOptions): Promise<T>;
  post<T>(path: string, body?: unknown, options?: RequestOptions): Promise<T>;
  patch<T>(path: string, body?: unknown, options?: RequestOptions): Promise<T>;
  put<T>(path: string, body?: unknown, options?: RequestOptions): Promise<T>;
  del<T = void>(path: string, body?: unknown, options?: RequestOptions): Promise<T>;
}

export interface ApiDeps {
  fetch?: typeof fetch;
  /** Called with the page to show when the session or the instance needs attention. */
  redirect?: (to: string) => void;
  currentPath?: () => string;
  /** The query string of the current page, with its leading question mark or empty. */
  currentSearch?: () => string;
}

function buildUrl(path: string, query?: RequestOptions['query']): string {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query ?? {})) {
    if (value !== null && value !== undefined) params.set(key, String(value));
  }
  const qs = params.toString();
  if (!qs) return base + path;
  return `${base}${path}${path.includes('?') ? '&' : '?'}${qs}`;
}

// Retry-After may also be an HTTP date; only the delay-seconds form is used.
function readRetryAfter(res: Response): number | undefined {
  const raw = res.headers.get('Retry-After');
  if (raw === null || !/^\d+$/.test(raw.trim())) return undefined;
  return Number(raw.trim());
}

async function readError(res: Response): Promise<ApiError> {
  const retryAfter = readRetryAfter(res);
  let parsed: unknown;
  try {
    parsed = await res.json();
  } catch {
    return new ApiError(res.status, 'http_error', `HTTP ${res.status}`, {}, retryAfter);
  }
  const err = (parsed as { error?: { code?: unknown; message?: unknown; fields?: unknown } } | null)
    ?.error;
  if (!err || typeof err.code !== 'string') {
    return new ApiError(res.status, 'http_error', `HTTP ${res.status}`, {}, retryAfter);
  }
  const fields = err.fields && typeof err.fields === 'object' ? (err.fields as FieldErrors) : {};
  return new ApiError(
    res.status,
    err.code,
    typeof err.message === 'string' ? err.message : '',
    fields,
    retryAfter
  );
}

export function createApi(deps: ApiDeps = {}): Api {
  const doFetch = deps.fetch ?? ((input, init) => fetch(input, init));
  const redirect = deps.redirect ?? ((to) => location.assign(to));
  const currentPath = deps.currentPath ?? (() => location.pathname);
  const currentSearch = deps.currentSearch ?? (() => location.search);

  // The error is still thrown after redirecting so callers stop their own work.
  function redirectFor(error: ApiError): void {
    const target =
      error.code === 'setup_required' ? '/setup' : error.status === 401 ? '/login' : undefined;
    if (!target || pathMatches(currentPath(), target)) return;
    redirect(target === '/login' ? loginUrl(currentPath() + currentSearch()) : target);
  }

  async function request<T>(
    method: string,
    path: string,
    body: unknown,
    options: RequestOptions = {}
  ): Promise<T> {
    const headers: Record<string, string> = { Accept: 'application/json' };
    const init: RequestInit = { method, headers, signal: options.signal };
    if (body !== undefined) {
      headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(body);
    }

    let res: Response;
    try {
      res = await doFetch(buildUrl(path, options.query), init);
    } catch (e) {
      if (e instanceof DOMException && e.name === 'AbortError') throw e;
      throw new ApiError(0, 'network', 'Network error');
    }

    if (!res.ok) {
      const error = await readError(res);
      redirectFor(error);
      throw error;
    }
    if (res.status === 204) return undefined as T;
    try {
      return (await res.json()) as T;
    } catch {
      throw new ApiError(res.status, 'bad_response', 'The server sent an unreadable reply');
    }
  }

  return {
    get: (path, options) => request('GET', path, undefined, options),
    post: (path, body, options) => request('POST', path, body, options),
    patch: (path, body, options) => request('PATCH', path, body, options),
    put: (path, body, options) => request('PUT', path, body, options),
    del: (path, body, options) => request('DELETE', path, body, options)
  };
}

export const api = createApi();
