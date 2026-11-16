<script lang="ts">
  import type { Snippet } from 'svelte';
  import { errorText } from '#lib/api/error-text.js';
  import { maintenanceApi, type Preset } from '#lib/api/maintenance.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import { provideOptions } from '#lib/maintenance/options.js';
  import { m } from '#lib/paraglide/messages.js';
  import { defaultDueTime } from '#lib/schedule/schedule.js';
  import { tankList } from '#lib/tank/tanks.svelte.js';

  let { children }: { children: Snippet } = $props();

  let presets = $state.raw<Preset[]>([]);
  let dueTime = $state(defaultDueTime);
  let ready = $state(false);
  let failure = $state('');
  let attempt = $state(0);

  provideOptions({
    tanks: () => tankList.active,
    presets: () => presets,
    dueTime: () => dueTime
  });

  // The forms below read the tanks, templates and reminder time, so nothing is shown until all three have arrived.
  $effect(() => {
    void attempt;
    const controller = new AbortController();
    ready = false;
    failure = '';
    Promise.all([
      tankList.load(),
      maintenanceApi.presets(false, controller.signal),
      maintenanceApi.defaults(controller.signal)
    ])
      .then(([, list, defaults]) => {
        if (controller.signal.aborted) return;
        presets = list;
        dueTime = defaults.due_time;
        ready = true;
      })
      .catch((e: unknown) => {
        if (controller.signal.aborted) return;
        failure = errorText(e);
      });
    return () => controller.abort();
  });
</script>

{#if failure}
  <Alert kind="error">{failure}</Alert>
  <button type="button" class="btn" onclick={() => attempt++}>{m.maintenance_retry()}</button>
{:else if !ready}
  <p class="muted" role="status">{m.loading()}</p>
{:else}
  {@render children()}
{/if}
