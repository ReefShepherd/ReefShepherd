import { tankIdFromParams } from '#lib/tank/route-params.js';
import type { LayoutLoad } from './$types';

// Every page under a tank reads its id from here, so a malformed id never reaches them.
export const load: LayoutLoad = ({ params }) => ({ tankId: tankIdFromParams(params) });
