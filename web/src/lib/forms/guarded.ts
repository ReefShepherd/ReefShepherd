import { focusFirstInvalid } from '#lib/dom.js';
import type { FieldErrors } from './codes.js';
import { serverErrors, type ShownFields } from './serverErrors.js';

export interface GuardHooks {
  /** Receives the translated failure text, and an empty string when a new attempt starts. */
  failure: (text: string) => void;
  /** Receives true while the action runs. */
  busy?: (busy: boolean) => void;
  /** Receives the fields the server flagged among shown, and an empty object when a new attempt starts. */
  fields?: (fields: FieldErrors) => void;
  /** The fields the form has an input for, so a failure that flags none of them is reported through failure; without it every flagged field is shown. */
  shown?: ShownFields;
  /** The form to move focus within, to its first invalid field, after a failure. */
  form?: () => HTMLFormElement | null | undefined;
}

/** Runs an action that may call the API and reports its failure as field errors or text instead of throwing. */
export async function guarded(action: () => Promise<void>, hooks: GuardHooks): Promise<void> {
  hooks.failure('');
  hooks.fields?.({});
  hooks.busy?.(true);
  try {
    await action();
  } catch (e) {
    const { fields, message } = serverErrors(e, hooks.fields ? hooks.shown : []);
    hooks.fields?.(fields);
    hooks.failure(message ?? '');
    if (hooks.form) await focusFirstInvalid(hooks.form());
  } finally {
    hooks.busy?.(false);
  }
}
