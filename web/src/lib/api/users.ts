import { api as defaultApi, type Api } from './client.js';

export type Role = 'admin' | 'member';

export interface User {
  id: number;
  username: string;
  display_name: string;
  email: string | null;
  role: Role;
  disabled: boolean;
  has_password: boolean;
  totp_enabled: boolean;
  created_at: string;
}

export interface InstanceInfo {
  name: string;
  timezone: string;
  /** The display unit chosen for each unit family. */
  units: Record<string, string>;
  /** The login mode in force: "password" or "disabled". */
  auth_mode: string;
}

export interface Me extends User {
  theme: string;
  locale: string;
  default_tank_id: number | null;
  instance: InstanceInfo;
}

export interface SetupRequest {
  instance_name: string;
  timezone: string;
  units: Record<string, string>;
  theme: string;
  auth_mode: 'password' | 'disabled';
  username?: string;
  display_name?: string;
  password?: string;
}

export interface NewUser {
  username: string;
  display_name: string;
  email: string | null;
  role: Role;
  password: string;
}

/** Fields of an existing user an administrator may change; omitted fields stay as they are. */
export interface UserChanges {
  display_name?: string;
  email?: string | null;
  role?: Role;
  disabled?: boolean;
  password?: string;
}

export function createUsersApi(api: Api) {
  return {
    setup: (request: SetupRequest, signal?: AbortSignal) =>
      api.post<Me>('/setup', request, { signal }),
    me: (signal?: AbortSignal) => api.get<Me>('/me', { signal }),
    updateMe: (
      changes: { theme?: string; display_name?: string; email?: string | null },
      signal?: AbortSignal
    ) => api.patch<Me>('/me', changes, { signal }),
    list: (signal?: AbortSignal) => api.get<User[]>('/users', { signal }),
    create: (user: NewUser, signal?: AbortSignal) => api.post<User>('/users', user, { signal }),
    update: (id: number, changes: UserChanges, signal?: AbortSignal) =>
      api.patch<User>(`/users/${id}`, changes, { signal }),
    remove: (id: number, signal?: AbortSignal) =>
      api.del<void>(`/users/${id}`, undefined, { signal }),
    resetMfa: (id: number, signal?: AbortSignal) =>
      api.post<void>(`/users/${id}/reset-mfa`, undefined, { signal })
  };
}

export const usersApi = createUsersApi(defaultApi);
