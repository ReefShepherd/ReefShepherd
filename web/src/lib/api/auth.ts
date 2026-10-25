import { api } from './client.js';

/** What the sign-in page needs to know before anyone is signed in. */
export interface AuthConfig {
  /** The login mode in force: "password" or "disabled". */
  mode: string;
  setup_required: boolean;
  methods: string[];
  instance_name: string;
  /** The server's release version, "dev" for an unversioned build. */
  version: string;
  /** True when the server environment decides the login mode, so the setting cannot change it. */
  mode_locked: boolean;
}

export interface Session {
  id: number;
  auth_method: string;
  user_agent: string;
  ip_address: string;
  created_at: string;
  last_seen_at: string;
  expires_at: string;
  current: boolean;
}

export interface TotpEnrollment {
  secret: string;
  otpauth_url: string;
  /** A PNG as a data: URL. */
  qr_code: string;
}

/** Proof for turning two-step login off: the password, or an authenticator code for an account without one. */
export interface TotpProof {
  password?: string;
  code?: string;
}

export const authApi = {
  config: () => api.get<AuthConfig>('/auth/config'),
  login: (username: string, password: string) =>
    api.post<{ mfa_required: boolean }>('/auth/login', { username, password }),
  verifyCode: (code: string) => api.post<void>('/auth/mfa', { code }),
  verifyRecoveryCode: (recoveryCode: string) =>
    api.post<void>('/auth/mfa', { recovery_code: recoveryCode }),
  logout: () => api.post<void>('/auth/logout'),

  /** currentPassword is left out by an account that has no password yet. */
  changePassword: (newPassword: string, currentPassword?: string) =>
    api.post<void>('/me/password', {
      current_password: currentPassword,
      new_password: newPassword
    }),
  beginTotp: () => api.post<TotpEnrollment>('/me/totp/enroll'),
  confirmTotp: (code: string) =>
    api.post<{ recovery_codes: string[] }>('/me/totp/confirm', { code }),
  /** Cancelling an enrolment that was never confirmed needs no proof. */
  disableTotp: (proof?: TotpProof) => api.del<void>('/me/totp', proof),

  sessions: () => api.get<Session[]>('/me/sessions'),
  revokeSession: (id: number) => api.del<void>(`/me/sessions/${id}`)
};
