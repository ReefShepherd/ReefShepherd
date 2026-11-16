import { error } from '@sveltejs/kit';
import { parseId } from '#lib/api/tank-id.js';

/** The id of the task in a route's params; a missing or malformed id is a 404 page. */
export function taskIdFromParams(params: { taskId?: string }): number {
  const id = parseId(params.taskId);
  if (id === null) error(404, 'Not found');
  return id;
}
