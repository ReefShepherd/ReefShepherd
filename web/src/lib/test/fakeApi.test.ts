import { describe, expect, it } from 'vitest';
import { fakeApi } from './fakeApi.js';

describe('fakeApi', () => {
  it('answers a route with its value or with a function of the body', async () => {
    const { api, calls } = fakeApi({
      'GET /a': { n: 1 },
      'POST /b': (body: unknown) => ({ echoed: body })
    });
    expect(await api.get('/a', { query: { x: 2 } })).toEqual({ n: 1 });
    expect(await api.post('/b', { y: 3 })).toEqual({ echoed: { y: 3 } });
    expect(calls).toMatchObject([
      { method: 'GET', path: '/a', query: { x: 2 } },
      { method: 'POST', path: '/b', body: { y: 3 } }
    ]);
  });

  it('fails a request that has no route', async () => {
    const { api } = fakeApi();
    await expect(api.get('/missing')).rejects.toThrow('no route for GET /missing');
  });
});
