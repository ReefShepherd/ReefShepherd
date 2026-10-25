import { describe, expect, it } from 'vitest';
import { fakeApi } from '#lib/test/fakeApi.js';
import { createParamsApi } from './params.js';
import { createTanksApi } from './tanks.js';

describe('paramsApi', () => {
  it('reads the parameters of a tank, passing the abort signal on', async () => {
    const { api, calls } = fakeApi({ 'GET /tanks/3/parameters': { parameters: [] } });
    const signal = new AbortController().signal;
    await createParamsApi(api).tankParameters(3, signal);
    expect(calls).toMatchObject([{ method: 'GET', path: '/tanks/3/parameters', signal }]);
  });

  it('builds every tank path from the tank id', async () => {
    const { api, calls } = fakeApi({
      'PUT /tanks/5/parameters/4': {},
      'POST /tanks/5/results': { results: [] },
      'GET /tanks/5/results': { results: [], next_cursor: null }
    });
    const params = createParamsApi(api);
    await params.saveTankParameter(5, 4, { enabled: true });
    await params.createResults(5, { results: [] });
    await params.results(5, { limit: 10 });
    expect(calls.map((c) => `${c.method} ${c.path}`)).toEqual([
      'PUT /tanks/5/parameters/4',
      'POST /tanks/5/results',
      'GET /tanks/5/results'
    ]);
    expect(calls[2].query).toEqual({ limit: 10 });
  });

  it('unwraps the parameter list and the stored results', async () => {
    const { api } = fakeApi({
      'GET /parameters': { parameters: [{ id: 1 }] },
      'POST /tanks/2/results': { results: [{ id: 9 }] }
    });
    const params = createParamsApi(api);
    expect(await params.definitions()).toEqual([{ id: 1 }]);
    expect(await params.createResults(2, { results: [] })).toEqual([{ id: 9 }]);
  });
});

describe('tanksApi', () => {
  it('reads and changes one tank under its own path', async () => {
    const { api, calls } = fakeApi({
      'GET /tanks/7': {},
      'PATCH /tanks/7': {},
      'POST /tanks/7/archive': {},
      'DELETE /tanks/7': undefined
    });
    const tanks = createTanksApi(api);
    await tanks.get(7);
    await tanks.update(7, { name: 'Nano' });
    await tanks.archive(7);
    await tanks.remove(7, 'Nano');
    expect(calls.map((c) => `${c.method} ${c.path}`)).toEqual([
      'GET /tanks/7',
      'PATCH /tanks/7',
      'POST /tanks/7/archive',
      'DELETE /tanks/7'
    ]);
    expect(calls[3].query).toEqual({ confirm: 'Nano' });
  });
});
