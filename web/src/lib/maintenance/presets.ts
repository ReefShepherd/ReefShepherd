import type { Preset } from '#lib/api/maintenance.js';

/** The order the server lists templates in: by sort order, then name, then id. */
function byOrder(a: Preset, b: Preset): number {
  return a.sort_order - b.sort_order || a.name.localeCompare(b.name) || a.id - b.id;
}

/** Puts a changed template in place of its old copy, or adds it when it is new, keeping the listing order. */
export function replacePreset(presets: readonly Preset[], updated: Preset): Preset[] {
  const next = presets.some((p) => p.id === updated.id)
    ? presets.map((p) => (p.id === updated.id ? updated : p))
    : [...presets, updated];
  return next.sort(byOrder);
}

/** Only the templates an admin made can be edited; the shipped ones can only be hidden or shown. */
export function isOwnPreset(preset: Pick<Preset, 'preset_key'>): boolean {
  return preset.preset_key === null;
}
