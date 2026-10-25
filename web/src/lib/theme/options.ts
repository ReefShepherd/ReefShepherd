import { m } from '#lib/paraglide/messages.js';
import type { ThemeName } from './theme.svelte.js';

/** The themes in the order pickers list them. */
export const themeOptions: { name: ThemeName; label: () => string }[] = [
  { name: 'light', label: m.theme_light },
  { name: 'dark', label: m.theme_dark },
  { name: 'oled', label: m.theme_oled },
  { name: 'system', label: m.theme_system }
];
