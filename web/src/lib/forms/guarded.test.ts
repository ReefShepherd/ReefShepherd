import { describe, expect, it } from 'vitest';
import { ApiError } from '#lib/api/client.js';
import { m } from '#lib/paraglide/messages.js';
import type { FieldErrors } from './codes.js';
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

const rejected = (fields: Record<string, string>) =>
  new ApiError(422, 'validation', 'invalid', fields);

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

  it('hands the flagged fields to the form instead of showing a general failure', async () => {
    const seen: FieldErrors[] = [];
    const failures: string[] = [];
    await guarded(
      async () => {
        throw rejected({ username: 'already taken', other: 'required' });
      },
      {
        failure: (t) => failures.push(t),
        fields: (f) => seen.push(f),
        shown: ['username']
      }
    );
    expect(seen).toEqual([{}, { username: 'taken' }]);
    expect(failures).toEqual(['', '']);
  });

  it('shows the general failure when none of the flagged fields is one the form shows', async () => {
    const seen: FieldErrors[] = [];
    const failures: string[] = [];
    await guarded(
      async () => {
        throw rejected({ cards: 'required' });
      },
      { failure: (t) => failures.push(t), fields: (f) => seen.push(f), shown: ['name'] }
    );
    expect(seen).toEqual([{}, {}]);
    expect(failures).toEqual(['', m.api_error_validation()]);
  });

  it('shows the general failure when the form has no field hook', async () => {
    const failures: string[] = [];
    await guarded(
      async () => {
        throw rejected({ name: 'required' });
      },
      { failure: (t) => failures.push(t) }
    );
    expect(failures).toEqual(['', m.api_error_validation()]);
  });

  it('moves focus to the first invalid field of the form after a failure only', async () => {
    const selectors: string[] = [];
    let focused = 0;
    const form = {
      querySelector: (selector: string) => {
        selectors.push(selector);
        return { focus: () => focused++ };
      }
    } as unknown as HTMLFormElement;
    const hooks = { failure: () => {}, form: () => form };
    await guarded(async () => {}, hooks);
    expect(focused).toBe(0);
    await guarded(async () => {
      throw new Error('boom');
    }, hooks);
    expect(focused).toBe(1);
    expect(selectors).toEqual(['[aria-invalid="true"]']);
  });
});
