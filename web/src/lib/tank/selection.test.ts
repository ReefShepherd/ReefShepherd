import { describe, expect, it } from 'vitest';
import { reconcileCurrentTank, retargetPath } from './selection.js';

const tank = (id: number, archived = false) => ({
  id,
  archived_at: archived ? '2026-01-01T00:00:00Z' : null
});

describe('reconcileCurrentTank', () => {
  const tanks = [tank(1), tank(2), tank(3, true)];

  it('keeps the stored tank while it is active', () => {
    expect(reconcileCurrentTank('2', tanks, 1)).toBe('2');
  });

  it('clears a stored tank that no longer exists by falling back to the default, then the first', () => {
    expect(reconcileCurrentTank('9', tanks, 2)).toBe('2');
    expect(reconcileCurrentTank('9', tanks, null)).toBe('1');
  });

  it('does not keep an archived tank or fall back to an archived default', () => {
    expect(reconcileCurrentTank('3', tanks, 3)).toBe('1');
  });

  it('is null when there is no active tank', () => {
    expect(reconcileCurrentTank('1', [], 1)).toBeNull();
    expect(reconcileCurrentTank(null, [tank(3, true)], null)).toBeNull();
  });

  it('picks a tank when nothing is stored', () => {
    expect(reconcileCurrentTank(null, tanks, null)).toBe('1');
  });
});

describe('retargetPath', () => {
  it('moves a tank page to another tank', () => {
    expect(retargetPath('/tanks/1/tests', '2')).toBe('/tanks/2/tests');
    expect(retargetPath('/tanks/1/charts/5', '2')).toBe('/tanks/2/charts/5');
    expect(retargetPath('/tanks/1', '2')).toBe('/tanks/2');
  });

  it('leaves pages that do not belong to one tank alone', () => {
    expect(retargetPath('/tanks', '2')).toBeNull();
    expect(retargetPath('/tanks/new', '2')).toBeNull();
    expect(retargetPath('/tanks/parameters', '2')).toBeNull();
    expect(retargetPath('/', '2')).toBeNull();
    expect(retargetPath('/settings/users', '2')).toBeNull();
  });
});
