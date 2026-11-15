import { cachedResource } from '#lib/cache/cached-resource.svelte.js';
import { usersApi, type Me } from '#lib/api/users.js';
import { browserTimeZone, normalizeTimeZone } from '#lib/format/timezone.js';
import { isThemeName, theme, type ThemeName } from '#lib/theme/theme.svelte.js';

function applyTheme(me: Me): void {
  if (isThemeName(me.theme)) theme.set(me.theme);
}

const user = cachedResource(async (signal) => {
  const me = await usersApi.me(signal);
  signal.throwIfAborted();
  applyTheme(me);
  return me;
});

// An instance zone the browser does not know falls through to the browser's own zone, then UTC.
const timeZone = $derived(
  normalizeTimeZone(user.value?.instance.timezone ?? '') ?? browserTimeZone() ?? 'UTC'
);

/** The signed-in user and the instance they are signed in to, as the server last reported them. */
export const session = {
  get me(): Me | null {
    return user.value;
  },
  get isAdmin(): boolean {
    return user.value?.role === 'admin';
  },
  /** The IANA timezone every date is shown in: the instance's, else the browser's, else UTC. */
  get timeZone(): string {
    return timeZone;
  },
  /** Reads /me, applies the saved theme, and returns the result. */
  load: user.refresh,
  /** The user as last known, or the read already under way, or a fresh read when nothing is known yet. */
  ready(): Promise<Me> {
    return user.value ? Promise.resolve(user.value) : user.load();
  },
  /** Replaces the cached user with a fresher copy from the server. */
  adopt(me: Me): void {
    user.set(me);
    applyTheme(me);
  },
  clear: user.invalidate,
  /** Applies a theme at once and saves it to the account; if saving fails the theme stays applied on this device and the error is thrown. */
  async chooseTheme(name: ThemeName): Promise<void> {
    theme.set(name);
    user.set(await usersApi.updateMe({ theme: name }));
  }
};
