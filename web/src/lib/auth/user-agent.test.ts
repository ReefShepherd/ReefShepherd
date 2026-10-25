import { describe, expect, it } from 'vitest';
import { m } from '#lib/paraglide/messages.js';
import { describeUserAgent } from './user-agent.js';

const cases: [string, string][] = [
  [
    'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36',
    'Chrome on Linux'
  ],
  ['Mozilla/5.0 (X11; Linux x86_64; rv:131.0) Gecko/20100101 Firefox/131.0', 'Firefox on Linux'],
  [
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0',
    'Edge on Windows'
  ],
  [
    'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1',
    'Safari on iOS'
  ],
  [
    'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36',
    'Chrome on Android'
  ],
  [
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15',
    'Safari on macOS'
  ]
];

describe('describeUserAgent', () => {
  it.each(cases)('describes %s', (agent, expected) => {
    expect(describeUserAgent(agent)).toBe(expected);
  });

  it('names only what it recognises', () => {
    expect(describeUserAgent('curl/8.0')).toBe(m.sessions_unknown_device());
    expect(describeUserAgent('')).toBe(m.sessions_unknown_device());
    expect(describeUserAgent('SomeApp (Linux)')).toBe('Linux');
  });
});
