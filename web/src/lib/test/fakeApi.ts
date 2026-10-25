import type { Api, RequestOptions } from '#lib/api/client.js';

/** One request a fake API received; path has no query string, which is in query. */
export interface FakeCall {
  method: string;
  path: string;
  body: unknown;
  query: RequestOptions['query'];
  signal: AbortSignal | undefined;
}

/** A reply, or a function that makes one from the request body. */
export type FakeReply = unknown | ((body: unknown) => unknown);

/** An Api whose replies come from routes keyed "METHOD /path", such as "GET /tanks/3"; a request with no route fails the test. */
export function fakeApi(routes: Record<string, FakeReply> = {}): { api: Api; calls: FakeCall[] } {
  const calls: FakeCall[] = [];

  const handle = async (method: string, path: string, body: unknown, options?: RequestOptions) => {
    calls.push({ method, path, body, query: options?.query, signal: options?.signal });
    const key = `${method} ${path}`;
    if (!(key in routes)) throw new Error(`fakeApi: no route for ${key}`);
    const reply = routes[key];
    return typeof reply === 'function' ? (reply as (body: unknown) => unknown)(body) : reply;
  };

  const api = {
    get: (path, options) => handle('GET', path, undefined, options),
    post: (path, body, options) => handle('POST', path, body, options),
    patch: (path, body, options) => handle('PATCH', path, body, options),
    put: (path, body, options) => handle('PUT', path, body, options),
    del: (path, body, options) => handle('DELETE', path, body, options)
  } as Api;
  return { api, calls };
}
