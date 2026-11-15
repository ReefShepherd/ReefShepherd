import { errorText } from './error-text.js';

export interface GuardHooks {
  /** Receives the translated failure text, and an empty string when a new attempt starts. */
  failure: (text: string) => void;
  /** Receives true while the action runs. */
  busy?: (busy: boolean) => void;
}

/** Runs an action that may call the API and reports its failure as text instead of throwing. */
export async function guarded(action: () => Promise<void>, hooks: GuardHooks): Promise<void> {
  hooks.failure('');
  hooks.busy?.(true);
  try {
    await action();
  } catch (e) {
    hooks.failure(errorText(e));
  } finally {
    hooks.busy?.(false);
  }
}
