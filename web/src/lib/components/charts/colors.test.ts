import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { colorTokens, resolveColors } from './colors.js';

const css = readFileSync(new URL('../../../app.css', import.meta.url), 'utf8');

// The first :root block holds the light and dark values through light-dark(), which the OLED theme also inherits.
const rootBlock = /:root\s*\{([^}]*)\}/.exec(css)?.[1] ?? '';

describe('resolveColors', () => {
  it('resolves every token through the resolver', () => {
    const seen: string[] = [];
    const colors = resolveColors((expression) => {
      seen.push(expression);
      return `rgb(${seen.length}, 0, 0)`;
    }, 'Inter, sans-serif');
    expect(seen.sort()).toEqual(Object.values(colorTokens).sort());
    expect(colors.line).toBe(`rgb(${seen.indexOf(colorTokens.line) + 1}, 0, 0)`);
    expect(colors.font).toBe('Inter, sans-serif');
  });

  it('gives each theme its own colors by asking the resolver again', () => {
    const themes: Record<string, Record<string, string>> = {
      light: { 'var(--accent)': 'rgb(10, 110, 145)', 'var(--surface)': 'rgb(255, 255, 255)' },
      oled: { 'var(--accent)': 'rgb(58, 155, 189)', 'var(--surface)': 'rgb(11, 15, 19)' }
    };
    const read = (theme: string) => resolveColors((e) => themes[theme][e] ?? 'rgb(1, 2, 3)', '');
    expect(read('light').line).toBe('rgb(10, 110, 145)');
    expect(read('oled').line).toBe('rgb(58, 155, 189)');
    expect(read('oled').surface).toBe('rgb(11, 15, 19)');
  });

  it('falls back to fixed colors when a token resolves to nothing', () => {
    const colors = resolveColors(() => '   ', '');
    for (const key of Object.keys(colorTokens) as (keyof typeof colorTokens)[]) {
      expect(colors[key], key).toMatch(/^#[0-9a-f]{6}$/);
    }
    expect(colors.font).toBe('system-ui, sans-serif');
  });
});

describe('theme tokens', () => {
  it('defines every token the charts read in the root theme block', () => {
    const wanted = Object.values(colorTokens).map((e) => /var\((--[a-z-]+)\)/.exec(e)?.[1]);
    expect(wanted.length).toBeGreaterThan(0);
    for (const name of wanted) {
      expect(rootBlock, `${name} missing from :root`).toMatch(new RegExp(`${name}\\s*:`));
    }
  });
});
