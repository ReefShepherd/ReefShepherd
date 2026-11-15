import { describe, expect, it } from 'vitest';
import { parseId, tankPath } from './tank-id.js';

describe('parseId', () => {
  it('reads a positive whole number', () => {
    expect(parseId('1')).toBe(1);
    expect(parseId('42')).toBe(42);
  });

  it('rejects anything else', () => {
    for (const value of [
      null,
      undefined,
      7,
      '',
      '0',
      '-1',
      '01',
      '1.5',
      '1e3',
      'abc',
      '3/tests',
      ' 3',
      '99999999999999999999'
    ]) {
      expect(parseId(value), String(value)).toBeNull();
    }
  });
});

describe('tankPath', () => {
  it('joins the id and the segments under /tanks', () => {
    expect(tankPath(3)).toBe('/tanks/3');
    expect(tankPath(3, 'parameters', 12, 'series')).toBe('/tanks/3/parameters/12/series');
  });

  it('escapes every segment', () => {
    expect(tankPath(3, 'a/b')).toBe('/tanks/3/a%2Fb');
  });
});
