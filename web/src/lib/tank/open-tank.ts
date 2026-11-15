import type { Tank } from '#lib/api/tanks.js';
import { getContext, setContext } from 'svelte';

/** The tank whose pages are open, read once by the layout for all of them; null until it arrives or when it cannot be read. */
export interface OpenTank {
  readonly tank: Tank | null;
}

const openTankKey = Symbol('open-tank');

export function provideOpenTank(source: () => Tank | null): void {
  setContext<OpenTank>(openTankKey, {
    get tank() {
      return source();
    }
  });
}

export function useOpenTank(): OpenTank {
  return getContext<OpenTank>(openTankKey);
}
