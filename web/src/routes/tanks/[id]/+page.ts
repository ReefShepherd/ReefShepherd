import { tankIdFromParams, tankPath } from '#lib/api/tank-id.js';
import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

// A tank has no page of its own; its settings come first.
export const load: PageLoad = ({ params }) => {
  redirect(307, tankPath(tankIdFromParams(params), 'settings'));
};
