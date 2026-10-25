import { api as defaultApi, type Api } from './client.js';
import { tankPath } from './tank-id.js';

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
    list: (includeArchived = false, signal?: AbortSignal) =>
      api.get<Tank[]>('/tanks', {
        signal,
        query: { include_archived: includeArchived || undefined }
      }),
    get: (id: number, signal?: AbortSignal) => api.get<Tank>(tankPath(id), { signal }),
    create: (tank: NewTank, signal?: AbortSignal) => api.post<Tank>('/tanks', tank, { signal }),
    update: (id: number, changes: TankChanges, signal?: AbortSignal) =>
      api.patch<Tank>(tankPath(id), changes, { signal }),
    archive: (id: number, signal?: AbortSignal) =>
      api.post<Tank>(tankPath(id, 'archive'), undefined, { signal }),
    unarchive: (id: number, signal?: AbortSignal) =>
      api.post<Tank>(tankPath(id, 'unarchive'), undefined, { signal }),
    /** Administrators only; confirm must be the tank's exact name. */
    remove: (id: number, confirm: string, signal?: AbortSignal) =>
      api.del<void>(tankPath(id), undefined, { signal, query: { confirm } })
  };
}

export const tanksApi = createTanksApi(defaultApi);
