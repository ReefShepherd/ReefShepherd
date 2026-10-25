import { describe, expect, it } from 'vitest';
import { fieldCodeOf, hasErrors } from './codes.js';

describe('fieldCodeOf', () => {
  it('passes a code through unchanged', () => {
    for (const code of [
      'required',
      'invalid',
      'too_short',
      'too_long',
      'out_of_range',
      'taken',
      'incorrect',
      'locked'
    ]) {
      expect(fieldCodeOf(code)).toBe(code);
    }
  });

  it.each([
    ['required', 'required'],
    ['is required', 'required'],
    ['is required for this card type', 'required'],
    ['must not be blank', 'required'],
    ['must not be null', 'required'],
    ['instance name is blank', 'required'],
    ['send either code or recovery_code', 'required'],
    ['every_n_days needs an anchor date', 'required'],
    ['must list at least one result', 'required'],
    ['must have at least one card', 'required'],
    ['must be at least 8 characters', 'too_short'],
    ['must be at most 100 characters', 'too_long'],
    ['must list at most 50 results', 'too_long'],
    ['must have at most 12 cards', 'too_long'],
    ['must be between 0 and 4', 'out_of_range'],
    ['is outside the possible range', 'out_of_range'],
    ['must be a positive, realistic tank volume', 'out_of_range'],
    ['must be a whole number from 1 to 3650', 'out_of_range'],
    ['must not be above ideal_high', 'out_of_range'],
    ['must not be after to', 'out_of_range'],
    ['already taken', 'taken'],
    ['another parameter already has this name', 'taken'],
    ['incorrect password', 'incorrect'],
    ['that code is not valid', 'incorrect'],
    ["must be the tank's name", 'incorrect'],
    ['set by REEFSHEPHERD_AUTH_MODE on the server', 'locked']
  ])('reads the server sentence %j as %s', (detail, code) => {
    expect(fieldCodeOf(detail)).toBe(code);
  });

  it('reads formats, enums and anything unknown as invalid', () => {
    for (const detail of [
      'must be true or false',
      'unknown field',
      'no such parameter',
      'this account has no password',
      'unknown timezone "x"',
      ''
    ]) {
      expect(fieldCodeOf(detail)).toBe('invalid');
    }
  });
});

describe('hasErrors', () => {
  it('is true only when a field is flagged', () => {
    expect(hasErrors({})).toBe(false);
    expect(hasErrors({ name: 'required' })).toBe(true);
  });
});
