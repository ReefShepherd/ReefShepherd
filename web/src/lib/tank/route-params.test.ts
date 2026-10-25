import { isHttpError } from '@sveltejs/kit';
import { describe, expect, it } from 'vitest';
import { parameterIdFromParams, tankIdFromParams } from './route-params.js';

function thrown(run: () => unknown): unknown {
  try {
    run();
  } catch (e) {
    return e;
  }
  return undefined;
}

describe('tankIdFromParams', () => {
  it('returns the id as a number', () => {
    expect(tankIdFromParams({ id: '12' })).toBe(12);
  });

  it('throws a 404 for a missing or malformed id', () => {
    for (const params of [{}, { id: 'abc' }, { id: '0' }, { id: '2.5' }]) {
      expect(
        isHttpError(
          thrown(() => tankIdFromParams(params)),
          404
        ),
        JSON.stringify(params)
      ).toBe(true);
    }
  });
});

describe('parameterIdFromParams', () => {
  it('returns the id as a number', () => {
    expect(parameterIdFromParams({ pid: '7' })).toBe(7);
  });

  it('throws a 404 for a missing or malformed id', () => {
    for (const params of [{}, { pid: 'ph' }, { pid: '0' }, { pid: '1/2' }, { pid: '01' }]) {
      expect(
        isHttpError(
          thrown(() => parameterIdFromParams(params)),
          404
        ),
        JSON.stringify(params)
      ).toBe(true);
    }
  });
});
