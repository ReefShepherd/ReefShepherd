import { describe, expect, it } from 'vitest';
import { authenticatorLink } from './otpauth.js';

describe('authenticatorLink', () => {
  it('keeps an otpauth totp URL', () => {
    const url = 'otpauth://totp/ReefShepherd:reef?secret=JBSWY3DPEHPK3PXP&issuer=ReefShepherd';
    expect(authenticatorLink(url)).toBe(url);
  });

  it('drops anything else, so a link can never run script or leave the app', () => {
    for (const url of [
      '',
      'javascript:alert(1)',
      'https://example.com/otpauth://totp/x',
      'data:text/html,hi',
      'otpauth://hotp/x?secret=A',
      'otpauth://totp/',
      'otpauth://totp/x y',
      ' otpauth://totp/x'
    ]) {
      expect(authenticatorLink(url), url).toBeUndefined();
    }
  });
});
