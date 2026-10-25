import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const root = fileURLToPath(new URL('..', import.meta.url));
const settings = JSON.parse(readFileSync(join(root, 'project.inlang/settings.json'), 'utf8')) as {
  baseLocale: string;
  locales: string[];
  'plugin.inlang.messageFormat': { pathPattern: string[] };
};
const patterns = settings['plugin.inlang.messageFormat'].pathPattern;

function files(locale: string): string[] {
  return patterns.map((p) => join(root, p.replace('{locale}', locale)));
}

function keys(file: string): string[] {
  const parsed = JSON.parse(readFileSync(file, 'utf8')) as Record<string, unknown>;
  return Object.keys(parsed).filter((k) => k !== '$schema');
}

describe('message files', () => {
  it('lists every file in messages/<locale>/ in pathPattern', () => {
    for (const locale of settings.locales) {
      const dir = join(root, 'messages', locale);
      const onDisk = readdirSync(dir)
        .filter((f) => f.endsWith('.json'))
        .sort();
      const listed = files(locale)
        .map((f) => f.slice(f.lastIndexOf('/') + 1))
        .sort();
      expect(listed, `${locale}: files missing from pathPattern`).toEqual(onDisk);
    }
  });

  it('has every file named in pathPattern for every locale', () => {
    for (const locale of settings.locales) {
      for (const file of files(locale)) expect(existsSync(file), file).toBe(true);
    }
  });

  it('never defines a key twice, because the later file would silently win', () => {
    for (const locale of settings.locales) {
      const seen = new Map<string, string>();
      for (const file of files(locale)) {
        for (const key of keys(file)) {
          expect(seen.get(key), `${key} is in both ${seen.get(key)} and ${file}`).toBeUndefined();
          seen.set(key, file);
        }
      }
    }
  });

  it('gives every locale the same keys as the base locale', () => {
    const all = (locale: string) => files(locale).flatMap(keys).sort();
    const base = all(settings.baseLocale);
    for (const locale of settings.locales) expect(all(locale), locale).toEqual(base);
  });
});
