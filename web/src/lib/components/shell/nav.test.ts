import { describe, expect, it } from 'vitest';
import { navEntries } from './nav.js';

const hrefs = (tankId: number | null) => navEntries(tankId).map((e) => e.href);
const activeIcons = (tankId: number | null, pathname: string) =>
  navEntries(tankId)
    .filter((e) => e.isActive(pathname))
    .map((e) => e.icon);

describe('navEntries', () => {
  it('lists every top-level page in order with a visible label', () => {
    expect(navEntries(null).map((e) => e.icon)).toEqual([
      'dashboard',
      'tanks',
      'tests',
      'charts',
      'maintenance',
      'settings'
    ]);
    for (const entry of navEntries(null)) expect(entry.label()).not.toBe('');
  });

  it('points Log tests and Charts at the current tank', () => {
    expect(hrefs(3)).toEqual([
      '/',
      '/tanks',
      '/tanks/3/tests',
      '/tanks/3/charts',
      '/tanks/3/maintenance',
      '/settings'
    ]);
  });

  it('points Log tests and Charts at the tank list when there is no current tank', () => {
    expect(hrefs(null)).toEqual(['/', '/tanks', '/tanks', '/tanks', '/maintenance', '/settings']);
  });

  it('marks exactly one entry active on each page', () => {
    expect(activeIcons(3, '/')).toEqual(['dashboard']);
    expect(activeIcons(3, '/tanks')).toEqual(['tanks']);
    expect(activeIcons(3, '/tanks/3/settings')).toEqual(['tanks']);
    expect(activeIcons(3, '/tanks/3/tests')).toEqual(['tests']);
    expect(activeIcons(3, '/tanks/9/charts/5')).toEqual(['charts']);
    expect(activeIcons(3, '/settings')).toEqual(['settings']);
  });

  it('marks Maintenance active on the all-tanks page and on every tank maintenance page', () => {
    expect(activeIcons(3, '/maintenance')).toEqual(['maintenance']);
    expect(activeIcons(3, '/maintenance/7')).toEqual(['maintenance']);
    expect(activeIcons(3, '/maintenance/templates')).toEqual(['maintenance']);
    expect(activeIcons(3, '/tanks/3/maintenance')).toEqual(['maintenance']);
    expect(activeIcons(3, '/tanks/9/maintenance/')).toEqual(['maintenance']);
    expect(activeIcons(null, '/maintenance')).toEqual(['maintenance']);
  });

  it('keeps the maintenance section out of Tanks without hiding other tank pages', () => {
    expect(activeIcons(3, '/tanks/3/maintenance')).not.toContain('tanks');
    expect(activeIcons(3, '/tanks/3/maintenance-notes')).toEqual(['tanks']);
    expect(activeIcons(3, '/maintenance-notes')).toEqual([]);
  });

  it('gives only Maintenance a shorter label for the phone bar', () => {
    const short = navEntries(null)
      .filter((e) => e.shortLabel !== undefined)
      .map((e) => [e.icon, e.label(), e.shortLabel?.()]);
    expect(short).toEqual([['maintenance', 'Maintenance', 'Tasks']]);
  });

  it('marks only Tanks active on the tank list when there is no current tank', () => {
    expect(activeIcons(null, '/tanks')).toEqual(['tanks']);
  });
});
