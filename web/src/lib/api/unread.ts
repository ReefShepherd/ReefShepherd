import type { Api } from './client.js';

/** How many notifications the signed-in user has not read; kept apart from the rest of the notification API so the header stays small. */
export async function unreadCount(api: Api, signal?: AbortSignal): Promise<number> {
  return (await api.get<{ count: number }>('/notifications/unread-count', { signal })).count;
}
