import { describe, expect, it } from 'vitest';
import type { InstanceSettings } from '#lib/api/settings.js';
import { hasChanges } from '#lib/settings/diff.js';
import { backupPatch, draftErrors, draftFromSettings, keepText } from './draft.js';

const original: InstanceSettings = {
  instance_name: 'Garage reef',
  timezone: 'Europe/Berlin',
  reminder_default_time: '09:00',
  backup_enabled: true,
  backup_time: '03:30',
  backup_keep: 14,
  units: {},
  auth_mode: 'password',
  env_locked: {}
};

describe('draftFromSettings', () => {
  it('reads the three backup settings', () => {
    expect(draftFromSettings(original)).toEqual({ enabled: true, time: '03:30', keep: '14' });
  });
});

describe('backupPatch', () => {
  it('is empty for an untouched draft', () => {
    const patch = backupPatch(original, draftFromSettings(original));
    expect(patch).toEqual({});
    expect(hasChanges(patch)).toBe(false);
  });

  it('sends only the fields that changed', () => {
    const draft = draftFromSettings(original);
    draft.enabled = false;
    expect(backupPatch(original, draft)).toEqual({ backup_enabled: false });
    draft.time = ' 04:15 ';
    draft.keep = ' 30 ';
    expect(backupPatch(original, draft)).toEqual({
      backup_enabled: false,
      backup_time: '04:15',
      backup_keep: 30
    });
  });

  it('sends the keep count as a number', () => {
    const draft = { ...draftFromSettings(original), keep: '7' };
    expect(backupPatch(original, draft).backup_keep).toBe(7);
  });
});

describe('a keep count an input hands back as a number or null', () => {
  // A number input bound with bind:value gives a number, or null when it is empty.
  const asDraft = (keep: unknown) => ({ ...draftFromSettings(original), keep }) as never;

  it('reads a numeric keep count like its text', () => {
    expect(draftErrors(asDraft(14))).toEqual({});
    expect(backupPatch(original, asDraft(14))).toEqual({});
    expect(backupPatch(original, asDraft(30))).toEqual({ backup_keep: 30 });
    expect(draftErrors(asDraft(0))).toEqual({ backup_keep: 'out_of_range' });
    expect(draftErrors(asDraft(2.5))).toEqual({ backup_keep: 'invalid' });
  });

  it('treats null and undefined as an empty keep count without throwing', () => {
    expect(draftErrors(asDraft(null))).toEqual({ backup_keep: 'required' });
    expect(draftErrors(asDraft(undefined))).toEqual({ backup_keep: 'required' });
    expect(backupPatch(original, asDraft(null))).toEqual({});
  });

  it('treats an empty string as missing and never sends it', () => {
    expect(draftErrors(asDraft(''))).toEqual({ backup_keep: 'required' });
    expect(backupPatch(original, asDraft(''))).toEqual({});
  });
});

describe('keepText', () => {
  it('turns every shape of keep count into trimmed text', () => {
    expect(keepText(14)).toBe('14');
    expect(keepText(' 7 ')).toBe('7');
    expect(keepText(null)).toBe('');
    expect(keepText(undefined)).toBe('');
  });
});

describe('draftErrors', () => {
  const ok = draftFromSettings(original);

  it('accepts the loaded settings', () => {
    expect(draftErrors(ok)).toEqual({});
  });

  it('flags a time that is not HH:MM', () => {
    expect(draftErrors({ ...ok, time: '' })).toEqual({ backup_time: 'required' });
    expect(draftErrors({ ...ok, time: '25:00' })).toEqual({ backup_time: 'invalid' });
    expect(draftErrors({ ...ok, time: '3:30' })).toEqual({ backup_time: 'invalid' });
  });

  it('holds the keep count to 1 through 365', () => {
    expect(draftErrors({ ...ok, keep: '1' })).toEqual({});
    expect(draftErrors({ ...ok, keep: '365' })).toEqual({});
    expect(draftErrors({ ...ok, keep: '0' })).toEqual({ backup_keep: 'out_of_range' });
    expect(draftErrors({ ...ok, keep: '366' })).toEqual({ backup_keep: 'out_of_range' });
    expect(draftErrors({ ...ok, keep: '' })).toEqual({ backup_keep: 'required' });
    expect(draftErrors({ ...ok, keep: '2.5' })).toEqual({ backup_keep: 'invalid' });
    expect(draftErrors({ ...ok, keep: 'many' })).toEqual({ backup_keep: 'invalid' });
  });
});
