import { tankPath } from '#lib/api/tank-id.js';
import type { Tank } from '#lib/api/tanks.js';

/** Which tank should be current: the stored one if it is still active, else the user's default, else the first active tank; null when there are none. */
export function reconcileCurrentTank(
  stored: number | null,
  tanks: readonly Pick<Tank, 'id' | 'archived_at'>[],
  defaultTankId: number | null
): number | null {
  const active = tanks.filter((t) => t.archived_at === null);
  const has = (id: number | null) => id !== null && active.some((t) => t.id === id);
  if (has(stored)) return stored;
  if (defaultTankId !== null && has(defaultTankId)) return defaultTankId;
  return active.length > 0 ? active[0].id : null;
}

const tankPage = /^\/tanks\/([^/]+)(\/.*)?$/;

/** The same page of another tank, such as /tanks/2/tests for /tanks/1/tests; null when the path does not belong to one tank. */
export function retargetPath(pathname: string, tankId: number): string | null {
  const match = tankPage.exec(pathname);
  if (!match || !/^\d+$/.test(match[1])) return null;
  return tankPath(tankId) + (match[2] ?? '');
}
