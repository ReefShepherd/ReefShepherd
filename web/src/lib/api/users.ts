import { api } from './client.js';

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

export const usersApi = {
  setup: (request: SetupRequest) => api.post<Me>('/setup', request),
  me: () => api.get<Me>('/me'),
  updateMe: (changes: { theme?: string; display_name?: string; email?: string | null }) =>
    api.patch<Me>('/me', changes),
  list: () => api.get<User[]>('/users'),
  create: (user: NewUser) => api.post<User>('/users', user),
  update: (id: number, changes: UserChanges) => api.patch<User>(`/users/${id}`, changes),
  remove: (id: number) => api.del<void>(`/users/${id}`),
  resetMfa: (id: number) => api.post<void>(`/users/${id}/reset-mfa`)
};
