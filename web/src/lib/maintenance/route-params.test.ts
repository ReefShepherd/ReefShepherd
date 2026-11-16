import { isHttpError } from '@sveltejs/kit';
import { describe, expect, it } from 'vitest';
import { taskIdFromParams } from './route-params.js';

function thrown(run: () => unknown): unknown {
  try {
    run();
  } catch (e) {
    return e;
  }
  return undefined;
}

describe('taskIdFromParams', () => {
  it('returns the id as a number', () => {
    expect(taskIdFromParams({ taskId: '12' })).toBe(12);
  });

  it('throws a 404 for a missing or malformed id', () => {
    for (const params of [{}, { taskId: 'abc' }, { taskId: '0' }, { taskId: '2.5' }]) {
      expect(
        isHttpError(
          thrown(() => taskIdFromParams(params)),
          404
        ),
        JSON.stringify(params)
      ).toBe(true);
    }
  });
});
