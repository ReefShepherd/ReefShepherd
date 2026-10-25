import { isHttpError } from '@sveltejs/kit';
import { describe, expect, it } from 'vitest';
import { parseTankId, tankIdFromParams, tankPath } from './tank-id.js';

describe('parseTankId', () => {
  it('reads a positive whole number', () => {
    expect(parseTankId('1')).toBe(1);
    expect(parseTankId('42')).toBe(42);
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
      expect(parseTankId(value), String(value)).toBeNull();
    }
  });
});

describe('tankIdFromParams', () => {
  it('returns the id as a number', () => {
    expect(tankIdFromParams({ id: '12' })).toBe(12);
  });

  it('throws a 404 for a missing or malformed id', () => {
    for (const params of [{}, { id: 'abc' }, { id: '0' }, { id: '2.5' }]) {
      let thrown: unknown;
      try {
        tankIdFromParams(params);
      } catch (e) {
        thrown = e;
      }
      expect(isHttpError(thrown, 404), JSON.stringify(params)).toBe(true);
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
