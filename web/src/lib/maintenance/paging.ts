import type { Completion, CompletionPage, CompletionQuery } from '#lib/api/maintenance.js';

export const completionPageSize = 50;

/** The query for one page of a task's history. */
export function completionQuery(
  cursor: string | null,
  limit: number = completionPageSize
): CompletionQuery {
  const query: CompletionQuery = { limit };
  if (cursor !== null) query.cursor = cursor;
  return query;
}

/** Newest first, with equal times broken by id so the order is stable. */
function newestFirst(a: Completion, b: Completion): number {
  if (a.done_at !== b.done_at) return a.done_at < b.done_at ? 1 : -1;
  return b.id - a.id;
}

/** Adds a fetched page after the entries already shown, skipping any that is already there. */
export function appendPage(shown: readonly Completion[], page: CompletionPage): Completion[] {
  const known = new Set(shown.map((c) => c.id));
  return [...shown, ...page.items.filter((c) => !known.has(c.id))];
}

/** Puts a saved entry in place of its old copy and restores the newest-first order, since its time may have changed. */
export function replaceCompletion(shown: readonly Completion[], updated: Completion): Completion[] {
  return shown.map((c) => (c.id === updated.id ? updated : c)).sort(newestFirst);
}

export function removeCompletion(shown: readonly Completion[], id: number): Completion[] {
  return shown.filter((c) => c.id !== id);
}

/** Puts a new entry in the history in order, unless it is already there. */
export function addCompletion(shown: readonly Completion[], added: Completion): Completion[] {
  if (shown.some((c) => c.id === added.id)) return [...shown];
  return [...shown, added].sort(newestFirst);
}
