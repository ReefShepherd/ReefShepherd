<script lang="ts">
  import { isApiError } from '#lib/api/client.js';
  import { tanksApi, type Tank } from '#lib/api/tanks.js';
  import { currentTank } from '#lib/tank/current-tank.svelte.js';
  import { provideOpenTank } from '#lib/tank/open-tank.js';

  let { data, children } = $props();

  let tank = $state.raw<Tank | null>(null);
  provideOpenTank(() => tank);

  // Opening a tank's page makes it the current tank, and a tank that no longer exists is forgotten.
  $effect(() => {
    const id = data.tankId;
    const controller = new AbortController();
    tank = null;
    tanksApi
      .get(id, controller.signal)
      .then((loaded) => {
        if (controller.signal.aborted) return;
        tank = loaded;
        if (loaded.archived_at === null) currentTank.set(id);
      })
      .catch((e: unknown) => {
        if (
          !controller.signal.aborted &&
          isApiError(e) &&
          e.status === 404 &&
          currentTank.id === id
        ) {
          currentTank.set(null);
        }
      });
    return () => controller.abort();
  });
</script>

{@render children()}
