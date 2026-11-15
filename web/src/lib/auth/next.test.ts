import { describe, expect, it } from 'vitest';
import { loginUrl, safeNext } from './next.js';

describe('safeNext', () => {
  it('keeps a same-origin relative path with its query and hash', () => {
    expect(safeNext('/tanks/3/charts/5?range=30d#top')).toBe('/tanks/3/charts/5?range=30d#top');
    expect(safeNext('/settings/account')).toBe('/settings/account');
  });

  it('falls back to the home page when there is nothing usable', () => {
    expect(safeNext(null)).toBe('/');
    expect(safeNext(undefined)).toBe('/');
    expect(safeNext('')).toBe('/');
    expect(safeNext('/')).toBe('/');
  });

  it('refuses absolute and protocol-relative URLs', () => {
    for (const raw of [
      'https://evil.example/',
      'http://evil.example',
      '//evil.example',
      '//evil.example/path',
      '///evil.example',
      'javascript:alert(1)',
      'data:text/html,x',
      'tanks',
      'evil.example/x'
    ]) {
      expect(safeNext(raw), raw).toBe('/');
    }
  });

  it('refuses backslash and control-character tricks that browsers read as another host', () => {
    for (const raw of [
      '/\\evil.example',
      '/\\/evil.example',
      '\\\\evil.example',
      '/a\\b',
      '/\t/evil.example',
      '/\n/evil.example',
      '/\r/evil.example',
      '/a\u0000b',
      '/\u007f'
    ]) {
      expect(safeNext(raw), JSON.stringify(raw)).toBe('/');
    }
  });

  it('refuses the sign-in and setup pages, which would loop', () => {
    expect(safeNext('/login')).toBe('/');
    expect(safeNext('/login?next=/tanks')).toBe('/');
    expect(safeNext('/setup')).toBe('/');
    expect(safeNext('/tanks/../login')).toBe('/');
    expect(safeNext('/loginx')).toBe('/loginx');
  });

  it('refuses a path that is too long', () => {
    expect(safeNext(`/${'a'.repeat(3000)}`)).toBe('/');
  });
});

describe('loginUrl', () => {
  it('leaves out the return address for the home page', () => {
    expect(loginUrl('/')).toBe('/login');
  });

  it('carries the current page, encoded', () => {
    expect(loginUrl('/tanks/3?x=1&y=2')).toBe('/login?next=%2Ftanks%2F3%3Fx%3D1%26y%3D2');
  });

  it('never carries an unsafe address', () => {
    expect(loginUrl('//evil.example')).toBe('/login');
  });
});
