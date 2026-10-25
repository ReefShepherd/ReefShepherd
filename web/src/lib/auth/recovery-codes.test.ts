import { describe, expect, it } from 'vitest';
import { recoveryCodesFilename, recoveryCodesText } from './recovery-codes.js';

const info = {
  username: 'jayce',
  instanceName: 'Garage reef',
  createdAt: new Date('2026-10-24T12:30:00Z'),
  timeZone: 'UTC'
};
const codes = ['ABCDE-FGHJK', 'LMNPQ-RSTUV', 'WXYZ2-34567'];

describe('recoveryCodesText', () => {
  it('writes every code on its own line, in order, at the end of the file', () => {
    const lines = recoveryCodesText(codes, info).split('\n');
    expect(lines.slice(-4)).toEqual([...codes, '']);
  });

  it('names the account, the instance and the date', () => {
    const text = recoveryCodesText(codes, info);
    expect(text).toContain('jayce');
    expect(text).toContain('Garage reef');
    expect(text).toContain('2026-10-24');
    expect(text.startsWith('ReefShepherd')).toBe(true);
  });

  it('writes the date of the instance timezone, not the UTC date', () => {
    const late = { ...info, createdAt: new Date('2026-10-24T23:30:00Z') };
    expect(recoveryCodesText(codes, { ...late, timeZone: 'Pacific/Auckland' })).toContain(
      '2026-10-25'
    );
    expect(recoveryCodesText(codes, { ...late, timeZone: 'America/Los_Angeles' })).toContain(
      '2026-10-24'
    );
  });

  it('ends with a newline and has no code more than once', () => {
    const text = recoveryCodesText(codes, info);
    expect(text.endsWith('\n')).toBe(true);
    for (const code of codes) expect(text.split(code)).toHaveLength(2);
  });

  it('downloads as a plain text file', () => {
    expect(recoveryCodesFilename.endsWith('.txt')).toBe(true);
  });
});
