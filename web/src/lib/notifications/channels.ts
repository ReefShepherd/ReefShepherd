import type { Channel, createNotifyApi } from '#lib/api/notify.js';
import type { Me } from '#lib/api/users.js';

/** The list with a saved channel in place of its old copy. */
export function replaceChannel(channels: readonly Channel[], saved: Channel): Channel[] {
  return channels.map((c) => (c.id === saved.id ? saved : c));
}

export function removeChannel(channels: readonly Channel[], id: number): Channel[] {
  return channels.filter((c) => c.id !== id);
}

/** The channels an administrator manages; for a member the server is not asked, because it would refuse. */
export async function channelsFor(
  me: Pick<Me, 'role'>,
  notify: Pick<ReturnType<typeof createNotifyApi>, 'channels'>,
  signal?: AbortSignal
): Promise<Channel[]> {
  return me.role === 'admin' ? notify.channels(signal) : [];
}
