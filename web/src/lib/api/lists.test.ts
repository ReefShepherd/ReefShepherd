import { describe, expect, it } from 'vitest';
import { fakeApi } from '#lib/test/fakeApi.js';
import { createAuthApi } from './auth.js';
import { createParamsApi } from './params.js';
import { createSettingsApi } from './settings.js';
import { createTanksApi } from './tanks.js';
import { createUsersApi } from './users.js';

const items = [{ id: 1 }, { id: 2 }];

// A method that read the reply as a bare array would hand back the whole envelope instead of the items.
describe('collection reads', () => {
  const cases: [string, string, (api: ReturnType<typeof fakeApi>['api']) => Promise<unknown>][] = [
    ['GET /tanks', 'tanks.list', (api) => createTanksApi(api).list()],
    ['GET /users', 'users.list', (api) => createUsersApi(api).list()],
    ['GET /me/sessions', 'auth.sessions', (api) => createAuthApi(api).sessions()],
    ['GET /units', 'settings.units', (api) => createSettingsApi(api).units()],
    ['GET /parameters', 'params.definitions', (api) => createParamsApi(api).definitions()],
    [
      'POST /tanks/4/results',
      'params.createResults',
      (api) => createParamsApi(api).createResults(4, { results: [] })
    ]
  ];

  it.each(cases)('%s gives the items from %s', async (route, _name, call) => {
    const { api } = fakeApi({ [route]: { items } });
    expect(await call(api)).toEqual(items);
  });
});
