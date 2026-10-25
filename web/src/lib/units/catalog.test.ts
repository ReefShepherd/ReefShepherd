import { describe, expect, it } from 'vitest';
import type { UnitFamily } from '#lib/api/settings.js';
import { defaultUnits, selectableFamilies } from './catalog.js';

/** The shape GET /api/v1/units answers with, cut down to what the defaults depend on. */
const catalog: UnitFamily[] = [
  {
    family: 'temperature',
    canonical: 'c',
    units: [
      { unit: 'c', label: '°C', decimals: 1 },
      { unit: 'f', label: '°F', decimals: 1 }
    ]
  },
  {
    family: 'salinity',
    canonical: 'ppt',
    units: [
      { unit: 'ppt', label: 'ppt', decimals: 1 },
      { unit: 'sg', label: 'SG', decimals: 3 }
    ]
  },
  {
    family: 'volume',
    canonical: 'l',
    units: [
      { unit: 'l', label: 'L', decimals: 1 },
      { unit: 'us_gal', label: 'US gal', decimals: 1 }
    ]
  },
  { family: 'fixed', canonical: '', units: [{ unit: '', label: '', decimals: 2 }] }
];

describe('defaultUnits', () => {
  it('starts every family on its canonical unit and salinity on specific gravity', () => {
    expect(defaultUnits(catalog)).toEqual({ temperature: 'c', salinity: 'sg', volume: 'l' });
  });

  it('keeps the canonical salinity unit when the server offers no specific gravity', () => {
    const noSg = catalog.map((f) =>
      f.family === 'salinity' ? { ...f, units: f.units.filter((u) => u.unit !== 'sg') } : f
    );
    expect(defaultUnits(noSg)).toEqual({ temperature: 'c', volume: 'l' });
  });

  it('defaults every selectable family to a unit the catalog offers', () => {
    for (const family of selectableFamilies(catalog)) {
      expect(
        family.units.map((u) => u.unit),
        family.family
      ).toContain(defaultUnits(catalog)[family.family]);
    }
  });

  it('knows nothing before the catalog has loaded', () => {
    expect(defaultUnits([])).toEqual({});
  });
});

describe('selectableFamilies', () => {
  it('leaves out the fixed family and families with nothing to choose', () => {
    expect(selectableFamilies(catalog).map((f) => f.family)).toEqual([
      'temperature',
      'salinity',
      'volume'
    ]);
  });
});
