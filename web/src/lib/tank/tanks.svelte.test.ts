import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Tank } from '#lib/api/tanks.js';

const list = vi.fn<() => Promise<Tank[]>>();
const session = vi.hoisted(() => ({
  me: null as { default_tank_id: number | null } | null,
  ready: (): Promise<unknown> => Promise.resolve()
}));

vi.mock('#lib/api/tanks.js', () => ({ tanksApi: { list: () => list() } }));
vi.mock('#lib/session/session.svelte.js', () => ({ session }));

const tank = (id: number, archived = false): Tank => ({
  id,
  name: `Tank ${id}`,
  tank_type: 'reef',
  volume: null,
  volume_unit: 'l',
  started_on: null,
  note: '',
  sort_order: 0,
  archived_at: archived ? '2026-01-01T00:00:00Z' : null,
  created_at: '',
  updated_at: ''
});

async function load(stored: string | null) {
  vi.resetModules();
  vi.stubGlobal('localStorage', {
    getItem: () => stored,
    setItem: vi.fn(),
    removeItem: vi.fn()
  });
  const current = await import('./current-tank.svelte.js');
  const store = await import('./tanks.svelte.js');
  return { currentTank: current.currentTank, tankList: store.tankList };
}

afterEach(() => {
  vi.unstubAllGlobals();
  list.mockReset();
  session.me = null;
  session.ready = () => Promise.resolve();
});

describe('tankList', () => {
  it('clears a stored current tank whose tank is gone and picks one that exists', async () => {
    list.mockResolvedValue([tank(2), tank(3)]);
    const { currentTank, tankList } = await load('7');
    await tankList.load();
    expect(currentTank.id).toBe(2);
  });

  it('clears the current tank when no active tank is left', async () => {
    list.mockResolvedValue([tank(1, true)]);
    const { currentTank, tankList } = await load('1');
    await tankList.load();
    expect(currentTank.id).toBeNull();
    expect(tankList.active).toEqual([]);
  });

  it('keeps a stored tank that still exists', async () => {
    list.mockResolvedValue([tank(1), tank(2)]);
    const { currentTank, tankList } = await load('2');
    await tankList.load();
    expect(currentTank.id).toBe(2);
  });

  it('waits for the user before choosing, so the default tank wins over the first one', async () => {
    list.mockResolvedValue([tank(2), tank(3)]);
    let arrive = () => {};
    session.ready = () =>
      new Promise((resolve) => {
        arrive = () => {
          session.me = { default_tank_id: 3 };
          resolve(undefined);
        };
      });
    const { currentTank, tankList } = await load(null);
    const loading = tankList.load();
    await Promise.resolve();
    expect(tankList.tanks).toBeNull();
    expect(currentTank.id).toBeNull();
    arrive();
    await loading;
    expect(currentTank.id).toBe(3);
  });

  it('still lists the tanks when the user cannot be read', async () => {
    list.mockResolvedValue([tank(2), tank(3)]);
    session.ready = () => Promise.reject(new Error('offline'));
    const { currentTank, tankList } = await load(null);
    await tankList.load();
    expect(tankList.active.map((t) => t.id)).toEqual([2, 3]);
    expect(currentTank.id).toBe(2);
  });

  it('shares one read between callers made while it is under way', async () => {
    list.mockResolvedValue([tank(1)]);
    const { tankList } = await load(null);
    await Promise.all([tankList.load(), tankList.load()]);
    expect(list).toHaveBeenCalledTimes(1);
  });
});
