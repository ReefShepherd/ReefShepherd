import { api } from '#lib/api/client.js';
import { unreadCount } from '#lib/api/unread.js';
import { startPolling, type PollDeps } from './poll.js';

let count = $state(0);

/** How many notifications the signed-in user has not read, shared by the bell and the inbox. */
export const unread = {
  get count(): number {
    return count;
  },
  set(next: number): void {
    count = Math.max(0, next);
  },
  /** Reads the count from the server now. */
  async refresh(signal?: AbortSignal): Promise<void> {
    count = await unreadCount(api, signal);
  }
};

const pageDeps: PollDeps = {
  read: (signal) => unreadCount(api, signal),
  onCount: (next) => unread.set(next),
  hidden: () => document.hidden,
  onVisibilityChange(listener) {
    document.addEventListener('visibilitychange', listener);
    return () => document.removeEventListener('visibilitychange', listener);
  }
};

/** Keeps the count fresh while the user is signed in; update(false) stops polling and forgets the count, and stop() ends it for good. */
export function followSession(deps: PollDeps = pageDeps): {
  update(signedIn: boolean): void;
  stop(): void;
} {
  let stopPolling: (() => void) | undefined;
  const stop = () => {
    stopPolling?.();
    stopPolling = undefined;
  };
  return {
    update(signedIn) {
      if (signedIn) {
        stopPolling ??= startPolling(deps);
      } else {
        stop();
        count = 0;
      }
    },
    stop
  };
}
