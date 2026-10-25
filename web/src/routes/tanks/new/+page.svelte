<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { errorText } from '#lib/api/error-text.js';
  import { paramsApi, type ParameterDefinition } from '#lib/api/params.js';
  import type { Tank } from '#lib/api/tanks.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import TankForm from '#lib/components/tanks/TankForm.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';
  import { currentTank } from '#lib/tank/current-tank.svelte.js';
  import { volumeUnitLabel } from '#lib/tank/form.js';
  import { tankList } from '#lib/tank/tanks.svelte.js';
  import { unitCatalog } from '#lib/units/cache.svelte.js';

  let presets = $state<ParameterDefinition[] | null>(null);
  let failure = $state('');

  const first = $derived(tankList.tanks !== null && tankList.active.length === 0);
  const volumeUnit = $derived(
    volumeUnitLabel(unitCatalog.families, session.me?.instance.units.volume ?? '')
  );

  onMount(() => {
    Promise.all([paramsApi.definitions(), unitCatalog.load(), tankList.load()])
      .then(([definitions]) => (presets = definitions.filter((d) => d.preset_key !== null)))
      .catch((e: unknown) => (failure = errorText(e)));
  });

  async function saved(tank: Tank) {
    currentTank.set(tank.id);
    await tankList.refresh().catch(() => {});
    currentTank.set(tank.id);
    await goto('/');
  }
</script>

<PageTitle title={m.tanks_new_title()} />

<div class="narrow">
  <h1>{first ? m.tanks_first_title() : m.tanks_new_title()}</h1>
  {#if first}
    <p class="muted">{m.tanks_first_intro()}</p>
  {/if}

  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}

  {#if presets !== null}
    <TankForm
      {presets}
      {volumeUnit}
      onsaved={saved}
      oncancel={first ? undefined : () => goto('/tanks')}
    />
  {:else if !failure}
    <p class="muted" role="status">{m.loading()}</p>
  {/if}
</div>
