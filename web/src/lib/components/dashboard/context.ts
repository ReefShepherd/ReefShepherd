import type { TankParameter } from '#lib/api/params.js';
import { session } from '#lib/session/session.svelte.js';
import { getContext, setContext } from 'svelte';

/** What the cards read from the page that shows them. */
export interface DashboardContext {
  readonly tankId: number;
  /** The instance timezone, which decides what "today" means for due tests. */
  readonly timeZone: string;
  readonly parameters: readonly TankParameter[];
  parameter(id: number | null): TankParameter | undefined;
}

export const dashboardKey = Symbol('dashboard');

/** Makes the tank and its parameters available to the cards below the calling component; both are read again on every use. */
export function provideDashboard(source: {
  tankId: () => number;
  parameters: () => readonly TankParameter[];
}): void {
  setContext<DashboardContext>(dashboardKey, {
    get tankId() {
      return source.tankId();
    },
    get timeZone() {
      return session.timeZone;
    },
    get parameters() {
      return source.parameters();
    },
    parameter: (id) => source.parameters().find((p) => p.parameter_id === id)
  });
}

export function useDashboard(): DashboardContext {
  return getContext<DashboardContext>(dashboardKey);
}
