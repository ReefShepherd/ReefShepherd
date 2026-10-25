import { usersApi, type Me } from '#lib/api/users.js';
import { browserTimeZone, normalizeTimeZone } from '#lib/format/timezone.js';
import { isThemeName, theme, type ThemeName } from '#lib/theme/theme.svelte.js';

let current = $state<Me | null>(null);
let reading: Promise<Me> | null = null;

// An instance zone the browser does not know falls through to the browser's own zone, then UTC.
const timeZone = $derived(
  normalizeTimeZone(current?.instance.timezone ?? '') ?? browserTimeZone() ?? 'UTC'
);

function adopt(me: Me): void {
  current = me;
  if (isThemeName(me.theme)) theme.set(me.theme);
}

async function load(): Promise<Me> {
  const me = await usersApi.me();
  adopt(me);
  return me;
}

/** The signed-in user and the instance they are signed in to, as the server last reported them. */
export const session = {
  get me(): Me | null {
    return current;
  },
  get isAdmin(): boolean {
    return current?.role === 'admin';
  },
  /** The IANA timezone every date is shown in: the instance's, else the browser's, else UTC. */
  get timeZone(): string {
    return timeZone;
  },
  /** Reads /me, applies the saved theme, and returns the result. */
  load,
  /** The user as last known, or the read already under way, or a fresh read when nothing is known yet. */
  ready(): Promise<Me> {
    if (current) return Promise.resolve(current);
    if (!reading) {
      const request = load().finally(() => {
        if (reading === request) reading = null;
      });
      reading = request;
    }
    return reading;
  },
  /** Replaces the cached user with a fresher copy from the server. */
  adopt,
  clear(): void {
    current = null;
  },
  /** Applies a theme at once and saves it to the account; if saving fails the theme stays applied on this device and the error is thrown. */
  async chooseTheme(name: ThemeName): Promise<void> {
    theme.set(name);
    const me = await usersApi.updateMe({ theme: name });
    current = me;
  }
};
