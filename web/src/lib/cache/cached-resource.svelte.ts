/** A value read from the server and kept for every screen that needs it. */
export interface CachedResource<T> {
  /** The latest value, or null until a read has finished. */
  readonly value: T | null;
  /** The read already under way, or a new one. */
  load(): Promise<T>;
  /** Starts a new read, cancelling any read under way, so the result cannot predate a change just made; callers of the cancelled read get the new read's result. */
  refresh(): Promise<T>;
  /** Replaces the value with a fresher copy the caller already holds; a read under way is cancelled and its callers get the new value. */
  set(next: T): void;
  /** Forgets the value; a read under way is cancelled and rejects with an AbortError. */
  invalidate(): void;
}

/** Keeps one value and at most one live read of it; a cancelled read never stores its result. */
export function cachedResource<T>(read: (signal: AbortSignal) => Promise<T>): CachedResource<T> {
  let value = $state.raw<T | null>(null);
  type Read = { controller: AbortController; promise: Promise<T> };
  let running: Read | null = null;

  function cancel(): void {
    running?.controller.abort();
    running = null;
  }

  function start(): Promise<T> {
    cancel();
    const controller = new AbortController();
    // A read cancelled by a newer read answers with that one's result, or with the value set since; with neither it rejects.
    const superseded = () =>
      running?.promise ??
      (value === null ? Promise.reject(controller.signal.reason) : Promise.resolve(value));
    const mine: Read = {
      controller,
      promise: read(controller.signal)
        .then(
          (next) => {
            if (controller.signal.aborted) return superseded();
            value = next;
            return next;
          },
          (e: unknown) => {
            if (controller.signal.aborted) return superseded();
            throw e;
          }
        )
        .finally(() => {
          if (running === mine) running = null;
        })
    };
    running = mine;
    return mine.promise;
  }

  return {
    get value() {
      return value;
    },
    load: () => running?.promise ?? start(),
    refresh: start,
    set(next) {
      cancel();
      value = next;
    },
    invalidate() {
      cancel();
      value = null;
    }
  };
}
