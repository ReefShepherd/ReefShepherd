import { describe, expect, it, vi } from 'vitest';
import type { Api } from './client.js';
import { createDashboardApi } from './dashboard.js';

function fakeApi() {
  const calls: { method: string; path: string; body?: unknown }[] = [];
  const reply = { cards: [] };
  const record = (method: string) =>
    vi.fn(async (path: string, ...rest: unknown[]) => {
      calls.push({ method, path, body: method === 'GET' ? undefined : rest[0] });
      return reply;
    });
  const api = { get: record('GET'), put: record('PUT'), post: record('POST') } as unknown as Api;
  return { api, calls };
}

describe('dashboardApi', () => {
  it('reads and resets the layout of one tank', async () => {
    const { api, calls } = fakeApi();
    const dashboard = createDashboardApi(api);
    await dashboard.get('3');
    await dashboard.reset('3');
    expect(calls).toEqual([
      { method: 'GET', path: '/tanks/3/dashboard', body: undefined },
      { method: 'POST', path: '/tanks/3/dashboard/reset', body: undefined }
    ]);
  });

  it('puts the whole body and escapes the tank id', async () => {
    const { api, calls } = fakeApi();
    const body = {
      cards: [
        {
          card_type: 'tests_due',
          parameter_id: null,
          position: 0,
          size: 'wide' as const,
          options: {}
        }
      ]
    };
    await createDashboardApi(api).save('a/b', body);
    expect(calls).toEqual([{ method: 'PUT', path: '/tanks/a%2Fb/dashboard', body }]);
  });
});
