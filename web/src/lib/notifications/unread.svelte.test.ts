import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fakeApi } from '#lib/test/fakeApi.js';
import { unreadCount } from '#lib/api/unread.js';
import type { PollDeps } from './poll.js';
import { followSession, unread } from './unread.svelte.js';

function fakeDeps(): PollDeps {
  return {
    read: vi.fn(async () => 5),
    onCount: (count) => unread.set(count),
    hidden: () => false,
    onVisibilityChange: () => () => {}
  };
}

describe('unread store', () => {
  it('never goes below zero', () => {
    unread.set(3);
    expect(unread.count).toBe(3);
    unread.set(-2);
    expect(unread.count).toBe(0);
  });

  it('reads the count through the unread-count route', async () => {
    const { api, calls } = fakeApi({ 'GET /notifications/unread-count': { count: 7 } });
    expect(await unreadCount(api)).toBe(7);
    expect(calls.map((c) => `${c.method} ${c.path}`)).toEqual(['GET /notifications/unread-count']);
  });
});

describe('followSession', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => {
    vi.useRealTimers();
    unread.set(0);
  });

  it('reads only while signed in, and forgets the count and stops when the user signs out', async () => {
    const deps = fakeDeps();
    const follow = followSession(deps);
    follow.update(false);
    await vi.advanceTimersByTimeAsync(120_000);
    expect(deps.read).not.toHaveBeenCalled();

    follow.update(true);
    follow.update(true);
    await vi.advanceTimersByTimeAsync(0);
    expect(deps.read).toHaveBeenCalledTimes(1);
    expect(unread.count).toBe(5);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(deps.read).toHaveBeenCalledTimes(2);

    follow.update(false);
    expect(unread.count).toBe(0);
    await vi.advanceTimersByTimeAsync(300_000);
    expect(deps.read).toHaveBeenCalledTimes(2);

    follow.update(true);
    await vi.advanceTimersByTimeAsync(0);
    expect(deps.read).toHaveBeenCalledTimes(3);
    follow.stop();
  });

  it('stops reading when it is torn down', async () => {
    const deps = fakeDeps();
    const follow = followSession(deps);
    follow.update(true);
    await vi.advanceTimersByTimeAsync(0);
    expect(deps.read).toHaveBeenCalledTimes(1);
    follow.stop();
    await vi.advanceTimersByTimeAsync(300_000);
    expect(deps.read).toHaveBeenCalledTimes(1);
  });
});
