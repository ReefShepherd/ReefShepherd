import { describe, expect, it } from 'vitest';
import type { InstanceSettings } from '#lib/api/settings.js';
import {
  draftFromSettings,
  hasChanges,
  settingsPatch,
  turnsLoginOff,
  turnsLoginOn
} from './diff.js';

const original: InstanceSettings = {
  instance_name: 'Garage reef',
  timezone: 'Europe/Berlin',
  reminder_default_time: '09:00',
  units: { temperature: 'c', salinity: 'sg', alkalinity: 'dkh' },
  auth_mode: 'password',
  env_locked: {}
};

describe('settingsPatch', () => {
  it('is empty for an untouched draft', () => {
    const patch = settingsPatch(original, draftFromSettings(original));
    expect(patch).toEqual({});
    expect(hasChanges(patch)).toBe(false);
  });

  it('contains only the fields that changed', () => {
    const draft = draftFromSettings(original);
    draft.instanceName = 'Office reef';
    draft.units.salinity = 'ppt';
    expect(settingsPatch(original, draft)).toEqual({
      instance_name: 'Office reef',
      units: { salinity: 'ppt' }
    });
  });

  it('ignores surrounding spaces and the spelling of an unchanged timezone', () => {
    const draft = draftFromSettings(original);
    draft.instanceName = '  Garage reef ';
    draft.timeZone = ' Europe/Berlin ';
    expect(settingsPatch(original, draft)).toEqual({});
  });

  it('sends a changed timezone in canonical spelling', () => {
    const draft = draftFromSettings(original);
    draft.timeZone = ' America/New_York ';
    expect(settingsPatch(original, draft)).toEqual({ timezone: 'America/New_York' });
  });

  it('sends the admin password only together with turning login off', () => {
    const draft = draftFromSettings(original);
    draft.password = 'typed but unused';
    expect(settingsPatch(original, draft)).toEqual({});

    draft.loginRequired = false;
    expect(settingsPatch(original, draft)).toEqual({
      auth_mode: 'disabled',
      password: 'typed but unused'
    });
  });

  it('turns login on without a password', () => {
    const off = { ...original, auth_mode: 'disabled' };
    const draft = draftFromSettings(off);
    draft.loginRequired = true;
    draft.password = 'leftover';
    expect(settingsPatch(off, draft)).toEqual({ auth_mode: 'password' });
  });

  it('does not touch the login mode when the form leaves it alone', () => {
    const off = { ...original, auth_mode: 'disabled' };
    const draft = draftFromSettings(off);
    draft.instanceName = 'Other';
    expect(settingsPatch(off, draft)).toEqual({ instance_name: 'Other' });
  });
});

describe('login mode transitions', () => {
  it('detects turning login off and on', () => {
    const draft = draftFromSettings(original);
    expect(turnsLoginOff(original, draft)).toBe(false);
    expect(turnsLoginOn(original, draft)).toBe(false);
    draft.loginRequired = false;
    expect(turnsLoginOff(original, draft)).toBe(true);

    const off = { ...original, auth_mode: 'disabled' };
    const back = draftFromSettings(off);
    back.loginRequired = true;
    expect(turnsLoginOn(off, back)).toBe(true);
    expect(turnsLoginOff(off, back)).toBe(false);
  });
});
