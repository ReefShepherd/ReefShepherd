import { describe, expect, it } from 'vitest';
import { ApiError } from '#lib/api/client.js';
import { m } from '#lib/paraglide/messages.js';
import {
  buildSetupRequest,
  loginChoice,
  newSetupDraft,
  setupServerErrors,
  validateSetupStep,
  type SetupDraft
} from './wizard.js';

const startingUnits = {
  temperature: 'c',
  salinity: 'sg',
  alkalinity: 'dkh',
  phosphate: 'ppm',
  volume: 'l'
};

function draft(changes: Partial<SetupDraft> = {}): SetupDraft {
  return {
    ...newSetupDraft('Garage reef', 'Europe/Berlin', startingUnits),
    username: 'jayce',
    password: 'correct horse battery',
    passwordRepeat: 'correct horse battery',
    ...changes
  };
}

describe('newSetupDraft', () => {
  it('starts with login on and the given name, timezone and units', () => {
    const d = newSetupDraft('My reef', 'UTC', startingUnits);
    expect(d.loginRequired).toBe(true);
    expect(d.units).toEqual(startingUnits);
    expect(d.instanceName).toBe('My reef');
    expect(d.timeZone).toBe('UTC');
  });

  it('gives each draft its own units object', () => {
    const a = newSetupDraft('a', 'UTC', startingUnits);
    a.units.temperature = 'f';
    expect(startingUnits.temperature).toBe('c');
    expect(newSetupDraft('b', 'UTC', startingUnits).units.temperature).toBe('c');
  });
});

describe('validateSetupStep', () => {
  it('needs an instance name and a known timezone on the first step', () => {
    expect(validateSetupStep('instance', draft())).toEqual({});
    expect(Object.keys(validateSetupStep('instance', draft({ instanceName: '  ' })))).toEqual([
      'instanceName'
    ]);
    expect(Object.keys(validateSetupStep('instance', draft({ timeZone: 'Mars/Base' })))).toEqual([
      'timeZone'
    ]);
  });

  it('never blocks the units step', () => {
    expect(validateSetupStep('units', draft())).toEqual({});
  });

  it('checks the account only when login is on', () => {
    const bad = draft({ username: '', password: 'short', passwordRepeat: 'other' });
    expect(Object.keys(validateSetupStep('access', bad)).sort()).toEqual([
      'password',
      'passwordRepeat',
      'username'
    ]);
    expect(validateSetupStep('access', { ...bad, loginRequired: false })).toEqual({});
  });

  it('accepts a complete account', () => {
    expect(validateSetupStep('access', draft())).toEqual({});
  });
});

describe('buildSetupRequest', () => {
  it('sends the account when login is on, trimmed', () => {
    const request = buildSetupRequest(
      draft({ instanceName: ' Garage reef ', username: ' jayce ', displayName: ' Jayce ' })
    );
    expect(request).toEqual({
      instance_name: 'Garage reef',
      timezone: 'Europe/Berlin',
      units: startingUnits,
      theme: 'system',
      auth_mode: 'password',
      username: 'jayce',
      display_name: 'Jayce',
      password: 'correct horse battery'
    });
  });

  it('leaves the display name out when blank and never trims the password', () => {
    const request = buildSetupRequest(draft({ password: ' spaced pass phrase ' }));
    expect(request).not.toHaveProperty('display_name');
    expect(request.password).toBe(' spaced pass phrase ');
  });

  it('sends no account details when login is off', () => {
    const request = buildSetupRequest(draft({ loginRequired: false }));
    expect(request.auth_mode).toBe('disabled');
    expect(request).not.toHaveProperty('username');
    expect(request).not.toHaveProperty('password');
    expect(request).not.toHaveProperty('display_name');
  });

  it('sends the chosen units', () => {
    const d = draft();
    d.units.salinity = 'ppt';
    expect(buildSetupRequest(d).units.salinity).toBe('ppt');
  });
});

describe('loginChoice', () => {
  it('offers both choices, login on, when the environment does not decide', () => {
    expect(loginChoice({ mode: 'password', mode_locked: false })).toEqual({
      loginRequired: true,
      locked: false
    });
    expect(loginChoice({ mode: 'disabled', mode_locked: false })).toEqual({
      loginRequired: true,
      locked: false
    });
  });

  it('fixes the choice to what the environment decided', () => {
    expect(loginChoice({ mode: 'password', mode_locked: true })).toEqual({
      loginRequired: true,
      locked: true
    });
    expect(loginChoice({ mode: 'disabled', mode_locked: true })).toEqual({
      loginRequired: false,
      locked: true
    });
  });
});

describe('setupServerErrors', () => {
  const rejected = (fields: Record<string, string>) =>
    new ApiError(422, 'validation', 'invalid setup', fields);

  it('maps server field names to draft fields as translated text', () => {
    const { errors } = setupServerErrors(rejected({ username: 'already taken' }));
    expect(errors).toEqual({ username: m.field_error_taken() });
  });

  it('shows a rejected login mode on the access step', () => {
    const out = setupServerErrors(rejected({ auth_mode: 'must be password or disabled' }));
    expect(out.errors).toEqual({ loginRequired: m.field_error_invalid() });
    expect(out.step).toBe('access');
  });

  it('points at the earliest step that has a flagged field', () => {
    const both = rejected({ password: 'required', timezone: 'unknown timezone "x"' });
    expect(setupServerErrors(both).step).toBe('instance');
    expect(setupServerErrors(rejected({ password: 'required' })).step).toBe('access');
    expect(setupServerErrors(rejected({ 'units.salinity': 'unknown unit' })).step).toBe('units');
  });

  it('reports nothing for errors that flag no field', () => {
    expect(setupServerErrors(new ApiError(409, 'conflict', 'done'))).toEqual({
      errors: {},
      step: undefined
    });
    expect(setupServerErrors(new Error('x'))).toEqual({ errors: {}, step: undefined });
  });
});
