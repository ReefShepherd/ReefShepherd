import type { Role, User, UserChanges } from '#lib/api/users.js';

/** True when the user is the only enabled administrator, whom the server will not demote, disable or delete. */
export function isLastEnabledAdmin(users: readonly User[], id: number): boolean {
  const enabledAdmins = users.filter((u) => u.role === 'admin' && !u.disabled);
  return enabledAdmins.length === 1 && enabledAdmins[0].id === id;
}

/** The name to show for a user: the display name when there is one, else the username. */
export function userLabel(user: Pick<User, 'username' | 'display_name'>): string {
  return user.display_name.trim() || user.username;
}

/** What the user edit form holds. */
export interface UserDraft {
  displayName: string;
  email: string;
  role: Role;
  disabled: boolean;
  /** A new password; blank leaves the current one. */
  password: string;
}

export function draftFromUser(user: User): UserDraft {
  return {
    displayName: user.display_name,
    email: user.email ?? '',
    role: user.role,
    disabled: user.disabled,
    password: ''
  };
}

/** Only the fields the form changed, so saving never rewrites something the administrator did not touch. */
export function userChanges(user: User, draft: UserDraft): UserChanges {
  const changes: UserChanges = {};
  const displayName = draft.displayName.trim();
  if (displayName !== user.display_name) changes.display_name = displayName;
  const email = draft.email.trim() === '' ? null : draft.email.trim();
  if (email !== user.email) changes.email = email;
  if (draft.role !== user.role) changes.role = draft.role;
  if (draft.disabled !== user.disabled) changes.disabled = draft.disabled;
  if (draft.password !== '') changes.password = draft.password;
  return changes;
}
