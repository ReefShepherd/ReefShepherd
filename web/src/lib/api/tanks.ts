import { api as defaultApi, type Api } from './client.js';

export const tankTypes = ['reef', 'fowlr', 'frag', 'quarantine', 'other'] as const;
export type TankType = (typeof tankTypes)[number];

export interface Tank {
  id: number;
  name: string;
  tank_type: TankType;
  /** In the instance's volume unit, which volume_unit names. */
  volume: number | null;
  volume_unit: string;
  /** YYYY-MM-DD. */
  started_on: string | null;
  note: string;
  sort_order: number;
  archived_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface NewTank {
  name: string;
  tank_type: TankType;
  volume: number | null;
  started_on: string | null;
  note: string;
  /** Preset keys of the parameters to switch on for the tank. */
  presets: string[];
}

/** Fields to change on a tank; a null volume or start date clears it. */
export interface TankChanges {
  name?: string;
  tank_type?: TankType;
  volume?: number | null;
  started_on?: string | null;
  note?: string;
  sort_order?: number;
}

export function createTanksApi(api: Api) {
  return {
    list: (includeArchived = false) =>
      api.get<Tank[]>('/tanks', { query: { include_archived: includeArchived || undefined } }),
    get: (id: number | string) => api.get<Tank>(`/tanks/${encodeURIComponent(String(id))}`),
    create: (tank: NewTank) => api.post<Tank>('/tanks', tank),
    update: (id: number, changes: TankChanges) => api.patch<Tank>(`/tanks/${id}`, changes),
    archive: (id: number) => api.post<Tank>(`/tanks/${id}/archive`),
    unarchive: (id: number) => api.post<Tank>(`/tanks/${id}/unarchive`),
    /** Administrators only; confirm must be the tank's exact name. */
    remove: (id: number, confirm: string) =>
      api.del<void>(`/tanks/${id}`, undefined, { query: { confirm } })
  };
}

export const tanksApi = createTanksApi(defaultApi);
