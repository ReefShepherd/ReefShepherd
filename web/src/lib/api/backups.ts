import { api as defaultApi, apiBase, type Api } from './client.js';

export type BackupKind = 'scheduled' | 'premigrate' | 'prerestore';

export interface Backup {
  name: string;
  kind: BackupKind;
  size_bytes: number;
  /** RFC 3339 UTC. */
  created_at: string;
}

/** The same-origin address that serves a backup file as a download; the session cookie authorises it. */
export function downloadUrl(name: string): string {
  return `${apiBase}/backups/${encodeURIComponent(name)}/download`;
}

export function createBackupsApi(api: Api) {
  return {
    list: (signal?: AbortSignal) => api.list<Backup>('/backups', { signal }),
    create: (signal?: AbortSignal) => api.post<Backup>('/backups', undefined, { signal }),
    remove: (name: string, signal?: AbortSignal) =>
      api.del<void>(`/backups/${encodeURIComponent(name)}`, undefined, { signal }),
    downloadUrl
  };
}

export const backupsApi = createBackupsApi(defaultApi);
