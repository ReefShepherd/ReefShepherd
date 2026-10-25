import { settingsApi, type UnitFamily } from '#lib/api/settings.js';

let loaded = $state<UnitFamily[] | null>(null);
let pending: Promise<UnitFamily[]> | null = null;

/** The server's unit catalog, read once and shared by every screen that needs unit labels. */
export const unitCatalog = {
  get families(): UnitFamily[] {
    return loaded ?? [];
  },
  load(): Promise<UnitFamily[]> {
    if (loaded !== null) return Promise.resolve(loaded);
    pending ??= settingsApi
      .units()
      .then((families) => (loaded = families))
      .finally(() => (pending = null));
    return pending;
  }
};
