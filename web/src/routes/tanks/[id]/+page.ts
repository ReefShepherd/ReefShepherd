import { tankPath } from '#lib/api/tank-id.js';
import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

// A tank has no page of its own; its settings come first.
export const load: PageLoad = async ({ parent }) => {
  const { tankId } = await parent();
  redirect(307, tankPath(tankId, 'settings'));
};
