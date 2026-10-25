import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';
import {
  extractThemeInitScript,
  themeInitMarker,
  themeNames,
  themeStorageKey
} from '../src/lib/theme/constants.js';

const html = readFileSync(fileURLToPath(new URL('../src/app.html', import.meta.url)), 'utf8');
const script = extractThemeInitScript(html) ?? '';

describe('the no-flash theme script in app.html', () => {
  it('carries the marker the build uses to hash it', () => {
    expect(html).toContain(`<script ${themeInitMarker}>`);
    expect(script).not.toBe('');
  });

  it('reads the key the theme store writes', () => {
    expect(script).toContain(`localStorage.getItem('${themeStorageKey}')`);
  });

  it('accepts exactly the themes the store knows', () => {
    const accepted = [...script.matchAll(/t === '([^']+)'/g)].map((match) => match[1]);
    expect(accepted.sort()).toEqual([...themeNames].sort());
  });
});
