import { describe, expect, it } from 'vitest';
import type { UnitFamily } from '#lib/api/settings.js';
import type { Tank } from '#lib/api/tanks.js';
import {
  draftFromTank,
  newTankBody,
  tankChanges,
  validateTank,
  volumeUnitLabel,
  type TankDraft
} from './form.js';

const saved: Tank = {
  id: 1,
  name: 'Display',
  tank_type: 'reef',
  volume: 100,
  volume_unit: 'us_gal',
  started_on: '2025-01-02',
  note: 'main tank',
  sort_order: 0,
  archived_at: null,
  created_at: '',
  updated_at: ''
};

const draft = (patch: Partial<TankDraft> = {}): TankDraft => ({
  ...draftFromTank(saved),
  ...patch
});

describe('draftFromTank', () => {
  it('starts a new tank as an empty reef', () => {
    expect(draftFromTank(undefined)).toEqual({
      name: '',
      tankType: 'reef',
      volume: '',
      startedOn: '',
      note: ''
    });
  });

  it('writes a whole volume without a trailing zero', () => {
    expect(draftFromTank(saved).volume).toBe('100');
    expect(draftFromTank({ ...saved, volume: 37.5 }).volume).toBe('37.5');
  });
});

describe('validateTank', () => {
  it('requires a name of at most 100 characters', () => {
    expect(validateTank(draft({ name: '  ' })).name).toBe('required');
    expect(validateTank(draft({ name: 'x'.repeat(101) })).name).toBe('too_long');
    expect(validateTank(draft({ name: 'x'.repeat(100) })).name).toBeUndefined();
  });

  it('limits the note to 10000 characters', () => {
    expect(validateTank(draft({ note: 'x'.repeat(10000) })).note).toBeUndefined();
    expect(validateTank(draft({ note: 'x'.repeat(10001) })).note).toBe('too_long');
  });

  it('accepts an empty volume, a decimal comma, and refuses zero or text', () => {
    expect(validateTank(draft({ volume: '' })).volume).toBeUndefined();
    expect(validateTank(draft({ volume: '37,5' })).volume).toBeUndefined();
    expect(validateTank(draft({ volume: '0' })).volume).toBe('out_of_range');
    expect(validateTank(draft({ volume: 'big' })).volume).toBe('invalid');
  });
});

describe('newTankBody', () => {
  it('turns the draft into the create request with the chosen presets', () => {
    expect(
      newTankBody(draft({ name: ' Frag ', volume: '12,5', startedOn: '', note: ' n ' }), [
        'ph',
        'calcium'
      ])
    ).toEqual({
      name: 'Frag',
      tank_type: 'reef',
      volume: 12.5,
      started_on: null,
      note: 'n',
      presets: ['ph', 'calcium']
    });
  });

  it('sends a null volume when the field is empty', () => {
    expect(newTankBody(draft({ volume: ' ' }), []).volume).toBeNull();
  });
});

describe('tankChanges', () => {
  it('is empty for an untouched draft', () => {
    expect(tankChanges(saved, draft())).toEqual({});
  });

  it('lists only changed fields, and clears a volume or date that was emptied', () => {
    expect(tankChanges(saved, draft({ name: 'Display 2', tankType: 'frag' }))).toEqual({
      name: 'Display 2',
      tank_type: 'frag'
    });
    expect(tankChanges(saved, draft({ volume: '', startedOn: '' }))).toEqual({
      volume: null,
      started_on: null
    });
  });

  it('does not resend a volume that was only rounded for display', () => {
    const rounded = { ...saved, volume: 37.9 };
    expect(tankChanges(rounded, { ...draftFromTank(rounded), name: 'New' })).toEqual({
      name: 'New'
    });
  });
});

describe('volumeUnitLabel', () => {
  const catalog: UnitFamily[] = [
    {
      family: 'volume',
      canonical: 'l',
      units: [
        { unit: 'l', label: 'L', decimals: 1 },
        { unit: 'us_gal', label: 'gal', decimals: 1 }
      ]
    }
  ];

  it('reads the label from the catalog and falls back to the code', () => {
    expect(volumeUnitLabel(catalog, 'us_gal')).toBe('gal');
    expect(volumeUnitLabel(catalog, 'other')).toBe('other');
    expect(volumeUnitLabel([], 'l')).toBe('l');
  });
});
