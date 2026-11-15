export const themeNames = ['light', 'dark', 'oled', 'system'] as const;

/** localStorage key shared by the theme store and the no-flash script in app.html. */
export const themeStorageKey = 'theme';

/** Attribute that marks the no-flash script in app.html so the build can hash it for the CSP. */
export const themeInitMarker = 'data-theme-init';

/** Returns the source of the marked inline script in app.html, or undefined when it is missing. */
export function extractThemeInitScript(html: string): string | undefined {
  return new RegExp(`<script ${themeInitMarker}>([\\s\\S]*?)</script>`).exec(html)?.[1];
}
