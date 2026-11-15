import { describe, expect, it } from 'vitest';
import { fakeApi } from '#lib/test/fakeApi.js';
import { createDashboardApi } from './dashboard.js';

const reply = { cards: [] };

describe('dashboardApi', () => {
  it('reads and resets the layout of one tank', async () => {
    const { api, calls } = fakeApi({
      'GET /tanks/3/dashboard': reply,
      'POST /tanks/3/dashboard/reset': reply
    });
    const dashboard = createDashboardApi(api);
    await dashboard.get(3);
    await dashboard.reset(3);
    expect(calls).toMatchObject([
      { method: 'GET', path: '/tanks/3/dashboard', body: undefined },
      { method: 'POST', path: '/tanks/3/dashboard/reset', body: undefined }
    ]);
  });

  it('puts the whole body', async () => {
    const { api, calls } = fakeApi({ 'PUT /tanks/3/dashboard': reply });
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
    await createDashboardApi(api).save(3, body);
    expect(calls).toMatchObject([{ method: 'PUT', path: '/tanks/3/dashboard', body }]);
  });
});
