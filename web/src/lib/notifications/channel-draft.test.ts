import { describe, expect, it } from 'vitest';
import { maskedSecret, type Channel, type ChannelKind } from '#lib/api/notify.js';
import {
  buildConfig,
  changeKind,
  channelChanges,
  defaultNtfyServer,
  draftFromChannel,
  emptyDraft,
  newChannel,
  secretValue,
  shownFields,
  toggleEvent,
  validateDraft
} from './channel-draft.js';

function channel(kind: ChannelKind, config: unknown, extra: Partial<Channel> = {}): Channel {
  return {
    id: 1,
    name: 'Kitchen',
    kind,
    config: config as Channel['config'],
    events: ['maintenance_due', 'tests_due', 'out_of_range', 'backup_failed'],
    enabled: true,
    last_sent_at: null,
    last_error: '',
    last_error_at: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...extra
  };
}

const discord = () => channel('discord', { url: maskedSecret });
const ntfy = () =>
  channel('ntfy', { server: 'https://ntfy.example', topic: maskedSecret, token: maskedSecret });
const email = () =>
  channel('email', {
    host: 'smtp.example.com',
    port: 465,
    security: 'tls',
    username: 'reef',
    password: maskedSecret,
    from: 'reef@example.com',
    to: ['a@example.com', 'b@example.com']
  });

describe('draftFromChannel', () => {
  it('never puts a masked secret into a field, only that one is stored', () => {
    const d = draftFromChannel(discord());
    expect(d.url).toEqual({ stored: true, text: '', remove: false });
    const n = draftFromChannel(ntfy());
    expect(n.topic).toEqual({ stored: true, text: '', remove: false });
    expect(n.token).toEqual({ stored: true, text: '', remove: false });
    const e = draftFromChannel(email());
    expect(e.password).toEqual({ stored: true, text: '', remove: false });
    for (const secret of [d.url, n.topic, n.token, e.password]) {
      expect(secret.text).not.toContain('*');
    }
  });

  it('shows an unset optional secret as not stored', () => {
    const d = draftFromChannel(channel('ntfy', { server: 'https://ntfy.sh', topic: maskedSecret }));
    expect(d.token.stored).toBe(false);
  });

  it('reads the plain settings back into text fields', () => {
    const d = draftFromChannel(email());
    expect(d).toMatchObject({
      name: 'Kitchen',
      host: 'smtp.example.com',
      port: '465',
      security: 'tls',
      username: 'reef',
      from: 'reef@example.com',
      to: 'a@example.com, b@example.com'
    });
  });

  it('keeps the usual server for an ntfy channel that did not store one', () => {
    expect(draftFromChannel(channel('ntfy', { topic: maskedSecret })).server).toBe(
      defaultNtfyServer
    );
  });
});

describe('secrets in the saved config', () => {
  it('sends the mask back for a secret the admin did not touch', () => {
    expect(buildConfig(draftFromChannel(discord()))).toEqual({ url: maskedSecret });
    expect(buildConfig(draftFromChannel(ntfy()))).toEqual({
      server: 'https://ntfy.example',
      topic: maskedSecret,
      token: maskedSecret
    });
    expect(buildConfig(draftFromChannel(email()))).toMatchObject({ password: maskedSecret });
  });

  it('sends what the admin typed instead of the mask', () => {
    const d = draftFromChannel(discord());
    d.url.text = ' https://discord.example/hook ';
    expect(buildConfig(d)).toEqual({ url: 'https://discord.example/hook' });
    const e = draftFromChannel(email());
    e.password.text = ' new pass ';
    expect(buildConfig(e)).toMatchObject({ password: ' new pass ' });
  });

  it('leaves an optional secret out when the admin removes it', () => {
    const d = draftFromChannel(ntfy());
    d.token.remove = true;
    expect(buildConfig(d)).not.toHaveProperty('token', maskedSecret);
    expect(JSON.parse(JSON.stringify(buildConfig(d)))).toEqual({
      server: 'https://ntfy.example',
      topic: maskedSecret
    });
  });

  it('prefers a newly typed value over a removal', () => {
    expect(secretValue({ stored: true, text: 'x', remove: true })).toBe('x');
    expect(secretValue({ stored: false, text: '', remove: false })).toBe('');
  });

  it('always sends the whole config when saving changes, and the other fields only when they differ', () => {
    const original = discord();
    const d = draftFromChannel(original);
    expect(channelChanges(original, d)).toEqual({ config: { url: maskedSecret } });
    d.name = ' Renamed ';
    d.enabled = false;
    d.events = ['tests_due'];
    expect(channelChanges(original, d)).toEqual({
      config: { url: maskedSecret },
      name: 'Renamed',
      enabled: false,
      events: ['tests_due']
    });
  });

  it('does not treat the same events in another order as a change', () => {
    const original = discord();
    const d = draftFromChannel(original);
    d.events = [...d.events].reverse();
    expect(channelChanges(original, d)).not.toHaveProperty('events');
  });
});

