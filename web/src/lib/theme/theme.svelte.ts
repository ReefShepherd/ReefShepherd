import { themeNames, themeStorageKey } from './constants.js';

export type ThemeName = (typeof themeNames)[number];

export function isThemeName(value: unknown): value is ThemeName {
  return typeof value === 'string' && (themeNames as readonly string[]).includes(value);
}

let current = $state<ThemeName>('system');

if (typeof document !== 'undefined') {
  const initial = document.documentElement.dataset.theme;
  if (isThemeName(initial)) current = initial;
}

export const theme = {
  get current(): ThemeName {
    return current;
  },
  set(name: ThemeName): void {
    current = name;
    document.documentElement.dataset.theme = name;
    try {
      localStorage.setItem(themeStorageKey, name);
    } catch {
      /* storage unavailable */
    }
  }
};
