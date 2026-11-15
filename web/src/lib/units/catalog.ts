import type { UnitFamily } from '#lib/api/settings.js';
import { m } from '#lib/paraglide/messages.js';

/** What a fresh instance starts with, as the server does: each family's canonical unit, except salinity in specific gravity, which is what hobbyists read on a refractometer. */
export function defaultUnits(catalog: readonly UnitFamily[]): Record<string, string> {
  const out: Record<string, string> = {};
  for (const family of selectableFamilies(catalog)) {
    const salinityInSg = family.family === 'salinity' && family.units.some((u) => u.unit === 'sg');
    out[family.family] = salinityInSg ? 'sg' : family.canonical;
  }
  return out;
}

/** The families a person can choose a unit for; a family with a single unit, such as the fixed one, has nothing to choose. */
export function selectableFamilies(catalog: readonly UnitFamily[]): UnitFamily[] {
  return catalog.filter((f) => f.units.length > 1);
}

export function familyLabel(family: string): string {
  switch (family) {
    case 'temperature':
      return m.units_family_temperature();
    case 'salinity':
      return m.units_family_salinity();
    case 'alkalinity':
      return m.units_family_alkalinity();
    case 'phosphate':
      return m.units_family_phosphate();
    case 'volume':
      return m.units_family_volume();
    default:
      return family;
  }
}
