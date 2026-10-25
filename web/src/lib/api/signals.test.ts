import { describe, expect, it } from 'vitest';
import { fakeApi } from '#lib/test/fakeApi.js';
import { createAuthApi } from './auth.js';
import { createChartsApi } from './charts.js';
import { createDashboardApi } from './dashboard.js';
import { createParamsApi } from './params.js';
import { createSettingsApi } from './settings.js';
import { createTanksApi } from './tanks.js';
import { createUsersApi } from './users.js';

// Every call of every module hands the caller's abort signal to the client.
describe('abort signals', () => {
  const signal = new AbortController().signal;
  const ok = { parameters: [], results: [], cards: [] };

  it('reach the client from every module call', async () => {
    const { api, calls } = fakeApi(
      new Proxy({}, { has: () => true, get: () => ok }) as Record<string, unknown>
    );
    const auth = createAuthApi(api);
    const users = createUsersApi(api);
    const settings = createSettingsApi(api);
    const tanks = createTanksApi(api);
    const params = createParamsApi(api);

    const all: Promise<unknown>[] = [
      auth.config(signal),
      auth.login('u', 'p', signal),
      auth.verifyCode('1', signal),
      auth.verifyRecoveryCode('r', signal),
      auth.logout(signal),
      auth.changePassword('n', 'c', signal),
      auth.beginTotp(signal),
      auth.confirmTotp('1', signal),
      auth.disableTotp({}, signal),
      auth.sessions(signal),
      auth.revokeSession(1, signal),
      users.setup({} as never, signal),
      users.me(signal),
      users.updateMe({}, signal),
      users.list(signal),
      users.create({} as never, signal),
      users.update(1, {}, signal),
      users.remove(1, signal),
      users.resetMfa(1, signal),
      settings.get(signal),
      settings.update({}, signal),
      settings.units(signal),
      tanks.list(false, signal),
      tanks.get(1, signal),
      tanks.create({} as never, signal),
      tanks.update(1, {}, signal),
      tanks.archive(1, signal),
      tanks.unarchive(1, signal),
      tanks.remove(1, 'n', signal),
      params.definitions(false, signal),
      params.createDefinition({} as never, signal),
      params.updateDefinition(1, {}, signal),
      params.archiveDefinition(1, signal),
      params.unarchiveDefinition(1, signal),
      params.removeDefinition(1, signal),
      params.tankParameters(1, signal),
      params.saveTankParameter(1, 2, {}, signal),
      params.createResults(1, { results: [] }, signal),
      params.results(1, {}, signal),
      params.updateResult(1, {}, signal),
      params.removeResult(1, signal),
      createDashboardApi(api).get(1, signal),
      createDashboardApi(api).save(1, { cards: [] }, signal),
      createDashboardApi(api).reset(1, signal),
      createChartsApi(api).series(1, 2, null, signal)
    ];
    await Promise.all(all);

    expect(calls).toHaveLength(all.length);
    expect(calls.filter((c) => c.signal !== signal).map((c) => `${c.method} ${c.path}`)).toEqual(
      []
    );
  });
});
