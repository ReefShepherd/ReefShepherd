import type { InstanceSettings, SettingsPatch } from '#lib/api/settings.js';
import type { FieldErrors } from '#lib/forms/codes.js';
import { collectErrors, validateTime, validateWholeNumber } from '#lib/forms/validation.js';

export const keepRange = { min: 1, max: 365 } as const;

/** What the backup settings form edits; the keep count is typed as text so a half-typed number survives. */
export interface BackupDraft {
  enabled: boolean;
  time: string;
  keep: string;
}

/** The keep count as trimmed text, however the input handed it over (a number input gives a number, or null when empty). */
export function keepText(keep: unknown): string {
  return keep === null || keep === undefined ? '' : String(keep).trim();
}

export function draftFromSettings(settings: InstanceSettings): BackupDraft {
  return {
    enabled: settings.backup_enabled,
    time: settings.backup_time,
    keep: String(settings.backup_keep)
  };
}

/** The problems the form can find itself, keyed by the server's field names. */
export function draftErrors(draft: BackupDraft): FieldErrors {
  return collectErrors({
    backup_time: validateTime(draft.time),
    backup_keep: validateWholeNumber(keepText(draft.keep), keepRange.min, keepRange.max)
  });
}

/** Only the backup settings the form changed, so an untouched field is never sent. */
export function backupPatch(original: InstanceSettings, draft: BackupDraft): SettingsPatch {
  const patch: SettingsPatch = {};
  if (draft.enabled !== original.backup_enabled) patch.backup_enabled = draft.enabled;
  const time = draft.time.trim();
  if (time !== original.backup_time) patch.backup_time = time;
  const keep = keepText(draft.keep);
  if (keep !== '' && Number(keep) !== original.backup_keep) patch.backup_keep = Number(keep);
  return patch;
}
