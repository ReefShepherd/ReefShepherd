import { describe, expect, it } from 'vitest';
import { fakeApi } from '#lib/test/fakeApi.js';
import { createNotifyApi } from './notify.js';
import { unreadCount } from './unread.js';

describe('notification requests', () => {
  it('sends the unread filter only when it is on', async () => {
    const { api, calls } = fakeApi({ 'GET /notifications': { items: [], next_cursor: null } });
    const notify = createNotifyApi(api);
    await notify.notifications({ unread: true, limit: 10 });
    await notify.notifications({ unread: false });
    expect(calls.map((c) => c.query)).toEqual([{ unread: true, limit: 10 }, { unread: undefined }]);
  });

  it('marks notifications read by id or all at once', async () => {
    const { api, calls } = fakeApi({ 'POST /notifications/read': undefined });
    const notify = createNotifyApi(api);
    await notify.markRead({ ids: [1, 2] });
    await notify.markRead({ all: true });
    expect(calls.map((c) => c.body)).toEqual([{ ids: [1, 2] }, { all: true }]);
  });

  it('reads the unread count as a number', async () => {
    const { api } = fakeApi({ 'GET /notifications/unread-count': { count: 3 } });
    expect(await unreadCount(api)).toBe(3);
  });

  it('manages channels and sends a test through the channel path', async () => {
    const { api, calls } = fakeApi({
      'POST /notification-channels': {},
      'PATCH /notification-channels/4': {},
      'DELETE /notification-channels/4': undefined,
      'POST /notification-channels/4/test': { sent: false, error: 'boom' }
    });
    const notify = createNotifyApi(api);
    await notify.createChannel({ name: 'n', kind: 'webhook', config: { url: 'http://x' } });
    await notify.updateChannel(4, { enabled: false });
    await notify.removeChannel(4);
    expect(await notify.testChannel(4)).toEqual({ sent: false, error: 'boom' });
    expect(calls.map((c) => `${c.method} ${c.path}`)).toEqual([
      'POST /notification-channels',
      'PATCH /notification-channels/4',
      'DELETE /notification-channels/4',
      'POST /notification-channels/4/test'
    ]);
  });
});
