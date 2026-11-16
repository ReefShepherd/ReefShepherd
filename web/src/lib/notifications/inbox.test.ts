import { describe, expect, it } from 'vitest';
import { createNotifyApi, type Channel, type Notification } from '#lib/api/notify.js';
import { fakeApi } from '#lib/test/fakeApi.js';
import { channelsFor, removeChannel, replaceChannel } from './channels.js';
import {
  appendPage,
  inboxLink,
  inboxPageSize,
  inboxQuery,
  markAllRead,
  markedRead,
  markOneRead
} from './inbox.js';

function note(id: number, read = false): Notification {
  return {
    id,
    event_type: 'tests_due',
    severity: 'info',
    title: `n${id}`,
    body: '',
    link: '',
    tank_id: null,
    created_at: '2026-01-01T00:00:00Z',
    read
  };
}

describe('inboxQuery', () => {
  it('asks for one page, filtered to unread only when chosen', () => {
    expect(inboxQuery(false, null)).toEqual({ limit: inboxPageSize });
    expect(inboxQuery(true, 'abc')).toEqual({ limit: inboxPageSize, unread: true, cursor: 'abc' });
  });
});

describe('appendPage', () => {
  it('adds a page after the shown rows without repeating one', () => {
    const shown = [note(3), note(2)];
    const next = appendPage(shown, { items: [note(2), note(1)], next_cursor: null });
    expect(next.map((n) => n.id)).toEqual([3, 2, 1]);
  });
});

describe('markedRead', () => {
  it('marks only the listed notifications and leaves the rest alone', () => {
    const shown = [note(3), note(2), note(1, true)];
    const next = markedRead(shown, [2, 1]);
    expect(next.map((n) => n.read)).toEqual([false, true, true]);
    expect(next[0]).toBe(shown[0]);
    expect(next[2]).toBe(shown[2]);
    expect(shown[1].read).toBe(false);
  });
});

describe('marking read', () => {
  function deps() {
    const sent: unknown[] = [];
    const counts: number[] = [];
    let refreshes = 0;
    return {
      sent,
      counts,
      refreshes: () => refreshes,
      deps: {
        markRead: async (which: unknown) => void sent.push(which),
        unread: {
          set: (n: number) => void counts.push(n),
          refresh: async () => void refreshes++
        }
      }
    };
  }

  it('marks one notification by id, refreshes the count and returns the marked list', async () => {
    const d = deps();
    const shown = [note(2), note(1)];
    const next = await markOneRead(d.deps, shown, 2);
    expect(d.sent).toEqual([{ ids: [2] }]);
    expect(d.refreshes()).toBe(1);
    expect(next.map((n) => n.read)).toEqual([true, false]);
    expect(shown[0].read).toBe(false);
  });

  it('sends nothing for a notification that is read already or not in the list', async () => {
    const d = deps();
    const shown = [note(2, true)];
    expect(await markOneRead(d.deps, shown, 2)).toEqual(shown);
    expect(await markOneRead(d.deps, shown, 9)).toEqual(shown);
    expect(d.sent).toEqual([]);
    expect(d.refreshes()).toBe(0);
  });

  it('leaves the list unmarked when the server refuses', async () => {
    const d = deps();
    d.deps.markRead = async () => {
      throw new Error('refused');
    };
    await expect(markOneRead(d.deps, [note(1)], 1)).rejects.toThrow('refused');
    expect(d.refreshes()).toBe(0);
  });

  it('marks everything read at once and clears the count', async () => {
    const d = deps();
    await markAllRead(d.deps);
    expect(d.sent).toEqual([{ all: true }]);
    expect(d.counts).toEqual([0]);
  });
});

describe('inboxLink', () => {
  it('opens paths inside the app and nothing else', () => {
    expect(inboxLink('/tanks/2/tests')).toBe('/tanks/2/tests');
    expect(inboxLink('')).toBeNull();
    expect(inboxLink('https://evil.example')).toBeNull();
    expect(inboxLink('//evil.example')).toBeNull();
    expect(inboxLink('/\\evil.example')).toBeNull();
    expect(inboxLink('/tanks\\2')).toBeNull();
    expect(inboxLink('javascript:alert(1)')).toBeNull();
  });

  it('rejects the characters browsers strip from an address, which could hide a second slash', () => {
    expect(inboxLink('/\t/evil.example/tab')).toBeNull();
    expect(inboxLink('/\r/evil.example')).toBeNull();
    expect(inboxLink('/\n/evil.example')).toBeNull();
    expect(inboxLink('/\u0000/evil.example')).toBeNull();
    expect(inboxLink('/\u001f/evil.example')).toBeNull();
    expect(inboxLink('/\u007f/evil.example')).toBeNull();
    expect(inboxLink('/tanks/2\n')).toBeNull();
  });
});

describe('channelsFor', () => {
  it('asks the server for channels only on behalf of an administrator', async () => {
    const { api, calls } = fakeApi({ 'GET /notification-channels': { items: [{ id: 1 }] } });
    const notify = createNotifyApi(api);
    expect(await channelsFor({ role: 'member' }, notify)).toEqual([]);
    expect(calls).toHaveLength(0);
    expect(await channelsFor({ role: 'admin' }, notify)).toEqual([{ id: 1 }]);
    expect(calls.map((c) => `${c.method} ${c.path}`)).toEqual(['GET /notification-channels']);
  });
});

describe('channel lists', () => {
  const a = { id: 1, name: 'a' } as Channel;
  const b = { id: 2, name: 'b' } as Channel;

  it('replaces a channel by id and removes one', () => {
    expect(replaceChannel([a, b], { ...b, name: 'B' }).map((c) => c.name)).toEqual(['a', 'B']);
    expect(removeChannel([a, b], 1)).toEqual([b]);
  });
});
