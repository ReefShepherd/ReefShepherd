import { tanksApi, type Tank } from '#lib/api/tanks.js';
import { cachedResource } from '#lib/cache/cached-resource.svelte.js';
import { session } from '#lib/session/session.svelte.js';
import { currentTank } from './current-tank.svelte.js';
import { reconcileCurrentTank } from './selection.js';

// The user's default tank decides the fallback, so the list is only applied once /me has been read or has failed.
const tanks = cachedResource(async (signal) => {
  const [list] = await Promise.all([
    tanksApi.list(true, signal),
    session.ready().catch(() => null)
  ]);
  signal.throwIfAborted();
  const next = reconcileCurrentTank(currentTank.id, list, session.me?.default_tank_id ?? null);
  if (next !== currentTank.id) currentTank.set(next);
  return list;
});

/** The tanks of the instance, shared by the tank switcher and the pages; null until the first load finishes. */
export const tankList = {
  get tanks(): Tank[] | null {
    return tanks.value;
  },
  get active(): Tank[] {
    return (tanks.value ?? []).filter((t) => t.archived_at === null);
  },
  /** Reads the tanks, or joins the read already under way; drops a stored current tank that is gone or archived. */
  load: tanks.load,
  /** Reads the tanks again after a change, even if a read is already under way. */
  refresh: tanks.refresh,
  clear: tanks.invalidate
};