describe('new channels', () => {
  it('starts subscribed to every event with the usual defaults per kind', () => {
    expect(emptyDraft('discord').events).toHaveLength(4);
    expect(emptyDraft('ntfy').server).toBe(defaultNtfyServer);
    expect(emptyDraft('email')).toMatchObject({ port: '587', security: 'starttls' });
  });

  it('builds the request body for an email channel', () => {
    const d = emptyDraft('email');
    Object.assign(d, {
      name: ' Mail ',
      host: ' smtp.example.com ',
      from: 'reef@example.com',
      to: 'a@example.com, , b@example.com'
    });
    d.password.text = 'secret';
    expect(newChannel(d)).toEqual({
      name: 'Mail',
      kind: 'email',
      config: {
        host: 'smtp.example.com',
        port: 587,
        security: 'starttls',
        username: undefined,
        password: 'secret',
        from: 'reef@example.com',
        to: ['a@example.com', 'b@example.com']
      },
      events: ['maintenance_due', 'tests_due', 'out_of_range', 'backup_failed'],
      enabled: true
    });
  });

  it('keeps the name, switch and events when the kind changes', () => {
    const d = emptyDraft('discord');
    d.name = 'Reef';
    d.enabled = false;
    d.events = ['tests_due'];
    d.url.text = 'https://x.example';
    const next = changeKind(d, 'email');
    expect(next).toMatchObject({
      kind: 'email',
      name: 'Reef',
      enabled: false,
      events: ['tests_due']
    });
    expect(next.url.text).toBe('');
  });
});

describe('toggleEvent', () => {
  it('adds and removes an event and keeps the usual order', () => {
    expect(toggleEvent(['tests_due'], 'maintenance_due', true)).toEqual([
      'maintenance_due',
      'tests_due'
    ]);
    expect(toggleEvent(['maintenance_due', 'tests_due'], 'maintenance_due', false)).toEqual([
      'tests_due'
    ]);
  });
});

describe('validateDraft', () => {
  it('asks for a name, an event and a new secret', () => {
    const d = emptyDraft('discord');
    d.events = [];
    expect(validateDraft(d)).toEqual({
      name: 'required',
      events: 'required',
      'config.url': 'required'
    });
  });

  it('lets a stored secret stay empty but checks one that is typed', () => {
    const d = draftFromChannel(discord());
    expect(validateDraft(d)).toEqual({});
    d.url.text = 'not a url';
    expect(validateDraft(d)).toEqual({ 'config.url': 'invalid' });
    d.url.text = 'ftp://example.com/x';
    expect(validateDraft(d)).toEqual({ 'config.url': 'invalid' });
  });

  it('requires an ntfy topic unless one is stored and checks a typed server', () => {
    const d = emptyDraft('ntfy');
    d.name = 'Phone';
    expect(validateDraft(d)).toEqual({ 'config.topic': 'required' });
    d.topic.text = 'abc';
    d.server = 'ntfy.example.com';
    expect(validateDraft(d)).toEqual({ 'config.server': 'invalid' });
    d.server = '';
    expect(validateDraft(d)).toEqual({});
    expect(validateDraft(draftFromChannel(ntfy()))).toEqual({});
  });

  it('checks the mail server, port, sender and recipients', () => {
    const d = emptyDraft('email');
    d.name = 'Mail';
    expect(validateDraft(d)).toEqual({
      'config.host': 'required',
      'config.from': 'required',
      'config.to': 'required'
    });
    Object.assign(d, { host: 'h', from: 'f@example.com', to: 'a@example.com, nope' });
    d.port = '70000';
    expect(validateDraft(d)).toEqual({ 'config.port': 'out_of_range', 'config.to': 'invalid' });
    d.port = 'x';
    d.to = 'a@example.com';
    expect(validateDraft(d)).toEqual({ 'config.port': 'invalid' });
    d.port = '25';
    expect(validateDraft(d)).toEqual({});
  });

  it('does not ask for a mail password, because the server may not need one', () => {
    const d = draftFromChannel(email());
    d.password = { stored: false, text: '', remove: false };
    expect(validateDraft(d)).toEqual({});
  });
});

describe('shownFields', () => {
  it('lists the inputs of each kind', () => {
    expect(shownFields('slack')).toEqual(['name', 'events', 'config.url']);
    expect(shownFields('ntfy')).toContain('config.token');
    expect(shownFields('email')).toContain('config.security');
    expect(shownFields('email')).not.toContain('config.url');
  });
});
