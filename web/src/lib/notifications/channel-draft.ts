import {
  eventTypes,
  maskedSecret,
  type Channel,
  type ChannelChanges,
  type ChannelKind,
  type ConfigByKind,
  type EmailSecurity,
  type EventType,
  type NewChannel
} from '#lib/api/notify.js';
import type { FieldErrors } from '#lib/forms/codes.js';
import {
  collectErrors,
  parseEmailList,
  validateEmailList,
  validateHttpUrl,
  validateName,
  validatePort,
  validateRequired
} from '#lib/forms/validation.js';

export const channelNameMax = 100;
export const defaultNtfyServer = 'https://ntfy.sh';

/** A setting the server keeps secret: whether it holds a value, what the admin typed to replace it, and whether the admin chose to remove it. */
export interface SecretDraft {
  stored: boolean;
  text: string;
  remove: boolean;
}

/** Everything the channel form edits; the fields of other kinds stay empty. */
export interface ChannelDraft {
  name: string;
  kind: ChannelKind;
  enabled: boolean;
  events: EventType[];
  /** Discord, Slack and webhook address. */
  url: SecretDraft;
  server: string;
  topic: SecretDraft;
  token: SecretDraft;
  host: string;
  port: string;
  security: EmailSecurity;
  username: string;
  password: SecretDraft;
  from: string;
  to: string;
}

const noSecret = (): SecretDraft => ({ stored: false, text: '', remove: false });

/** A blank draft for a new channel of this kind, subscribed to every event. */
export function emptyDraft(kind: ChannelKind): ChannelDraft {
  return {
    name: '',
    kind,
    enabled: true,
    events: [...eventTypes],
    url: noSecret(),
    server: kind === 'ntfy' ? defaultNtfyServer : '',
    topic: noSecret(),
    token: noSecret(),
    host: '',
    port: kind === 'email' ? '587' : '',
    security: 'starttls',
    username: '',
    password: noSecret(),
    from: '',
    to: ''
  };
}

/** The same draft for another kind: the name, switch and events carry over and the kind's own settings start blank. */
export function changeKind(draft: ChannelDraft, kind: ChannelKind): ChannelDraft {
  return { ...emptyDraft(kind), name: draft.name, enabled: draft.enabled, events: draft.events };
}

// A secret the server shows masked is remembered as stored; the draft never holds the masked text.
const secretOf = (value: unknown): SecretDraft => ({
  stored: typeof value === 'string' && value !== '',
  text: '',
  remove: false
});

const textOf = (value: unknown): string => (typeof value === 'string' ? value : '');

/** The draft for editing a channel, with every secret left empty. */
export function draftFromChannel(channel: Channel): ChannelDraft {
  const draft = emptyDraft(channel.kind);
  const config = channel.config as unknown as Record<string, unknown>;
  draft.name = channel.name;
  draft.enabled = channel.enabled;
  draft.events = eventTypes.filter((type) => channel.events.includes(type));
  draft.url = secretOf(config.url);
  draft.server = textOf(config.server) || draft.server;
  draft.topic = secretOf(config.topic);
  draft.token = secretOf(config.token);
  draft.host = textOf(config.host);
  draft.port = typeof config.port === 'number' ? String(config.port) : '';
  draft.security =
    (['starttls', 'tls', 'none'] as const).find((s) => s === config.security) ?? 'starttls';
  draft.username = textOf(config.username);
  draft.password = secretOf(config.password);
  draft.from = textOf(config.from);
  draft.to = Array.isArray(config.to) ? config.to.join(', ') : '';
  return draft;
}

/** What to send for a secret: the new text, else the mask that keeps the stored value, else nothing; a password keeps its spaces and the rest are trimmed. */
export function secretValue(secret: SecretDraft, keepSpaces = false): string {
  const text = keepSpaces ? secret.text : secret.text.trim();
  if (text !== '') return text;
  return secret.stored && !secret.remove ? maskedSecret : '';
}

