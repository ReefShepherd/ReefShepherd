import { tanksApi, type Tank } from '#lib/api/tanks.js';
import { session } from '#lib/session/session.svelte.js';
import { currentTank } from './current-tank.svelte.js';
import { reconcileCurrentTank } from './selection.js';

let list = $state<Tank[] | null>(null);
let pending: Promise<Tank[]> | null = null;

// The user's default tank decides the fallback, so the list is only applied once /me has been read or has failed.
function fetchTanks(): Promise<Tank[]> {
  const request = Promise.all([tanksApi.list(true), session.ready().catch(() => null)])
    .then(([tanks]) => {
      list = tanks;
      const next = reconcileCurrentTank(currentTank.id, tanks, session.me?.default_tank_id ?? null);
      if (next !== currentTank.id) currentTank.set(next);
      return tanks;
    })
    .finally(() => {
      if (pending === request) pending = null;
    });
  pending = request;
  return request;
}

/** The tanks of the instance, shared by the tank switcher and the pages; null until the first load finishes. */
export const tankList = {
  get tanks(): Tank[] | null {
    return list;
  },
  get active(): Tank[] {
    return (list ?? []).filter((t) => t.archived_at === null);
  },
  /** Reads the tanks, or joins the read already under way; drops a stored current tank that is gone or archived. */
  load(): Promise<Tank[]> {
    return pending ?? fetchTanks();
  },
  /** Reads the tanks again after a change, even if a read is already under way. */
  refresh(): Promise<Tank[]> {
    return fetchTanks();
  },
  clear(): void {
    list = null;
  }
};
