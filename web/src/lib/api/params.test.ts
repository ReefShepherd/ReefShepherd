import { describe, expect, it, vi } from 'vitest';
import type { Api, RequestOptions } from './client.js';
import { createParamsApi } from './params.js';
import { createTanksApi } from './tanks.js';

function fakeApi() {
  const calls: { method: string; path: string; query?: RequestOptions['query'] }[] = [];
  const record = (method: string) =>
    vi.fn(async (path: string, ...rest: unknown[]) => {
      const options = rest.at(-1) as RequestOptions | undefined;
      calls.push({ method, path, query: options?.query });
      return { results: [] };
    });
  const api = {
    get: record('GET'),
    post: record('POST'),
    put: record('PUT'),
    patch: record('PATCH'),
    del: record('DELETE')
  } as unknown as Api;
  return { api, calls };
}

describe('paramsApi', () => {
  it('reads the parameters of a tank, passing the abort signal on', async () => {
    const { api, calls } = fakeApi();
    const signal = new AbortController().signal;
    const get = vi.mocked(api.get);
    await createParamsApi(api).tankParameters(3, signal);
    expect(calls[0].path).toBe('/tanks/3/parameters');
    expect(get.mock.calls[0][1]).toMatchObject({ signal });
  });

  it('escapes the tank id in every tank path', async () => {
    const { api, calls } = fakeApi();
    const params = createParamsApi(api);
    await params.tankParameters('a/b');
    await params.saveTankParameter('a/b', 4, { enabled: true });
    await params.createResults('a/b', { results: [] });
    await params.results('a/b');
    expect(calls.map((c) => c.path)).toEqual([
      '/tanks/a%2Fb/parameters',
      '/tanks/a%2Fb/parameters/4',
      '/tanks/a%2Fb/results',
      '/tanks/a%2Fb/results'
    ]);
  });
});

describe('tanksApi', () => {
  it('escapes the tank id when reading one tank', async () => {
    const { api, calls } = fakeApi();
    await createTanksApi(api).get('a/b');
    await createTanksApi(api).get(7);
    expect(calls.map((c) => c.path)).toEqual(['/tanks/a%2Fb', '/tanks/7']);
  });
});
