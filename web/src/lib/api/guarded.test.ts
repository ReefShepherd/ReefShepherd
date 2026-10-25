import { describe, expect, it } from 'vitest';
import { m } from '#lib/paraglide/messages.js';
import { ApiError } from './client.js';
import { guarded } from './guarded.js';

function recorder() {
  const log: string[] = [];
  return {
    log,
    hooks: {
      failure: (text: string) => log.push(`failure:${text}`),
      busy: (busy: boolean) => log.push(`busy:${busy}`)
    }
  };
}

describe('guarded', () => {
  it('clears the failure, marks busy while running and reports nothing on success', async () => {
    const { log, hooks } = recorder();
    await guarded(async () => void log.push('run'), hooks);
    expect(log).toEqual(['failure:', 'busy:true', 'run', 'busy:false']);
  });

  it('turns a thrown API error into its translated text and stops being busy', async () => {
    const { log, hooks } = recorder();
    await guarded(async () => {
      throw new ApiError(403, 'forbidden', 'admins only');
    }, hooks);
    expect(log).toEqual([
      'failure:',
      'busy:true',
      `failure:${m.api_error_forbidden()}`,
      'busy:false'
    ]);
  });

  it('works without a busy hook', async () => {
    const texts: string[] = [];
    await guarded(
      async () => {
        throw new Error('boom');
      },
      { failure: (t) => texts.push(t) }
    );
    expect(texts).toEqual(['', m.api_error_unknown()]);
  });
});
