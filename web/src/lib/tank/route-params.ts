import { error } from '@sveltejs/kit';
import { parseId } from '#lib/api/tank-id.js';

function idOrNotFound(value: string | undefined): number {
  const id = parseId(value);
  if (id === null) error(404, 'Not found');
  return id;
}

/** The id of the tank in a route's params, once for the whole route; a missing or malformed id is a 404 page. */
export function tankIdFromParams(params: { id?: string }): number {
  return idOrNotFound(params.id);
}

/** The id of the parameter in a route's params; a missing or malformed id is a 404 page. */
export function parameterIdFromParams(params: { pid?: string }): number {
  return idOrNotFound(params.pid);
}
