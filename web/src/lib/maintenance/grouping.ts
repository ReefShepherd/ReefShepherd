import type { Task } from '#lib/api/maintenance.js';
import { todayIn } from '#lib/format/local-time.js';

export type TaskSegment = 'due' | 'upcoming' | 'all';

export const segments: readonly TaskSegment[] = ['due', 'upcoming', 'all'];

/** Where a task stands against the clock: late, due later today in the instance timezone, due on a later day, without a due date, or switched off. */
export type DueState = 'overdue' | 'today' | 'later' | 'unscheduled' | 'paused';

/** The calendar day of an instant in the instance timezone, as YYYY-MM-DD. */
function dayOf(instant: string, timeZone: string): string {
  return todayIn(timeZone, new Date(instant));
}

export function dueState(task: Task, now: Date, timeZone: string): DueState {
  if (task.paused) return 'paused';
  if (task.next_due_at === null) return 'unscheduled';
  if (Date.parse(task.next_due_at) < now.getTime()) return 'overdue';
  return dayOf(task.next_due_at, timeZone) === todayIn(timeZone, now) ? 'today' : 'later';
}

/** Whether the list for a segment shows the task; due means due by the end of today, so anything overdue is in it. */
export function inSegment(task: Task, segment: TaskSegment, now: Date, timeZone: string): boolean {
  if (segment === 'all') return true;
  if (task.paused || task.next_due_at === null) return false;
  const byToday = dayOf(task.next_due_at, timeZone) <= todayIn(timeZone, now);
  return segment === 'due' ? byToday : !byToday;
}

export function segmentCounts(
  tasks: readonly Task[],
  now: Date,
  timeZone: string
): Record<TaskSegment, number> {
  const counts: Record<TaskSegment, number> = { due: 0, upcoming: 0, all: tasks.length };
  for (const task of tasks) {
    if (inSegment(task, 'due', now, timeZone)) counts.due++;
    else if (inSegment(task, 'upcoming', now, timeZone)) counts.upcoming++;
  }
  return counts;
}

/** Soonest due first, then tasks without a due date, then paused ones, with equal ones by name. */
export function sortTasks(tasks: readonly Task[]): Task[] {
  const rank = (t: Task) => (t.paused ? 2 : t.next_due_at === null ? 1 : 0);
  return [...tasks].sort((a, b) => {
    if (rank(a) !== rank(b)) return rank(a) - rank(b);
    if (a.next_due_at !== null && b.next_due_at !== null && a.next_due_at !== b.next_due_at) {
      return Date.parse(a.next_due_at) - Date.parse(b.next_due_at);
    }
    return a.name.localeCompare(b.name) || a.id - b.id;
  });
}

export interface TaskSection {
  /** Null for the tasks of the whole system. */
  tankId: number | null;
  tankName: string | null;
  tasks: Task[];
}

/** The tasks of a segment in the order they are shown, soonest due first. */
export function segmentTasks(
  tasks: readonly Task[],
  segment: TaskSegment,
  now: Date,
  timeZone: string
): Task[] {
  return sortTasks(tasks.filter((t) => inSegment(t, segment, now, timeZone)));
}

/** Splits the tasks by tank, tanks by name and the whole system last; the order inside a section is kept. */
export function groupByTank(tasks: readonly Task[]): TaskSection[] {
  const byTank = new Map<number, TaskSection>();
  const system: TaskSection = { tankId: null, tankName: null, tasks: [] };
  for (const task of tasks) {
    if (task.tank_id === null) {
      system.tasks.push(task);
      continue;
    }
    let section = byTank.get(task.tank_id);
    if (!section) {
      section = { tankId: task.tank_id, tankName: task.tank_name, tasks: [] };
      byTank.set(task.tank_id, section);
    }
    section.tasks.push(task);
  }
  const sections = [...byTank.values()].sort(
    (a, b) =>
      (a.tankName ?? '').localeCompare(b.tankName ?? '') || (a.tankId ?? 0) - (b.tankId ?? 0)
  );
  if (system.tasks.length > 0) sections.push(system);
  return sections;
}

/** Puts a changed task in place of its old copy, or adds it when it is new. */
export function replaceTask(tasks: readonly Task[], updated: Task): Task[] {
  return tasks.some((t) => t.id === updated.id)
    ? tasks.map((t) => (t.id === updated.id ? updated : t))
    : [...tasks, updated];
}

export function removeTask(tasks: readonly Task[], id: number): Task[] {
  return tasks.filter((t) => t.id !== id);
}

export interface CardRows {
  shown: Task[];
  /** How many more tasks are due than the card has room for. */
  more: number;
}

export const cardRowLimit = 5;

/** The tasks the dashboard card lists: those due by the end of today, soonest first, up to the limit. */
export function cardRows(
  tasks: readonly Task[],
  now: Date,
  timeZone: string,
  limit: number = cardRowLimit
): CardRows {
  const due = segmentTasks(tasks, 'due', now, timeZone);
  return { shown: due.slice(0, limit), more: Math.max(0, due.length - limit) };
}
