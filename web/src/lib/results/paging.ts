import type { ResultPage, ResultQuery, TestResult } from '#lib/api/params.js';

export const resultPageSize = 50;

/** The query for one page of the history, filtered to a parameter when one is chosen. */
export function resultQuery(
  parameterId: number | null,
  cursor: string | null,
  limit: number = resultPageSize
): ResultQuery {
  const query: ResultQuery = { limit };
  if (parameterId !== null) query.parameter_id = parameterId;
  if (cursor !== null) query.cursor = cursor;
  return query;
}

/** Newest first, with equal times broken by id so the order is stable. */
function newestFirst(a: TestResult, b: TestResult): number {
  if (a.measured_at !== b.measured_at) return a.measured_at < b.measured_at ? 1 : -1;
  return b.id - a.id;
}

/** Adds a fetched page after the rows already shown, skipping any row that is already there. */
export function appendPage(shown: readonly TestResult[], page: ResultPage): TestResult[] {
  const known = new Set(shown.map((r) => r.id));
  return [...shown, ...page.results.filter((r) => !known.has(r.id))];
}

/** Puts a saved result in place of its old copy and restores the newest-first order, since its time may have changed. */
export function replaceResult(shown: readonly TestResult[], updated: TestResult): TestResult[] {
  return shown.map((r) => (r.id === updated.id ? updated : r)).sort(newestFirst);
}

export function removeResult(shown: readonly TestResult[], id: number): TestResult[] {
  return shown.filter((r) => r.id !== id);
}

/** Puts newly logged results at the top of the history, honoring the parameter filter. */
export function addResults(
  shown: readonly TestResult[],
  added: readonly TestResult[],
  parameterId: number | null
): TestResult[] {
  const wanted = added.filter((r) => parameterId === null || r.parameter_id === parameterId);
  const known = new Set(shown.map((r) => r.id));
  return [...shown, ...wanted.filter((r) => !known.has(r.id))].sort(newestFirst);
}
