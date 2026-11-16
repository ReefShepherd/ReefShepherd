import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { badgeText, pollIntervalMs, startPolling, type PollDeps } from './poll.js';

// A page whose visibility the test flips by hand.
function fakePage(initiallyHidden = false) {
  let hidden = initiallyHidden;
  const listeners = new Set<() => void>();
  const signals: AbortSignal[] = [];
  const counts: number[] = [];
  let next = 1;
  const deps: PollDeps = {
    read: vi.fn(async (signal: AbortSignal) => {
      signals.push(signal);
      return next++;
    }),
    onCount: (count) => counts.push(count),
    hidden: () => hidden,
    onVisibilityChange(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    }
  };
  return {
    deps,
    signals,
    counts,
    listeners,
    setHidden(value: boolean) {
      hidden = value;
      for (const listener of [...listeners]) listener();
    }
  };
}

describe('startPolling', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('reads at once and then every 60 seconds', async () => {
    expect(pollIntervalMs).toBe(60_000);
    const page = fakePage();
    startPolling(page.deps);
    await vi.advanceTimersByTimeAsync(0);
    expect(page.deps.read).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(59_999);
    expect(page.deps.read).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(page.deps.read).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(120_000);
    expect(page.deps.read).toHaveBeenCalledTimes(4);
    expect(page.counts).toEqual([1, 2, 3, 4]);
  });

  it('does not read while the page is hidden', async () => {
    const page = fakePage(true);
    startPolling(page.deps);
    await vi.advanceTimersByTimeAsync(300_000);
    expect(page.deps.read).not.toHaveBeenCalled();
  });

  it('stops reading when the page is hidden and reads again, at once, when it is shown', async () => {
    const page = fakePage();
    startPolling(page.deps);
    await vi.advanceTimersByTimeAsync(0);
    expect(page.deps.read).toHaveBeenCalledTimes(1);

    page.setHidden(true);
    await vi.advanceTimersByTimeAsync(300_000);
    expect(page.deps.read).toHaveBeenCalledTimes(1);

    page.setHidden(false);
    await vi.advanceTimersByTimeAsync(0);
    expect(page.deps.read).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(page.deps.read).toHaveBeenCalledTimes(3);
  });

  it('restarts the interval when the page is shown, so reads stay 60 seconds apart', async () => {
    const page = fakePage();
    startPolling(page.deps);
    await vi.advanceTimersByTimeAsync(50_000);
    page.setHidden(true);
    page.setHidden(false);
    await vi.advanceTimersByTimeAsync(0);
    expect(page.deps.read).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(59_999);
    expect(page.deps.read).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(1);
    expect(page.deps.read).toHaveBeenCalledTimes(3);
  });

  it('aborts a read under way when the page is hidden and drops its answer', async () => {
    let finish: (count: number) => void = () => {};
    const page = fakePage();
    page.deps.read = vi.fn((signal: AbortSignal) => {
      page.signals.push(signal);
      return new Promise<number>((resolve) => (finish = resolve));
    });
    startPolling(page.deps);
    page.setHidden(true);
    expect(page.signals[0].aborted).toBe(true);
    finish(9);
    await vi.advanceTimersByTimeAsync(0);
    expect(page.counts).toEqual([]);
  });

  it('stops for good, aborting a read under way, and ignores the page afterwards', async () => {
    const page = fakePage();
    page.deps.read = vi.fn((signal: AbortSignal) => {
      page.signals.push(signal);
      return new Promise<number>(() => {});
    });
    const stop = startPolling(page.deps);
    stop();
    expect(page.signals[0].aborted).toBe(true);
    expect(page.listeners.size).toBe(0);
    await vi.advanceTimersByTimeAsync(300_000);
    expect(page.deps.read).toHaveBeenCalledTimes(1);
  });

  it('keeps the last count when a read fails and tries again on the next interval', async () => {
    const page = fakePage();
    page.deps.read = vi
      .fn<(signal: AbortSignal) => Promise<number>>()
      .mockResolvedValueOnce(4)
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(6);
    startPolling(page.deps);
    await vi.advanceTimersByTimeAsync(120_000);
    expect(page.counts).toEqual([4, 6]);
  });

  it('aborts a slow read when the next one starts', async () => {
    const page = fakePage();
    page.deps.read = vi.fn((signal: AbortSignal) => {
      page.signals.push(signal);
      return new Promise<number>(() => {});
    });
    startPolling(page.deps);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(page.signals.map((s) => s.aborted)).toEqual([true, false]);
  });
});

describe('badgeText', () => {
  it('shows nothing for zero, the number up to 99 and 99+ above', () => {
    expect(badgeText(0)).toBe('');
    expect(badgeText(-1)).toBe('');
    expect(badgeText(1)).toBe('1');
    expect(badgeText(99)).toBe('99');
    expect(badgeText(100)).toBe('99+');
    expect(badgeText(4000)).toBe('99+');
  });
});
