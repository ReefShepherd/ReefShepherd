/** How often the unread count is read while the page is visible. */
export const pollIntervalMs = 60_000;

/** The largest count shown as a number; more reads as "99+". */
export const badgeCap = 99;

export interface PollDeps {
  /** Reads the current unread count. */
  read(signal: AbortSignal): Promise<number>;
  onCount(count: number): void;
  hidden(): boolean;
  /** Calls the listener whenever the page is shown or hidden and returns a function that stops listening. */
  onVisibilityChange(listener: () => void): () => void;
}

/** The text on the bell: nothing for no unread notifications, the number up to the cap, then "99+". */
export function badgeText(count: number): string {
  if (count <= 0) return '';
  return count > badgeCap ? `${badgeCap}+` : String(count);
}

/** Reads the count now and then every interval while the page is visible, pausing while it is hidden; the returned function stops it. */
export function startPolling(deps: PollDeps): () => void {
  let timer: ReturnType<typeof setInterval> | undefined;
  let running: AbortController | undefined;

  async function readNow(): Promise<void> {
    running?.abort();
    const mine = new AbortController();
    running = mine;
    try {
      const count = await deps.read(mine.signal);
      if (!mine.signal.aborted) deps.onCount(count);
    } catch {
      // The last count stays and the next read tries again.
    }
  }

  function pause(): void {
    clearInterval(timer);
    timer = undefined;
    running?.abort();
    running = undefined;
  }

  function sync(): void {
    pause();
    if (deps.hidden()) return;
    void readNow();
    timer = setInterval(() => void readNow(), pollIntervalMs);
  }

  const unlisten = deps.onVisibilityChange(sync);
  sync();
  return () => {
    unlisten();
    pause();
  };
}
