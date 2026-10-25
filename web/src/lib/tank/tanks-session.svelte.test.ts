import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Me } from '#lib/api/users.js';
import type { Tank } from '#lib/api/tanks.js';

const me = vi.fn<(signal?: AbortSignal) => Promise<Me>>();
const list = vi.fn<() => Promise<Tank[]>>();

vi.mock('#lib/api/users.js', () => ({ usersApi: { me: (signal?: AbortSignal) => me(signal) } }));
vi.mock('#lib/api/tanks.js', () => ({ tanksApi: { list: () => list() } }));
vi.mock('#lib/theme/theme.svelte.js', () => ({
  isThemeName: () => false,
  theme: { set: vi.fn() }
}));

const tank = (id: number) => ({ id, archived_at: null }) as Tank;
const user = (defaultTank: number) =>
  ({ id: 1, default_tank_id: defaultTank, instance: { timezone: 'UTC' } }) as unknown as Me;

afterEach(() => {
  vi.unstubAllGlobals();
  me.mockReset();
  list.mockReset();
});

describe('tankList with the real session', () => {
  it('waits for the user read that wins before choosing, so the default tank is used', async () => {
    vi.resetModules();
    vi.stubGlobal('localStorage', { getItem: () => null, setItem: vi.fn(), removeItem: vi.fn() });
    const { session } = await import('#lib/session/session.svelte.js');
    const { currentTank } = await import('./current-tank.svelte.js');
    const { tankList } = await import('./tanks.svelte.js');

    list.mockResolvedValue([tank(2), tank(3)]);
    // Like a real request, the first read fails with an abort once it is cancelled.
    me.mockImplementationOnce(
      (signal) =>
        new Promise<Me>((_, reject) =>
          signal?.addEventListener('abort', () => reject(signal.reason))
        )
    );
    const loading = tankList.load();
    await new Promise((resolve) => setTimeout(resolve));
    let answer: (me: Me) => void = () => {};
    me.mockReturnValue(new Promise<Me>((resolve) => (answer = resolve)));
    const refreshing = session.load();
    await new Promise((resolve) => setTimeout(resolve));
    answer(user(3));
    await refreshing;
    await loading;

    expect(currentTank.id).toBe(3);
  });
});
