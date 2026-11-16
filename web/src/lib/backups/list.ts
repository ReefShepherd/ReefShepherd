import type { Backup, BackupKind } from '#lib/api/backups.js';
import { isApiError } from '#lib/api/client.js';
import { errorText } from '#lib/api/error-text.js';
import { m } from '#lib/paraglide/messages.js';

/** Why "Back up now" failed; a conflict means another backup took the same second. */
export function createFailureText(error: unknown): string {
  return isApiError(error) && error.status === 409 ? m.backups_collision() : errorText(error);
}

const newestFirst = (a: Backup, b: Backup) =>
  Date.parse(b.created_at) - Date.parse(a.created_at) || (a.name < b.name ? 1 : -1);

/** The list with the backup added (or replaced by name), newest first like the server's own order. */
export function addBackup(list: Backup[], backup: Backup): Backup[] {
  return [...list.filter((b) => b.name !== backup.name), backup].sort(newestFirst);
}

export function removeBackup(list: Backup[], name: string): Backup[] {
  return list.filter((b) => b.name !== name);
}

/** The name of the row after the named one, where focus goes once that row is gone; null when it was the last row or is not listed. */
export function nextAfterRemoval(list: Backup[], name: string): string | null {
  const index = list.findIndex((b) => b.name === name);
  return index === -1 ? null : (list[index + 1]?.name ?? null);
}

export function kindLabel(kind: BackupKind): string {
  switch (kind) {
    case 'scheduled':
      return m.backups_kind_scheduled();
    case 'premigrate':
      return m.backups_kind_premigrate();
    case 'prerestore':
      return m.backups_kind_prerestore();
  }
}
