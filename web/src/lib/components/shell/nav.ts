import { tankPath } from '#lib/api/tank-id.js';
import { m } from '#lib/paraglide/messages.js';
import { pathMatches } from '#lib/path.js';

export type NavIconName = 'dashboard' | 'tanks' | 'tests' | 'charts' | 'settings';

export interface NavEntry {
  icon: NavIconName;
  href: string;
  label: () => string;
  isActive: (pathname: string) => boolean;
}

// Pages that live under a tank, such as /tanks/3/tests, belong to their own entry and not to Tanks.
const tankSection = (section: string) => new RegExp(`^/tanks/[^/]+/${section}(/|$)`);
const testsPage = tankSection('tests');
const chartsPage = tankSection('charts');

/** The entries of the main navigation; tankId is the tank the tank-scoped entries open. */
export function navEntries(tankId: number | null): NavEntry[] {
  return [
    {
      icon: 'dashboard',
      href: '/',
      label: m.nav_dashboard,
      isActive: (p) => pathMatches(p, '/')
    },
    {
      icon: 'tanks',
      href: '/tanks',
      label: m.nav_tanks,
      isActive: (p) => pathMatches(p, '/tanks') && !testsPage.test(p) && !chartsPage.test(p)
    },
    {
      icon: 'tests',
      href: tankId === null ? '/tanks' : tankPath(tankId, 'tests'),
      label: m.nav_tests,
      isActive: (p) => testsPage.test(p)
    },
    {
      icon: 'charts',
      href: tankId === null ? '/tanks' : tankPath(tankId, 'charts'),
      label: m.nav_charts,
      isActive: (p) => chartsPage.test(p)
    },
    {
      icon: 'settings',
      href: '/settings',
      label: m.nav_settings,
      isActive: (p) => pathMatches(p, '/settings')
    }
  ];
}
