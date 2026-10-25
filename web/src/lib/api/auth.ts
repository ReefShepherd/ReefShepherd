import { api as defaultApi, type Api } from './client.js';

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

export function createAuthApi(api: Api) {
  return {
    config: (signal?: AbortSignal) => api.get<AuthConfig>('/auth/config', { signal }),
    login: (username: string, password: string, signal?: AbortSignal) =>
      api.post<{ mfa_required: boolean }>('/auth/login', { username, password }, { signal }),
    verifyCode: (code: string, signal?: AbortSignal) =>
      api.post<void>('/auth/mfa', { code }, { signal }),
    verifyRecoveryCode: (recoveryCode: string, signal?: AbortSignal) =>
      api.post<void>('/auth/mfa', { recovery_code: recoveryCode }, { signal }),
    logout: (signal?: AbortSignal) => api.post<void>('/auth/logout', undefined, { signal }),

    /** currentPassword is left out by an account that has no password yet. */
    changePassword: (newPassword: string, currentPassword?: string, signal?: AbortSignal) =>
      api.post<void>(
        '/me/password',
        { current_password: currentPassword, new_password: newPassword },
        { signal }
      ),
    beginTotp: (signal?: AbortSignal) =>
      api.post<TotpEnrollment>('/me/totp/enroll', undefined, { signal }),
    confirmTotp: (code: string, signal?: AbortSignal) =>
      api.post<{ recovery_codes: string[] }>('/me/totp/confirm', { code }, { signal }),
    /** Cancelling an enrolment that was never confirmed needs no proof. */
    disableTotp: (proof?: TotpProof, signal?: AbortSignal) =>
      api.del<void>('/me/totp', proof, { signal }),

    sessions: (signal?: AbortSignal) => api.get<Session[]>('/me/sessions', { signal }),
    revokeSession: (id: number, signal?: AbortSignal) =>
      api.del<void>(`/me/sessions/${id}`, undefined, { signal })
  };
}

export const authApi = createAuthApi(defaultApi);
