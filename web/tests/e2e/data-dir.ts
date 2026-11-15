import { realpathSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, isAbsolute, join } from 'node:path';

const dirName = 'reefshepherd-e2e';
const safePath = /^[\w./-]+$/;

/** The directory the end-to-end server's data is deleted from before every run, which must be plain characters, absolute and directly inside tmp. */
export function e2eDataDir(tmp: string = realpathSync(tmpdir())): string {
  const dir = join(tmp, dirName);
  if (!safePath.test(dir) || !isAbsolute(dir) || dirname(dir) !== tmp) {
    throw new Error(
      `refusing to use ${JSON.stringify(dir)} as the end-to-end data directory: the temp directory must be an absolute path of letters, digits, _ . / and - only`
    );
  }
  return dir;
}
