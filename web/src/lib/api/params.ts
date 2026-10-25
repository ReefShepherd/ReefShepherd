import type { Schedule } from '#lib/schedule/schedule.js';
import { api as defaultApi, type Api } from './client.js';

export interface ParameterDefinition {
  id: number;
  /** Set for the built-in presets and null for custom parameters. */
  preset_key: string | null;
  name: string;
  unit_family: string;
  unit: string;
  unit_label: string;
  decimals: number;
  default_low: number | null;
  default_high: number | null;
  default_test_interval_days: number | null;
  sort_order: number;
  archived_at: string | null;
}

export interface NewParameter {
  name: string;
  unit_label: string;
  decimals: number;
  default_low: number | null;
  default_high: number | null;
  default_test_interval_days: number | null;
}

/** Fields to change on a custom parameter; a null default clears it. */
export type ParameterChanges = Partial<NewParameter>;

export interface LatestResult {
  result_id: number;
  value: number;
  measured_at: string;
  in_range: boolean;
}

export interface TankParameter {
  parameter_id: number;
  preset_key: string | null;
  name: string;
  unit_family: string;
  unit: string;
  unit_label: string;
  decimals: number;
  /** False until the tank has had settings saved for this parameter. */
  configured: boolean;
  enabled: boolean;
  ideal_low: number | null;
  ideal_high: number | null;
  schedule: Schedule;
  sort_order: number;
  latest: LatestResult | null;
  next_due: string | null;
  overdue: boolean;
}

export interface TankParameters {
  tank_id: number;
  /** The instance timezone that schedule times are in. */
  timezone: string;
  parameters: TankParameter[];
}

/** Fields to change on a tank's parameter; a null limit clears it, and a schedule replaces the whole one. */
export interface TankParameterChanges {
  enabled?: boolean;
  ideal_low?: number | null;
  ideal_high?: number | null;
  schedule?: Schedule;
  sort_order?: number;
}

export interface TestResult {
  id: number;
  tank_id: number;
  parameter_id: number;
  parameter_name: string;
  /** RFC 3339 UTC. */
  measured_at: string;
  value: number;
  unit: string;
  unit_label: string;
  in_range: boolean;
  note: string;
  source: string;
  created_by: number | null;
  created_at: string;
  updated_at: string;
}

export interface NewResultEntry {
  parameter_id: number;
  value: number;
  note?: string;
}

export interface NewResults {
  /** Omitted to use the time the server receives the entry. */
  measured_at?: string;
  results: NewResultEntry[];
}

export interface ResultPage {
  results: TestResult[];
  next_cursor: string | null;
}

export interface ResultQuery {
  parameter_id?: number;
  cursor?: string;
  limit?: number;
}

export interface ResultChanges {
  measured_at?: string;
  value?: number;
  note?: string;
}

export function createParamsApi(api: Api) {
  const tank = (tankId: number | string) => `/tanks/${encodeURIComponent(String(tankId))}`;
  return {
    definitions: (includeArchived = false) =>
      api
        .get<{
          parameters: ParameterDefinition[];
        }>('/parameters', { query: { include_archived: includeArchived || undefined } })
        .then((r) => r.parameters),
    createDefinition: (parameter: NewParameter) =>
      api.post<ParameterDefinition>('/parameters', parameter),
    updateDefinition: (id: number, changes: ParameterChanges) =>
      api.patch<ParameterDefinition>(`/parameters/${id}`, changes),
    archiveDefinition: (id: number) => api.post<ParameterDefinition>(`/parameters/${id}/archive`),
    unarchiveDefinition: (id: number) =>
      api.post<ParameterDefinition>(`/parameters/${id}/unarchive`),
    removeDefinition: (id: number) => api.del<void>(`/parameters/${id}`),

    tankParameters: (tankId: number | string, signal?: AbortSignal) =>
      api.get<TankParameters>(`${tank(tankId)}/parameters`, { signal }),
    saveTankParameter: (
      tankId: number | string,
      parameterId: number,
      changes: TankParameterChanges
    ) => api.put<TankParameter>(`${tank(tankId)}/parameters/${parameterId}`, changes),

    createResults: (tankId: number | string, entry: NewResults) =>
      api.post<{ results: TestResult[] }>(`${tank(tankId)}/results`, entry).then((r) => r.results),
    results: (tankId: number | string, query: ResultQuery = {}) =>
      api.get<ResultPage>(`${tank(tankId)}/results`, { query: { ...query } }),
    updateResult: (id: number, changes: ResultChanges) =>
      api.patch<TestResult>(`/results/${id}`, changes),
    removeResult: (id: number) => api.del<void>(`/results/${id}`)
  };
}

export const paramsApi = createParamsApi(defaultApi);
