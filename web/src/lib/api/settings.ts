import { api as defaultApi, type Api } from './client.js';

export interface InstanceSettings {
  instance_name: string;
  timezone: string;
  reminder_default_time: string;
  units: Record<string, string>;
  /** The login mode in force, which a server environment variable may decide. */
  auth_mode: string;
  /** Maps each setting the server environment decides to the variable that does. */
  env_locked: Record<string, string>;
}

export interface UnitOption {
  unit: string;
  label: string;
  decimals: number;
}

export interface UnitFamily {
  family: string;
  canonical: string;
  units: UnitOption[];
}

export interface SettingsPatch {
  instance_name?: string;
  timezone?: string;
  units?: Record<string, string>;
  auth_mode?: string;
  /** The admin's own password, needed to turn login off. */
  password?: string;
}

export function createSettingsApi(api: Api) {
  return {
    get: (signal?: AbortSignal) => api.get<InstanceSettings>('/settings', { signal }),
    update: (patch: SettingsPatch, signal?: AbortSignal) =>
      api.patch<InstanceSettings>('/settings', patch, { signal }),
    units: (signal?: AbortSignal) => api.get<UnitFamily[]>('/units', { signal })
  };
}

export const settingsApi = createSettingsApi(defaultApi);