/** Leaves a blank optional setting out of the config. */
const optional = (value: string): string | undefined => (value === '' ? undefined : value);

/** The config object for the draft's kind, with an untouched secret sent as the mask. */
export function buildConfig(draft: ChannelDraft): ConfigByKind[ChannelKind] {
  switch (draft.kind) {
    case 'discord':
    case 'slack':
    case 'webhook':
      return { url: secretValue(draft.url) };
    case 'ntfy':
      return {
        server: optional(draft.server.trim()),
        topic: secretValue(draft.topic),
        token: optional(secretValue(draft.token))
      };
    case 'email':
      return {
        host: draft.host.trim(),
        port: Number(draft.port.trim()),
        security: draft.security,
        username: optional(draft.username.trim()),
        password: optional(secretValue(draft.password, true)),
        from: draft.from.trim(),
        to: parseEmailList(draft.to)
      };
  }
}

/** The body for creating the channel. */
export function newChannel(draft: ChannelDraft): NewChannel {
  return {
    name: draft.name.trim(),
    kind: draft.kind,
    config: buildConfig(draft),
    events: draft.events,
    enabled: draft.enabled
  };
}

const sameEvents = (a: readonly EventType[], b: readonly EventType[]) =>
  a.length === b.length && a.every((type) => b.includes(type));

/** The changes to save: the name, events and switch only when they differ, and always the whole config because the server replaces it as one piece. */
export function channelChanges(original: Channel, draft: ChannelDraft): ChannelChanges {
  const changes: ChannelChanges = { config: buildConfig(draft) };
  if (draft.name.trim() !== original.name) changes.name = draft.name.trim();
  if (draft.enabled !== original.enabled) changes.enabled = draft.enabled;
  if (!sameEvents(draft.events, original.events)) changes.events = draft.events;
  return changes;
}

/** Ticks or unticks an event, keeping the events in their usual order. */
export function toggleEvent(
  events: readonly EventType[],
  type: EventType,
  on: boolean
): EventType[] {
  return eventTypes.filter((candidate) => (candidate === type ? on : events.includes(candidate)));
}

// A secret that is already stored may be left empty; a new one must be filled in.
const requiredSecret = (secret: SecretDraft) =>
  secretValue(secret) === '' ? ('required' as const) : null;

const typedSecretUrl = (secret: SecretDraft) =>
  secret.text.trim() === '' ? requiredSecret(secret) : validateHttpUrl(secret.text);

/** The problems the browser can see before sending, keyed by the server's field names. */
export function validateDraft(draft: ChannelDraft): FieldErrors {
  const common = {
    name: validateName(draft.name, channelNameMax),
    events: draft.events.length === 0 ? ('required' as const) : null
  };
  switch (draft.kind) {
    case 'discord':
    case 'slack':
    case 'webhook':
      return collectErrors({ ...common, 'config.url': typedSecretUrl(draft.url) });
    case 'ntfy':
      return collectErrors({
        ...common,
        'config.server': draft.server.trim() === '' ? null : validateHttpUrl(draft.server),
        'config.topic': requiredSecret(draft.topic)
      });
    case 'email':
      return collectErrors({
        ...common,
        'config.host': validateRequired(draft.host),
        'config.port': validatePort(draft.port),
        'config.from': validateRequired(draft.from),
        'config.to': validateEmailList(draft.to)
      });
  }
}

/** The server field names the form for this kind has an input for. */
export function shownFields(kind: ChannelKind): string[] {
  const common = ['name', 'events'];
  switch (kind) {
    case 'discord':
    case 'slack':
    case 'webhook':
      return [...common, 'config.url'];
    case 'ntfy':
      return [...common, 'config.server', 'config.topic', 'config.token'];
    case 'email':
      return [
        ...common,
        'config.host',
        'config.port',
        'config.security',
        'config.username',
        'config.password',
        'config.from',
        'config.to'
      ];
  }
}
