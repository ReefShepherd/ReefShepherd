import { api as defaultApi, type Api } from './client.js';

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

export interface DashboardApi {
  /** The signed-in user's cards for the tank, or the default cards on the first read. */
  get(tankId: string, signal?: AbortSignal): Promise<Layout>;
  /** Replaces the whole layout; the list must hold at least one card. */
  save(tankId: string, body: DashboardBody): Promise<Layout>;
  /** Discards the layout and returns the default one. */
  reset(tankId: string): Promise<Layout>;
}

export function createDashboardApi(api: Api): DashboardApi {
  const path = (tankId: string) => `/tanks/${encodeURIComponent(tankId)}/dashboard`;
  return {
    get: (tankId, signal) => api.get<Layout>(path(tankId), { signal }),
    save: (tankId, body) => api.put<Layout>(path(tankId), body),
    reset: (tankId) => api.post<Layout>(`${path(tankId)}/reset`)
  };
}

export const dashboardApi = createDashboardApi(defaultApi);
