import type { InstanceSettings, SettingsPatch } from '#lib/api/settings.js';
import { normalizeTimeZone } from '#lib/format/timezone.js';

/** What the instance settings form edits. */
export interface InstanceDraft {
  instanceName: string;
  timeZone: string;
  units: Record<string, string>;
  loginRequired: boolean;
  /** The admin's own password; needed only when this save turns login off. */
  password: string;
}

export function draftFromSettings(settings: InstanceSettings): InstanceDraft {
  return {
    instanceName: settings.instance_name,
    timeZone: settings.timezone,
    units: { ...settings.units },
    loginRequired: settings.auth_mode !== 'disabled',
    password: ''
  };
}

/** True when saving would turn login off, which needs the admin password. */
export function turnsLoginOff(original: InstanceSettings, draft: InstanceDraft): boolean {
  return original.auth_mode !== 'disabled' && !draft.loginRequired;
}

/** True when saving would turn login back on, after which this browser has to sign in. */
export function turnsLoginOn(original: InstanceSettings, draft: InstanceDraft): boolean {
  return original.auth_mode === 'disabled' && draft.loginRequired;
}

/** Only the settings the form changed, so an untouched field is never sent and never overwritten. */
export function settingsPatch(original: InstanceSettings, draft: InstanceDraft): SettingsPatch {
  const patch: SettingsPatch = {};

  const name = draft.instanceName.trim();
  if (name !== original.instance_name) patch.instance_name = name;

  const zone = normalizeTimeZone(draft.timeZone) ?? draft.timeZone.trim();
  if (zone !== original.timezone) patch.timezone = zone;

  const units: Record<string, string> = {};
  for (const [family, unit] of Object.entries(draft.units)) {
    if (original.units[family] !== unit) units[family] = unit;
  }
  if (Object.keys(units).length > 0) patch.units = units;

  if (turnsLoginOff(original, draft)) {
    patch.auth_mode = 'disabled';
    patch.password = draft.password;
  } else if (turnsLoginOn(original, draft)) {
    patch.auth_mode = 'password';
  }
  return patch;
}

export function hasChanges(patch: SettingsPatch): boolean {
  return Object.keys(patch).length > 0;
}
