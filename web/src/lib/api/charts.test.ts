import { describe, expect, it, vi } from 'vitest';
import type { Api, RequestOptions } from './client.js';
import { createChartsApi } from './charts.js';

function fakeApi(replies: unknown[]) {
  const calls: { path: string; query: RequestOptions['query'] }[] = [];
  const get = vi.fn(async (path: string, options?: RequestOptions) => {
    calls.push({ path, query: options?.query });
    return replies.shift();
  });
  return { api: { get } as unknown as Api, calls };
}

describe('chartsApi.series', () => {
  it('asks for the series of one parameter from an ISO start', async () => {
    const { api, calls } = fakeApi([{ points: [] }]);
    await createChartsApi(api).series('3', '12', new Date('2026-02-07T05:00:00Z'));
    expect(calls).toEqual([
      {
        path: '/tanks/3/parameters/12/series',
        query: { from: '2026-02-07T05:00:00.000Z' }
      }
    ]);
  });

  it('sends no start for the whole history and escapes ids', async () => {
    const { api, calls } = fakeApi([{ points: [] }]);
    await createChartsApi(api).series('a/b', '1', null);
    expect(calls[0].path).toBe('/tanks/a%2Fb/parameters/1/series');
    expect(calls[0].query).toEqual({ from: undefined });
  });
});
