import { api as defaultApi, type Api } from './client.js';
import { tankPath } from './tank-id.js';

/** One reading of the series endpoint; the value is in display units. */
export interface SeriesPoint {
  result_id: number;
  measured_at: string;
  value: number;
  in_range: boolean;
  note: string;
}

export interface Series {
  tank_id: number;
  parameter_id: number;
  name: string;
  unit: string;
  unit_label: string;
  decimals: number;
  range: { low: number | null; high: number | null };
  points: SeriesPoint[];
}

export function createChartsApi(api: Api) {
  return {
    /** Readings oldest first; from is inclusive and null means the whole history. */
    series: (tankId: number, parameterId: number, from: Date | null, signal?: AbortSignal) =>
      api.get<Series>(tankPath(tankId, 'parameters', parameterId, 'series'), {
        signal,
        query: { from: from?.toISOString() }
      })
  };
}

export const chartsApi = createChartsApi(defaultApi);
