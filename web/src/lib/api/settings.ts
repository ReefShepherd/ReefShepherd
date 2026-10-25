import { api } from './client.js';

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

export const settingsApi = {
  get: () => api.get<InstanceSettings>('/settings'),
  update: (patch: SettingsPatch) => api.patch<InstanceSettings>('/settings', patch),
  units: () => api.get<UnitFamily[]>('/units')
};
