import { settingsApi, type UnitFamily } from '#lib/api/settings.js';
import { cachedResource } from '#lib/cache/cached-resource.svelte.js';

const catalog = cachedResource((signal) => settingsApi.units(signal));

/** The server's unit catalog, read once and shared by every screen that needs unit labels. */
export const unitCatalog = {
  get families(): UnitFamily[] {
    return catalog.value ?? [];
  },
  load(): Promise<UnitFamily[]> {
    return catalog.value ? Promise.resolve(catalog.value) : catalog.load();
  }
};
