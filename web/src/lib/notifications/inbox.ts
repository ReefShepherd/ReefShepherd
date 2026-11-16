import { appPath } from '#lib/auth/next.js';
import type {
  MarkRead,
  Notification,
  NotificationPage,
  NotificationQuery
} from '#lib/api/notify.js';

export const inboxPageSize = 50;

/** The query for one page of the inbox, newest first. */
export function inboxQuery(unreadOnly: boolean, cursor: string | null): NotificationQuery {
  const query: NotificationQuery = { limit: inboxPageSize };
  if (unreadOnly) query.unread = true;
  if (cursor !== null) query.cursor = cursor;
  return query;
}

/** Adds a fetched page after the notifications already shown, skipping any that are there. */
export function appendPage(shown: readonly Notification[], page: NotificationPage): Notification[] {
  const known = new Set(shown.map((n) => n.id));
  return [...shown, ...page.items.filter((n) => !known.has(n.id))];
}

/** The same list with the given notifications marked read. */
export function markedRead(shown: readonly Notification[], ids: readonly number[]): Notification[] {
  return shown.map((n) => (ids.includes(n.id) && !n.read ? { ...n, read: true } : n));
}

/** What marking notifications read needs: the request, and the shared unread count to keep in step. */
export interface MarkDeps {
  markRead(which: MarkRead): Promise<void>;
  unread: { set(count: number): void; refresh(): Promise<void> };
}

/** Marks one notification read on the server and returns the list with it marked; one that is already read is left alone. */
export async function markOneRead(
  deps: MarkDeps,
  shown: readonly Notification[],
  id: number
): Promise<Notification[]> {
  const target = shown.find((n) => n.id === id);
  if (!target || target.read) return [...shown];
  await deps.markRead({ ids: [id] });
  // The exact count comes from the server; a failed read here is picked up by the next poll.
  deps.unread.refresh().catch(() => {});
  return markedRead(shown, [id]);
}

/** Marks every notification read on the server and clears the unread count. */
export async function markAllRead(deps: MarkDeps): Promise<void> {
  await deps.markRead({ all: true });
  deps.unread.set(0);
}

/** The link to open for a notification, or null when it is empty or leaves the app. */
export function inboxLink(link: string): string | null {
  return appPath.test(link) ? link : null;
}
