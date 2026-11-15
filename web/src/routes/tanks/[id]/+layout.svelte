<script lang="ts">
  import { page } from '$app/state';
  import { isApiError } from '#lib/api/client.js';
  import { tanksApi, type Tank } from '#lib/api/tanks.js';
  import { currentTank } from '#lib/tank/current-tank.svelte.js';
  import { provideOpenTank } from '#lib/tank/open-tank.js';

  let { children } = $props();

  let tank = $state.raw<Tank | null>(null);
  provideOpenTank(() => tank);

  // Opening a tank's page makes it the current tank, and a tank that no longer exists is forgotten.
  $effect(() => {
    const id = page.params.id;
    tank = null;
    if (!id || !/^\d+$/.test(id)) return;
    let stale = false;
    tanksApi
      .get(id)
      .then((loaded) => {
        if (stale) return;
        tank = loaded;
        if (loaded.archived_at === null) currentTank.set(id);
      })
      .catch((e: unknown) => {
        if (!stale && isApiError(e) && e.status === 404 && currentTank.id === id) {
          currentTank.set(null);
        }
      });
    return () => {
      stale = true;
    };
  });
</script>

{@render children()}
