import { describe, expect, it } from 'vitest';
import { e2eDataDir } from '../tests/e2e/data-dir.js';

describe('e2eDataDir', () => {
  it('names a fixed directory directly inside the temp directory', () => {
    expect(e2eDataDir('/tmp')).toBe('/tmp/reefshepherd-e2e');
    expect(e2eDataDir('/var/tmp/claude-1000')).toBe('/var/tmp/claude-1000/reefshepherd-e2e');
  });

  it('refuses a temp directory with a space, which a shell would split', () => {
    expect(() => e2eDataDir('/tmp/claude-1000/qa sp')).toThrow(/refusing/);
  });

  it('refuses shell metacharacters and quotes', () => {
    for (const tmp of ['/tmp/a;b', '/tmp/$(x)', '/tmp/`x`', "/tmp/a'b", '/tmp/a"b', '/tmp/a&b']) {
      expect(() => e2eDataDir(tmp), tmp).toThrow(/refusing/);
    }
  });

  it('refuses a relative temp directory', () => {
    expect(() => e2eDataDir('tmp')).toThrow(/refusing/);
    expect(() => e2eDataDir('.')).toThrow(/refusing/);
  });

  it('refuses a temp directory that is not already resolved', () => {
    expect(() => e2eDataDir('/tmp/a/../b')).toThrow(/refusing/);
    expect(() => e2eDataDir('/tmp//a')).toThrow(/refusing/);
    expect(() => e2eDataDir('/tmp/')).toThrow(/refusing/);
  });
});
