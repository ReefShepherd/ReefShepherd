import { m } from '#lib/paraglide/messages.js';

const browsers: [RegExp, string][] = [
  [/Edg(?:e|A|iOS)?\//, 'Edge'],
  [/OPR\/|Opera/, 'Opera'],
  [/Firefox\/|FxiOS\//, 'Firefox'],
  [/Chrome\/|CriOS\//, 'Chrome'],
  [/Safari\//, 'Safari']
];

// iOS devices also say "like Mac OS X", and Android also says "Linux", so the specific names come first.
const systems: [RegExp, string][] = [
  [/Android/, 'Android'],
  [/iPhone|iPad|iPod/, 'iOS'],
  [/Windows/, 'Windows'],
  [/Mac OS X|Macintosh/, 'macOS'],
  [/CrOS/, 'ChromeOS'],
  [/Linux|X11/, 'Linux']
];

function firstMatch(table: [RegExp, string][], agent: string): string | undefined {
  return table.find(([pattern]) => pattern.test(agent))?.[1];
}

/** A short name such as "Firefox on Linux" for a device list, from the browser's user agent. */
export function describeUserAgent(agent: string): string {
  const browser = firstMatch(browsers, agent);
  const system = firstMatch(systems, agent);
  if (browser && system) return m.device_browser_on_system({ browser, system });
  return browser ?? system ?? m.sessions_unknown_device();
}
