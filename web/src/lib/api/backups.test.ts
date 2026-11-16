import { describe, expect, it } from 'vitest';
import { fakeApi } from '#lib/test/fakeApi.js';
import { createBackupsApi, downloadUrl } from './backups.js';

const name = 'reefshepherd-20260101T030000Z.db';

describe('backups', () => {
  it('points the download link at the same-origin API path', () => {
    expect(downloadUrl(name)).toBe(`/api/v1/backups/${name}/download`);
    expect(downloadUrl('a b/c')).toBe('/api/v1/backups/a%20b%2Fc/download');
  });

  it('creates and deletes a backup by name', async () => {
    const { api, calls } = fakeApi({
      'POST /backups': {},
      [`DELETE /backups/${name}`]: undefined
    });
    const backups = createBackupsApi(api);
    await backups.create();
    await backups.remove(name);
    expect(calls.map((c) => `${c.method} ${c.path}`)).toEqual([
      'POST /backups',
      `DELETE /backups/${name}`
    ]);
  });
});
