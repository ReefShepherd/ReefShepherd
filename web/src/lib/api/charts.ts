import { api as defaultApi, type Api } from './client.js';

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

export interface ChartsApi {
  /** Readings oldest first; from is inclusive and null means the whole history. */
  series(
    tankId: string,
    parameterId: string,
    from: Date | null,
    signal?: AbortSignal
  ): Promise<Series>;
}

export function createChartsApi(api: Api): ChartsApi {
  const tank = (tankId: string) => `/tanks/${encodeURIComponent(tankId)}`;
  return {
    series(tankId, parameterId, from, signal) {
      return api.get<Series>(
        `${tank(tankId)}/parameters/${encodeURIComponent(parameterId)}/series`,
        {
          signal,
          query: { from: from?.toISOString() }
        }
      );
    }
  };
}

export const chartsApi = createChartsApi(defaultApi);
