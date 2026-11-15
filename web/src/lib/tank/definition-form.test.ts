import { describe, expect, it } from 'vitest';
import type { ParameterDefinition } from '#lib/api/params.js';
import {
  definitionChanges,
  definitionProblems,
  draftFromDefinition,
  newParameterBody
} from './definition-form.js';

const saved: ParameterDefinition = {
  id: 11,
  preset_key: null,
  name: 'Iodine',
  unit_family: 'fixed',
  unit: '',
  unit_label: 'ppb',
  decimals: 1,
  default_low: 40,
  default_high: 80,
  default_test_interval_days: 14,
  sort_order: 200,
  archived_at: null
};

describe('draftFromDefinition', () => {
  it('starts a new parameter with two decimals and nothing else', () => {
    expect(draftFromDefinition(undefined)).toEqual({
      name: '',
      unitLabel: '',
      decimals: 2,
      low: '',
      high: '',
      intervalDays: ''
    });
  });

  it("writes limits with the parameter's decimals", () => {
    expect(draftFromDefinition(saved)).toMatchObject({
      low: '40.0',
      high: '80.0',
      intervalDays: '14'
    });
  });
});

describe('definitionProblems', () => {
  const draft = draftFromDefinition(saved);

  it('is clean for a saved parameter', () => {
    expect(definitionProblems(draft)).toEqual({});
  });

  it('checks the name, unit label length, limits and interval', () => {
    expect(definitionProblems({ ...draft, name: ' ' }).name).toBe('required');
    expect(definitionProblems({ ...draft, name: 'x'.repeat(61) }).name).toBe('too_long');
    expect(definitionProblems({ ...draft, unitLabel: 'x'.repeat(17) }).unitLabel).toBe('too_long');
    expect(definitionProblems({ ...draft, low: 'a' }).low).toBe('invalid_number');
    expect(definitionProblems({ ...draft, low: '90' }).range).toBe('low_above_high');
    expect(definitionProblems({ ...draft, intervalDays: '0' }).intervalDays).toBe(
      'invalid_interval'
    );
    expect(definitionProblems({ ...draft, intervalDays: '3651' }).intervalDays).toBe(
      'invalid_interval'
    );
    expect(definitionProblems({ ...draft, intervalDays: '' }).intervalDays).toBeUndefined();
  });
});

describe('newParameterBody', () => {
  it('reads limits with a decimal comma and leaves blanks as null', () => {
    expect(
      newParameterBody({
        name: ' Boron ',
        unitLabel: ' ppm ',
        decimals: 1,
        low: '4,2',
        high: '',
        intervalDays: ''
      })
    ).toEqual({
      name: 'Boron',
      unit_label: 'ppm',
      decimals: 1,
      default_low: 4.2,
      default_high: null,
      default_test_interval_days: null
    });
  });
});

describe('definitionChanges', () => {
  it('is empty for an untouched draft and lists only edits otherwise', () => {
    expect(definitionChanges(saved, draftFromDefinition(saved))).toEqual({});
    expect(
      definitionChanges(saved, {
        ...draftFromDefinition(saved),
        name: 'Iodide',
        high: '',
        intervalDays: '30'
      })
    ).toEqual({ name: 'Iodide', default_high: null, default_test_interval_days: 30 });
  });
});
