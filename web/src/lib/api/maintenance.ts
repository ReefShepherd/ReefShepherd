import type { Schedule } from '#lib/schedule/schedule.js';
import { api as defaultApi, type Api, type Page } from './client.js';

export type CompletionOutcome = 'done' | 'skipped';

/** Which tasks a list holds by their due date; the server defaults to due. */
export type TaskStatus = 'due' | 'upcoming' | 'all';

export interface MaintenanceDefaults {
  /** The instance reminder time as HH:mm. */
  due_time: string;
}

export interface Preset {
  id: number;
  /** Set for the shipped presets and null for the admin's own. */
  preset_key: string | null;
  name: string;
  description: string;
  /** True for tasks that usually belong to the whole system. */
  tankless: boolean;
  schedule: Schedule;
  sort_order: number;
  archived_at: string | null;
}

export interface NewPreset {
  name: string;
  description?: string;
  tankless?: boolean;
  schedule: Schedule;
}

/** Fields to change on a preset; archived hides or shows it. */
export interface PresetChanges {
  name?: string;
  description?: string;
  tankless?: boolean;
  schedule?: Schedule;
  archived?: boolean;
}

export interface Task {
  id: number;
  /** Null for a task of the whole system. */
  tank_id: number | null;
  tank_name: string | null;
  preset_id: number | null;
  name: string;
  description: string;
  schedule: Schedule;
  notify: boolean;
  paused: boolean;
  last_done_at: string | null;
  last_outcome: CompletionOutcome | null;
  /** RFC 3339 UTC, null when the task has no schedule. */
  next_due_at: string | null;
  overdue: boolean;
  created_at: string;
  updated_at: string;
}

export interface NewTask {
  tank_id: number | null;
  preset_id?: number | null;
  name: string;
  description?: string;
  schedule: Schedule;
  notify?: boolean;
  paused?: boolean;
  /** Date the task was last done, as YYYY-MM-DD. */
  last_done_on?: string;
}

/** Fields to change on a task; a null tank_id makes it a whole-system task, and a schedule replaces the whole one. */
export interface TaskChanges {
  tank_id?: number | null;
  name?: string;
  description?: string;
  schedule?: Schedule;
  notify?: boolean;
  paused?: boolean;
}

export interface TaskQuery {
  tank_id?: number;
  /** With tank_id, also lists the tasks of the whole system. */
  include_tankless?: boolean;
  status?: TaskStatus;
}

export interface Completion {
  id: number;
  task_id: number;
  done_at: string;
  outcome: CompletionOutcome;
  note: string;
  done_by_name: string | null;
  created_at: string;
}

export interface NewCompletion {
  /** Omitted to use the time the server receives the entry. */
  done_at?: string;
  outcome?: CompletionOutcome;
  note?: string;
}

export interface CompletionChanges {
  done_at?: string;
  outcome?: CompletionOutcome;
  note?: string;
}

export interface CompletionResult {
  completion: Completion;
  task: Task;
}

export type CompletionPage = Page<Completion>;

export interface CompletionQuery {
  cursor?: string;
  limit?: number;
}

export function createMaintenanceApi(api: Api) {
  return {
    defaults: (signal?: AbortSignal) =>
      api.get<MaintenanceDefaults>('/maintenance/defaults', { signal }),

    presets: (includeArchived = false, signal?: AbortSignal) =>
      api.list<Preset>('/maintenance/presets', {
        signal,
        query: { include_archived: includeArchived || undefined }
      }),
    createPreset: (preset: NewPreset, signal?: AbortSignal) =>
      api.post<Preset>('/maintenance/presets', preset, { signal }),
    updatePreset: (id: number, changes: PresetChanges, signal?: AbortSignal) =>
      api.patch<Preset>(`/maintenance/presets/${id}`, changes, { signal }),

    tasks: (query: TaskQuery = {}, signal?: AbortSignal) =>
      api.list<Task>('/maintenance/tasks', { signal, query: { ...query } }),
    task: (id: number, signal?: AbortSignal) =>
      api.get<Task>(`/maintenance/tasks/${id}`, { signal }),
    createTask: (task: NewTask, signal?: AbortSignal) =>
      api.post<Task>('/maintenance/tasks', task, { signal }),
    updateTask: (id: number, changes: TaskChanges, signal?: AbortSignal) =>
      api.patch<Task>(`/maintenance/tasks/${id}`, changes, { signal }),
    removeTask: (id: number, signal?: AbortSignal) =>
      api.del<void>(`/maintenance/tasks/${id}`, undefined, { signal }),

    complete: (id: number, completion: NewCompletion = {}, signal?: AbortSignal) =>
      api.post<CompletionResult>(`/maintenance/tasks/${id}/complete`, completion, { signal }),
    completions: (taskId: number, query: CompletionQuery = {}, signal?: AbortSignal) =>
      api.listPage<Completion>(`/maintenance/tasks/${taskId}/completions`, {
        signal,
        query: { ...query }
      }),
    updateCompletion: (id: number, changes: CompletionChanges, signal?: AbortSignal) =>
      api.patch<Completion>(`/maintenance/completions/${id}`, changes, { signal }),
    removeCompletion: (id: number, signal?: AbortSignal) =>
      api.del<void>(`/maintenance/completions/${id}`, undefined, { signal })
  };
}

export const maintenanceApi = createMaintenanceApi(defaultApi);
