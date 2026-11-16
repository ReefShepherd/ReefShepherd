import { describe, expect, it } from 'vitest';
import { preset } from '#lib/test/maintenance-fixtures.js';
import { isOwnPreset, replacePreset } from './presets.js';

describe('replacePreset', () => {
  const a = preset({ id: 1, name: 'A', sort_order: 10 });
  const b = preset({ id: 2, name: 'B', sort_order: 20 });

  it('replaces a template in place', () => {
    const shown = replacePreset([a, b], { ...b, archived_at: '2026-11-01T00:00:00Z' });
    expect(shown.map((p) => p.id)).toEqual([1, 2]);
    expect(shown[1].archived_at).not.toBeNull();
  });

  it('adds a new template where its sort order puts it', () => {
    const own = preset({ id: 3, name: 'Own', preset_key: null, sort_order: 15 });
    expect(replacePreset([a, b], own).map((p) => p.id)).toEqual([1, 3, 2]);
  });
});

describe('isOwnPreset', () => {
  it('is true for templates without a key', () => {
    expect(isOwnPreset(preset({ preset_key: null }))).toBe(true);
    expect(isOwnPreset(preset({ preset_key: 'water_change' }))).toBe(false);
  });
});
