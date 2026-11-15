import { parameterIdFromParams } from '#lib/tank/route-params.js';
import type { PageLoad } from './$types';

// The page reads the parameter id from here, so a malformed id never reaches it.
export const load: PageLoad = ({ params }) => ({ parameterId: parameterIdFromParams(params) });
