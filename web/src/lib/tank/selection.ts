import type { Tank } from '#lib/api/tanks.js';

/** Which tank should be current: the stored one if it is still active, else the user's default, else the first active tank; null when there are none. */
export function reconcileCurrentTank(
  stored: string | null,
  tanks: readonly Pick<Tank, 'id' | 'archived_at'>[],
  defaultTankId: number | null
): string | null {
  const active = tanks.filter((t) => t.archived_at === null);
  const has = (id: string | null) => id !== null && active.some((t) => String(t.id) === id);
  if (has(stored)) return stored;
  if (defaultTankId !== null && has(String(defaultTankId))) return String(defaultTankId);
  return active.length > 0 ? String(active[0].id) : null;
}

const tankPath = /^\/tanks\/([^/]+)(\/.*)?$/;

/** The same page of another tank, such as /tanks/2/tests for /tanks/1/tests; null when the path does not belong to one tank. */
export function retargetPath(pathname: string, tankId: string): string | null {
  const match = tankPath.exec(pathname);
  if (!match || !/^\d+$/.test(match[1])) return null;
  return `/tanks/${encodeURIComponent(tankId)}${match[2] ?? ''}`;
}
