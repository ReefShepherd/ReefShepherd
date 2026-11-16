import { describe, expect, it } from 'vitest';
import { fakeApi } from '#lib/test/fakeApi.js';
import { emptySchedule } from '#lib/schedule/schedule.js';
import { createMaintenanceApi } from './maintenance.js';

const schedule = emptySchedule();

describe('maintenance requests', () => {
  it('lists presets without archived ones unless asked', async () => {
    const { api, calls } = fakeApi({ 'GET /maintenance/presets': { items: [] } });
    const maintenance = createMaintenanceApi(api);
    await maintenance.presets();
    await maintenance.presets(true);
    expect(calls.map((c) => c.query)).toEqual([
      { include_archived: undefined },
      { include_archived: true }
    ]);
  });

  it('filters tasks by the query it is given', async () => {
    const { api, calls } = fakeApi({ 'GET /maintenance/tasks': { items: [] } });
    await createMaintenanceApi(api).tasks({ tank_id: 3, include_tankless: true, status: 'all' });
    expect(calls[0]?.query).toEqual({ tank_id: 3, include_tankless: true, status: 'all' });
  });

  it('creates and changes tasks with the bodies it is given', async () => {
    const { api, calls } = fakeApi({
      'POST /maintenance/tasks': {},
      'PATCH /maintenance/tasks/5': {},
      'DELETE /maintenance/tasks/5': undefined
    });
    const maintenance = createMaintenanceApi(api);
    const created = { tank_id: null, name: 'RO/DI', schedule, last_done_on: '2026-11-01' };
    await maintenance.createTask(created);
    await maintenance.updateTask(5, { tank_id: null, paused: true });
    await maintenance.removeTask(5);
    expect(calls.map((c) => [c.method, c.path, c.body])).toEqual([
      ['POST', '/maintenance/tasks', created],
      ['PATCH', '/maintenance/tasks/5', { tank_id: null, paused: true }],
      ['DELETE', '/maintenance/tasks/5', undefined]
    ]);
  });

  it('completes a task and pages its history', async () => {
    const { api, calls } = fakeApi({
      'POST /maintenance/tasks/5/complete': { completion: {}, task: {} },
      'GET /maintenance/tasks/5/completions': { items: [], next_cursor: null },
      'PATCH /maintenance/completions/9': {},
      'DELETE /maintenance/completions/9': undefined
    });
    const maintenance = createMaintenanceApi(api);
    await maintenance.complete(5, { outcome: 'skipped', note: 'travelling' });
    await maintenance.completions(5, { cursor: 'c1', limit: 20 });
    await maintenance.updateCompletion(9, { note: 'x' });
    await maintenance.removeCompletion(9);
    expect(calls.map((c) => [c.method, c.path, c.body, c.query])).toEqual([
      [
        'POST',
        '/maintenance/tasks/5/complete',
        { outcome: 'skipped', note: 'travelling' },
        undefined
      ],
      ['GET', '/maintenance/tasks/5/completions', undefined, { cursor: 'c1', limit: 20 }],
      ['PATCH', '/maintenance/completions/9', { note: 'x' }, undefined],
      ['DELETE', '/maintenance/completions/9', undefined, undefined]
    ]);
  });

  it('hides and shows a preset through archived', async () => {
    const { api, calls } = fakeApi({ 'PATCH /maintenance/presets/2': {} });
    await createMaintenanceApi(api).updatePreset(2, { archived: true });
    expect(calls[0]?.body).toEqual({ archived: true });
  });
});
