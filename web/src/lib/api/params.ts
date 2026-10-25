import type { Schedule } from '#lib/schedule/schedule.js';
import { api as defaultApi, type Api } from './client.js';
import { tankPath } from './tank-id.js';

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
  return {
    definitions: (includeArchived = false, signal?: AbortSignal) =>
      api
        .get<{
          parameters: ParameterDefinition[];
        }>('/parameters', { signal, query: { include_archived: includeArchived || undefined } })
        .then((r) => r.parameters),
    createDefinition: (parameter: NewParameter, signal?: AbortSignal) =>
      api.post<ParameterDefinition>('/parameters', parameter, { signal }),
    updateDefinition: (id: number, changes: ParameterChanges, signal?: AbortSignal) =>
      api.patch<ParameterDefinition>(`/parameters/${id}`, changes, { signal }),
    archiveDefinition: (id: number, signal?: AbortSignal) =>
      api.post<ParameterDefinition>(`/parameters/${id}/archive`, undefined, { signal }),
    unarchiveDefinition: (id: number, signal?: AbortSignal) =>
      api.post<ParameterDefinition>(`/parameters/${id}/unarchive`, undefined, { signal }),
    removeDefinition: (id: number, signal?: AbortSignal) =>
      api.del<void>(`/parameters/${id}`, undefined, { signal }),

    tankParameters: (tankId: number, signal?: AbortSignal) =>
      api.get<TankParameters>(tankPath(tankId, 'parameters'), { signal }),
    saveTankParameter: (
      tankId: number,
      parameterId: number,
      changes: TankParameterChanges,
      signal?: AbortSignal
    ) => api.put<TankParameter>(tankPath(tankId, 'parameters', parameterId), changes, { signal }),

    createResults: (tankId: number, entry: NewResults, signal?: AbortSignal) =>
      api
        .post<{ results: TestResult[] }>(tankPath(tankId, 'results'), entry, { signal })
        .then((r) => r.results),
    results: (tankId: number, query: ResultQuery = {}, signal?: AbortSignal) =>
      api.get<ResultPage>(tankPath(tankId, 'results'), { signal, query: { ...query } }),
    updateResult: (id: number, changes: ResultChanges, signal?: AbortSignal) =>
      api.patch<TestResult>(`/results/${id}`, changes, { signal }),
    removeResult: (id: number, signal?: AbortSignal) =>
      api.del<void>(`/results/${id}`, undefined, { signal })
  };
}

export const paramsApi = createParamsApi(defaultApi);
