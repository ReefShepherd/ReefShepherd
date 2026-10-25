import { error } from '@sveltejs/kit';

/** A tank id from text such as a URL segment or a stored value; null unless it is a positive whole number. */
export function parseTankId(value: unknown): number | null {
  if (typeof value !== 'string' || !/^[1-9]\d*$/.test(value)) return null;
  const id = Number(value);
  return Number.isSafeInteger(id) ? id : null;
}

/** The id of the tank in a route's params, once for the whole route; a missing or malformed id is a 404 page. */
export function tankIdFromParams(params: { id?: string }): number {
  const id = parseTankId(params.id);
  if (id === null) error(404, 'Not found');
  return id;
}

/** The path of a tank or of something under it, such as /tanks/3/results; the API and the app's pages share this shape. */
export function tankPath(tankId: number, ...segments: (string | number)[]): string {
  return `/tanks/${[tankId, ...segments].map((s) => encodeURIComponent(String(s))).join('/')}`;
}
