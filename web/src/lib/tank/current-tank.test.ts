import { afterEach, describe, expect, it, vi } from 'vitest';

type Module = typeof import('./current-tank.svelte.js');

function stubStorage(storage: Partial<Storage>) {
  vi.stubGlobal('localStorage', storage);
}

async function load(): Promise<Module> {
  vi.resetModules();
  return import('./current-tank.svelte.js');
}

afterEach(() => vi.unstubAllGlobals());

describe('currentTank', () => {
  it('starts from the id saved under current_tank', async () => {
    const getItem = vi.fn(() => '7');
    stubStorage({ getItem });
    const { currentTank } = await load();
    expect(getItem).toHaveBeenCalledWith('current_tank');
    expect(currentTank.id).toBe('7');
  });

  it('starts empty when nothing is saved or the saved value is not a plain id', async () => {
    for (const stored of [null, '', '../admin', '3/tests', 'a b']) {
      stubStorage({ getItem: () => stored });
      const { currentTank } = await load();
      expect(currentTank.id, String(stored)).toBeNull();
    }
  });

  it('starts empty when storage throws', async () => {
    stubStorage({
      getItem: () => {
        throw new Error('blocked');
      }
    });
    const { currentTank } = await load();
    expect(currentTank.id).toBeNull();
  });

  it('set updates the id and saves it, and set(null) clears both', async () => {
    const setItem = vi.fn();
    const removeItem = vi.fn();
    stubStorage({ getItem: () => null, setItem, removeItem });
    const { currentTank } = await load();
    currentTank.set('4');
    expect(currentTank.id).toBe('4');
    expect(setItem).toHaveBeenCalledExactlyOnceWith('current_tank', '4');
    currentTank.set(null);
    expect(currentTank.id).toBeNull();
    expect(removeItem).toHaveBeenCalledExactlyOnceWith('current_tank');
  });

  it('set still updates the id when storage throws', async () => {
    const fail = () => {
      throw new Error('blocked');
    };
    stubStorage({ getItem: () => null, setItem: fail, removeItem: fail });
    const { currentTank } = await load();
    expect(() => currentTank.set('2')).not.toThrow();
    expect(currentTank.id).toBe('2');
    expect(() => currentTank.set(null)).not.toThrow();
    expect(currentTank.id).toBeNull();
  });
});
