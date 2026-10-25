import { describe, expect, it } from 'vitest';
import { fakeApi } from '#lib/test/fakeApi.js';
import { createChartsApi } from './charts.js';

describe('chartsApi.series', () => {
  it('asks for the series of one parameter from an ISO start', async () => {
    const { api, calls } = fakeApi({ 'GET /tanks/3/parameters/12/series': { points: [] } });
    await createChartsApi(api).series(3, '12', new Date('2026-02-07T05:00:00Z'));
    expect(calls).toMatchObject([
      {
        method: 'GET',
        path: '/tanks/3/parameters/12/series',
        query: { from: '2026-02-07T05:00:00.000Z' }
      }
    ]);
  });

  it('sends no start for the whole history and escapes the parameter id', async () => {
    const { api, calls } = fakeApi({ 'GET /tanks/3/parameters/a%2Fb/series': { points: [] } });
    await createChartsApi(api).series(3, 'a/b', null);
    expect(calls[0].path).toBe('/tanks/3/parameters/a%2Fb/series');
    expect(calls[0].query).toEqual({ from: undefined });
  });
});
