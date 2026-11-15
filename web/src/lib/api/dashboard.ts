import { api as defaultApi, type Api } from './client.js';
import { tankPath } from './tank-id.js';

export type CardSize = 'normal' | 'wide';

/** One saved card as the server reports it. */
export interface DashboardCard {
  id: number;
  card_type: string;
  parameter_id: number | null;
  position: number;
  size: CardSize;
  options: Record<string, unknown>;
}

/** One card of a replacement layout; the order of the list decides the positions the server stores. */
export interface CardRow {
  card_type: string;
  parameter_id: number | null;
  position: number;
  size: CardSize;
  options: Record<string, unknown>;
}

export interface DashboardBody {
  cards: CardRow[];
}

export interface Layout {
  cards: DashboardCard[];
}

export function createDashboardApi(api: Api) {
  return {
    /** The signed-in user's cards for the tank, or the default cards on the first read. */
    get: (tankId: number, signal?: AbortSignal) =>
      api.get<Layout>(tankPath(tankId, 'dashboard'), { signal }),
    /** Replaces the whole layout; the list must hold at least one card. */
    save: (tankId: number, body: DashboardBody, signal?: AbortSignal) =>
      api.put<Layout>(tankPath(tankId, 'dashboard'), body, { signal }),
    /** Discards the layout and returns the default one. */
    reset: (tankId: number, signal?: AbortSignal) =>
      api.post<Layout>(tankPath(tankId, 'dashboard', 'reset'), undefined, { signal })
  };
}

export const dashboardApi = createDashboardApi(defaultApi);
