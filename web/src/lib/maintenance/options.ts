import { getContext, setContext } from 'svelte';
import type { Preset } from '#lib/api/maintenance.js';
import type { Tank } from '#lib/api/tanks.js';

/** What the task forms offer, read once by the page that shows them. */
export interface MaintenanceOptions {
  /** The tanks a task can belong to. */
  readonly tanks: readonly Tank[];
  /** The templates that are not hidden. */
  readonly presets: readonly Preset[];
  /** The instance reminder time as HH:mm, which new schedules start at. */
  readonly dueTime: string;
}

const optionsKey = Symbol('maintenance-options');

/** Makes the options available to the components below the caller; each is read again on every use. */
export function provideOptions(source: {
  tanks: () => readonly Tank[];
  presets: () => readonly Preset[];
  dueTime: () => string;
}): void {
  setContext<MaintenanceOptions>(optionsKey, {
    get tanks() {
      return source.tanks();
    },
    get presets() {
      return source.presets();
    },
    get dueTime() {
      return source.dueTime();
    }
  });
}

export function useOptions(): MaintenanceOptions {
  return getContext<MaintenanceOptions>(optionsKey);
}
