import { api as defaultApi, type Api, type Page } from './client.js';

export const channelKinds = ['discord', 'slack', 'ntfy', 'webhook', 'email'] as const;
export type ChannelKind = (typeof channelKinds)[number];

export const eventTypes = [
  'maintenance_due',
  'tests_due',
  'out_of_range',
  'backup_failed'
] as const;
export type EventType = (typeof eventTypes)[number];

export type Severity = 'info' | 'warning' | 'critical';

/** What the server shows in place of a stored secret; sending it back keeps the stored value. */
export const maskedSecret = '********';

export interface DiscordConfig {
  url: string;
}

export interface SlackConfig {
  url: string;
}

export interface NtfyConfig {
  /** The ntfy server; the server uses https://ntfy.sh when it is left out. */
  server?: string;
  topic: string;
  token?: string;
}

export interface WebhookConfig {
  url: string;
}

export type EmailSecurity = 'starttls' | 'tls' | 'none';

export interface EmailConfig {
  host: string;
  port: number;
  security: EmailSecurity;
  username?: string;
  password?: string;
  from: string;
  to: string[];
}

export interface ConfigByKind {
  discord: DiscordConfig;
  slack: SlackConfig;
  ntfy: NtfyConfig;
  webhook: WebhookConfig;
  email: EmailConfig;
}

export interface Channel {
  id: number;
  name: string;
  kind: ChannelKind;
  /** The kind's settings, with every secret that is set shown as the masked value. */
  config: ConfigByKind[ChannelKind];
  events: EventType[];
  enabled: boolean;
  last_sent_at: string | null;
  last_error: string;
  last_error_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface NewChannel {
  name: string;
  kind: ChannelKind;
  config: ConfigByKind[ChannelKind];
  /** Omitted to subscribe to every event. */
  events?: EventType[];
  enabled?: boolean;
}

/** Fields to change on a channel (its kind cannot change); config replaces the whole object, and a secret sent masked keeps its stored value. */
export interface ChannelChanges {
  name?: string;
  config?: ConfigByKind[ChannelKind];
  events?: EventType[];
  enabled?: boolean;
}

/** The outcome of a test send; a refused send is a result, not an error. */
export interface TestSend {
  sent: boolean;
  error: string;
}

export interface Notification {
  id: number;
  event_type: EventType;
  severity: Severity;
  title: string;
  body: string;
  /** A path inside the app, empty when there is nothing to open. */
  link: string;
  tank_id: number | null;
  created_at: string;
  read: boolean;
}

export type NotificationPage = Page<Notification>;

export interface NotificationQuery {
  unread?: boolean;
  cursor?: string;
  limit?: number;
}

/** Which notifications to mark read: the listed ones or every one. */
export type MarkRead = { ids: number[] } | { all: true };

export function createNotifyApi(api: Api) {
  return {
    channels: (signal?: AbortSignal) => api.list<Channel>('/notification-channels', { signal }),
    createChannel: (channel: NewChannel, signal?: AbortSignal) =>
      api.post<Channel>('/notification-channels', channel, { signal }),
    updateChannel: (id: number, changes: ChannelChanges, signal?: AbortSignal) =>
      api.patch<Channel>(`/notification-channels/${id}`, changes, { signal }),
    removeChannel: (id: number, signal?: AbortSignal) =>
      api.del<void>(`/notification-channels/${id}`, undefined, { signal }),
    testChannel: (id: number, signal?: AbortSignal) =>
      api.post<TestSend>(`/notification-channels/${id}/test`, undefined, { signal }),

    notifications: (query: NotificationQuery = {}, signal?: AbortSignal) =>
      api.listPage<Notification>('/notifications', {
        signal,
        query: { ...query, unread: query.unread || undefined }
      }),
    markRead: (which: MarkRead, signal?: AbortSignal) =>
      api.post<void>('/notifications/read', which, { signal })
  };
}

export const notifyApi = createNotifyApi(defaultApi);
