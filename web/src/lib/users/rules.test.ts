import { describe, expect, it } from 'vitest';
import type { User } from '#lib/api/users.js';
import { draftFromUser, isLastEnabledAdmin, userChanges, userLabel } from './rules.js';

function user(id: number, changes: Partial<User> = {}): User {
  return {
    id,
    username: `user${id}`,
    display_name: '',
    email: null,
    role: 'member',
    disabled: false,
    has_password: true,
    totp_enabled: false,
    created_at: '2026-10-24T00:00:00Z',
    ...changes
  };
}

describe('isLastEnabledAdmin', () => {
  it('is true for the only enabled admin', () => {
    const users = [user(1, { role: 'admin' }), user(2)];
    expect(isLastEnabledAdmin(users, 1)).toBe(true);
    expect(isLastEnabledAdmin(users, 2)).toBe(false);
  });

  it('is false when another enabled admin exists', () => {
    const users = [user(1, { role: 'admin' }), user(2, { role: 'admin' })];
    expect(isLastEnabledAdmin(users, 1)).toBe(false);
  });

  it('does not count a disabled admin as a second one', () => {
    const users = [user(1, { role: 'admin' }), user(2, { role: 'admin', disabled: true })];
    expect(isLastEnabledAdmin(users, 1)).toBe(true);
    expect(isLastEnabledAdmin(users, 2)).toBe(false);
  });
});

describe('userLabel', () => {
  it('prefers the display name and falls back to the username', () => {
    expect(userLabel({ username: 'jay', display_name: 'Jayce' })).toBe('Jayce');
    expect(userLabel({ username: 'jay', display_name: '  ' })).toBe('jay');
  });
});

describe('userChanges', () => {
  const base = user(5, { display_name: 'Sam', email: 'sam@example.com' });

  it('is empty for an untouched draft', () => {
    expect(userChanges(base, draftFromUser(base))).toEqual({});
  });

  it('contains only what changed', () => {
    const draft = draftFromUser(base);
    draft.role = 'admin';
    draft.disabled = true;
    expect(userChanges(base, draft)).toEqual({ role: 'admin', disabled: true });
  });

  it('trims text and treats a blank email as clearing it', () => {
    const draft = draftFromUser(base);
    draft.displayName = '  Sam  ';
    draft.email = '   ';
    expect(userChanges(base, draft)).toEqual({ email: null });
  });

  it('does not clear an email that was already empty', () => {
    const none = user(6);
    expect(userChanges(none, draftFromUser(none))).toEqual({});
  });

  it('sends a password only when one was typed', () => {
    const draft = draftFromUser(base);
    expect(userChanges(base, draft)).not.toHaveProperty('password');
    draft.password = 'a new long password';
    expect(userChanges(base, draft)).toEqual({ password: 'a new long password' });
  });
});
