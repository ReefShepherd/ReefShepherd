import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Me } from '#lib/api/users.js';

const me = vi.fn<() => Promise<Me>>();
const browser = vi.hoisted(() => ({ zone: undefined as string | undefined }));

vi.mock('#lib/api/users.js', () => ({ usersApi: { me: () => me(), updateMe: vi.fn() } }));
vi.mock('#lib/format/timezone.js', async (original) => ({
  ...(await original<typeof import('#lib/format/timezone.js')>()),
  browserTimeZone: () => browser.zone
}));
vi.mock('#lib/theme/theme.svelte.js', () => ({
  isThemeName: () => true,
  theme: { set: vi.fn() }
}));

const user = (timezone: string, id = 1): Me =>
  ({ id, theme: 'system', default_tank_id: null, instance: { timezone } }) as unknown as Me;

async function loadSession() {
  vi.resetModules();
  return (await import('./session.svelte.js')).session;
}

beforeEach(() => {
  me.mockReset();
  browser.zone = 'Asia/Kolkata';
});

describe('session.timeZone', () => {
  it('follows the instance timezone as soon as the user is known', async () => {
    me.mockResolvedValue(user('Asia/Tokyo'));
    const session = await loadSession();
    await session.load();
    expect(session.timeZone).toBe('Asia/Tokyo');
    me.mockResolvedValue(user('Europe/Berlin'));
    await session.load();
    expect(session.timeZone).toBe('Europe/Berlin');
  });

  it('falls back to the browser zone before the user is known and after a sign-out', async () => {
    const session = await loadSession();
    expect(session.timeZone).toBe('Asia/Kolkata');
    me.mockResolvedValue(user('Asia/Tokyo'));
    await session.load();
    session.clear();
    expect(session.timeZone).toBe('Asia/Kolkata');
  });

  it('ignores a zone name the browser does not know', async () => {
    me.mockResolvedValue(user('Not/AZone'));
    const session = await loadSession();
    await session.load();
    expect(session.timeZone).toBe('Asia/Kolkata');
  });

  it('uses UTC when the browser cannot name its zone', async () => {
    browser.zone = undefined;
    const session = await loadSession();
    expect(session.timeZone).toBe('UTC');
  });
});

describe('session.ready', () => {
  it('shares one read between callers made while it is under way', async () => {
    me.mockResolvedValue(user('UTC'));
    const session = await loadSession();
    const [a, b] = await Promise.all([session.ready(), session.ready()]);
    expect(a).toBe(b);
    expect(me).toHaveBeenCalledTimes(1);
  });

  it('does not read again once the user is known', async () => {
    me.mockResolvedValue(user('UTC'));
    const session = await loadSession();
    await session.ready();
    await session.ready();
    expect(me).toHaveBeenCalledTimes(1);
  });

  it('reads again after a failed read', async () => {
    me.mockRejectedValueOnce(new Error('offline'));
    const session = await loadSession();
    await expect(session.ready()).rejects.toThrow('offline');
    me.mockResolvedValue(user('UTC'));
    await expect(session.ready()).resolves.toMatchObject({ id: 1 });
    expect(me).toHaveBeenCalledTimes(2);
  });
});
