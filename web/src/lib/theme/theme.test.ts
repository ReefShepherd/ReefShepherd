import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

type ThemeModule = typeof import('./theme.svelte.js');

function stubBrowser(initial: string | undefined, storage: Pick<Storage, 'setItem'>) {
  const dataset: Record<string, string | undefined> = { theme: initial };
  vi.stubGlobal('document', { documentElement: { dataset } });
  vi.stubGlobal('localStorage', storage);
  return dataset;
}

async function load(): Promise<ThemeModule> {
  vi.resetModules();
  return import('./theme.svelte.js');
}

beforeEach(() => vi.unstubAllGlobals());
afterEach(() => vi.unstubAllGlobals());

describe('isThemeName', () => {
  it('accepts exactly the four themes', async () => {
    const { isThemeName } = await load();
    for (const name of ['light', 'dark', 'oled', 'system']) expect(isThemeName(name)).toBe(true);
    for (const bad of ['', 'Dark', 'amoled', null, undefined, 3])
      expect(isThemeName(bad)).toBe(false);
  });
});

describe('theme store', () => {
  it('starts from the theme the no-flash script put on <html>', async () => {
    stubBrowser('oled', { setItem: vi.fn() });
    const { theme } = await load();
    expect(theme.current).toBe('oled');
  });

  it('falls back to system when <html> carries an unknown theme', async () => {
    stubBrowser('bogus', { setItem: vi.fn() });
    const { theme } = await load();
    expect(theme.current).toBe('system');
  });

  it('set applies the theme to <html> and caches it for the next load', async () => {
    const setItem = vi.fn();
    const dataset = stubBrowser('system', { setItem });
    const { theme } = await load();
    theme.set('dark');
    expect(theme.current).toBe('dark');
    expect(dataset.theme).toBe('dark');
    expect(setItem).toHaveBeenCalledExactlyOnceWith('theme', 'dark');
  });

  it('set still applies the theme when storage is unavailable', async () => {
    const dataset = stubBrowser('system', {
      setItem: () => {
        throw new Error('blocked');
      }
    });
    const { theme } = await load();
    expect(() => theme.set('oled')).not.toThrow();
    expect(dataset.theme).toBe('oled');
    expect(theme.current).toBe('oled');
  });
});
