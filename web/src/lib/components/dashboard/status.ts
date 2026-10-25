import type { TankParameter } from '#lib/api/params.js';
import { todayIn } from '#lib/format/local-time.js';

export type RangeState = 'none' | 'in' | 'out';

/** Whether the latest reading is inside the ideal range; 'none' when nothing was tested or no range is set. */
export function rangeState(
  p: Pick<TankParameter, 'ideal_low' | 'ideal_high' | 'latest'>
): RangeState {
  if (!p.latest || (p.ideal_low === null && p.ideal_high === null)) return 'none';
  return p.latest.in_range ? 'in' : 'out';
}

export interface DueGroups {
  /** Past their due time, longest overdue first. */
  overdue: TankParameter[];
  /** Due later today in the given zone, soonest first. */
  today: TankParameter[];
}

const byDue = (a: TankParameter, b: TankParameter) =>
  Date.parse(a.next_due ?? '') - Date.parse(b.next_due ?? '');

/** Sorts the enabled parameters that need a test into overdue and due today, where today is the calendar day in the zone. */
export function classifyDue(
  parameters: readonly TankParameter[],
  now: Date,
  timeZone: string
): DueGroups {
  const today = todayIn(timeZone, now);
  const groups: DueGroups = { overdue: [], today: [] };
  for (const p of parameters) {
    if (!p.enabled) continue;
    if (p.overdue) groups.overdue.push(p);
    else if (p.next_due !== null && todayIn(timeZone, new Date(p.next_due)) === today) {
      groups.today.push(p);
    }
  }
  groups.overdue.sort(byDue);
  groups.today.sort(byDue);
  return groups;
}
