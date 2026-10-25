import { describe, expect, it } from 'vitest';
import { pathMatches } from './path.js';

describe('pathMatches', () => {
  it('matches the root path only exactly', () => {
    expect(pathMatches('/', '/')).toBe(true);
    expect(pathMatches('/tanks', '/')).toBe(false);
  });

  it('matches a base path and everything below it', () => {
    expect(pathMatches('/tanks', '/tanks')).toBe(true);
    expect(pathMatches('/tanks/3/charts/5', '/tanks')).toBe(true);
  });

  it('does not match a sibling that shares a prefix', () => {
    expect(pathMatches('/tanksters', '/tanks')).toBe(false);
    expect(pathMatches('/setup2', '/setup')).toBe(false);
  });
});
