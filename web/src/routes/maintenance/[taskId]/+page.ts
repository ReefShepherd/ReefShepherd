import { taskIdFromParams } from '#lib/maintenance/route-params.js';
import type { PageLoad } from './$types';

// The page reads its task id from here, so a malformed id never reaches it.
export const load: PageLoad = ({ params }) => ({ taskId: taskIdFromParams(params) });
