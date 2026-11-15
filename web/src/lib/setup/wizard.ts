import { isApiError } from '#lib/api/client.js';
import { fieldText } from '#lib/api/field-errors.js';
import type { SetupRequest } from '#lib/api/users.js';
import {
  collectErrors,
  validateDisplayName,
  validateNewPassword,
  validatePasswordMatch,
  validateRequired,
  validateTimeZone,
  validateUsername
} from '#lib/auth/validation.js';
import { normalizeTimeZone } from '#lib/format/timezone.js';
import type { ThemeName } from '#lib/theme/theme.svelte.js';

export interface SetupDraft {
  instanceName: string;
  timeZone: string;
  theme: ThemeName;
  units: Record<string, string>;
  loginRequired: boolean;
  username: string;
  displayName: string;
  password: string;
  passwordRepeat: string;
}

export const setupSteps = ['instance', 'units', 'access'] as const;
export type SetupStep = (typeof setupSteps)[number];

/** units is the starting choice for each unit family, from defaultUnits once the server's catalog is known. */
export function newSetupDraft(
  instanceName: string,
  timeZone: string,
  units: Record<string, string>
): SetupDraft {
  return {
    instanceName,
    timeZone,
    theme: 'system',
    units: { ...units },
    loginRequired: true,
    username: '',
    displayName: '',
    password: '',
    passwordRepeat: ''
  };
}

/** Whether login starts on, and whether the server's environment fixes that choice. */
export function loginChoice(config: { mode: string; mode_locked: boolean }): {
  loginRequired: boolean;
  locked: boolean;
} {
  return config.mode_locked
    ? { loginRequired: config.mode !== 'disabled', locked: true }
    : { loginRequired: true, locked: false };
}

export type SetupErrors = Partial<Record<keyof SetupDraft, string>>;

/** The problems with the fields shown on a step; an empty object means the step may be left. */
export function validateSetupStep(step: SetupStep, draft: SetupDraft): SetupErrors {
  switch (step) {
    case 'instance':
      return collectErrors({
        instanceName: validateRequired(draft.instanceName),
        timeZone: validateTimeZone(draft.timeZone)
      });
    case 'units':
      return {};
    case 'access':
      if (!draft.loginRequired) return {};
      return collectErrors({
        username: validateUsername(draft.username),
        displayName: validateDisplayName(draft.displayName),
        password: validateNewPassword(draft.password),
        passwordRepeat: validatePasswordMatch(draft.password, draft.passwordRepeat)
      });
  }
}

/** The request that completes setup; without login the server makes a passwordless administrator named admin. */
export function buildSetupRequest(draft: SetupDraft): SetupRequest {
  const request: SetupRequest = {
    instance_name: draft.instanceName.trim(),
    timezone: normalizeTimeZone(draft.timeZone) ?? draft.timeZone.trim(),
    units: { ...draft.units },
    theme: draft.theme,
    auth_mode: draft.loginRequired ? 'password' : 'disabled'
  };
  if (draft.loginRequired) {
    request.username = draft.username.trim();
    request.password = draft.password;
    const displayName = draft.displayName.trim();
    if (displayName !== '') request.display_name = displayName;
  }
  return request;
}

// Server field names mapped to the draft field and the step that shows it.
const serverFields: Record<string, [keyof SetupDraft, SetupStep]> = {
  instance_name: ['instanceName', 'instance'],
  timezone: ['timeZone', 'instance'],
  username: ['username', 'access'],
  display_name: ['displayName', 'access'],
  password: ['password', 'access'],
  auth_mode: ['loginRequired', 'access']
};

/** Turns the fields a failed setup call flagged into form errors, and names the earliest step that shows one. */
export function setupServerErrors(err: unknown): {
  errors: SetupErrors;
  step: SetupStep | undefined;
} {
  const errors: SetupErrors = {};
  let step: SetupStep | undefined;
  const earlier = (candidate: SetupStep) => {
    if (step === undefined || setupSteps.indexOf(candidate) < setupSteps.indexOf(step))
      step = candidate;
  };
  for (const [field, [key, stepOf]] of Object.entries(serverFields)) {
    const text = fieldText(err, field);
    if (text === undefined) continue;
    errors[key] = text;
    earlier(stepOf);
  }
  if (isApiError(err) && Object.keys(err.fields).some((f) => f.startsWith('units.')))
    earlier('units');
  return { errors, step };
}
