<script lang="ts">
  import { errorText } from '#lib/api/error-text.js';
  import { paramsApi, type TankParameters, type TestResult } from '#lib/api/params.js';
  import { tankPath } from '#lib/api/tank-id.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import ResultsHistory from '#lib/components/tanks/ResultsHistory.svelte';
  import TestEntryForm from '#lib/components/tanks/TestEntryForm.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { useOpenTank } from '#lib/tank/open-tank.js';
  import { orderParameters } from '#lib/tank/parameter-settings.js';
  import type { PageProps } from './$types';

  let { data }: PageProps = $props();
  const id = $derived(data.tankId);

  const open = useOpenTank();
  const tankName = $derived(open.tank && open.tank.id === id ? open.tank.name : undefined);

  let loaded = $state<TankParameters | null>(null);
  let failure = $state('');
  let history = $state<ResultsHistory>();

  $effect(() => {
    const wanted = id;
    loaded = null;
    failure = '';
    paramsApi
      .tankParameters(wanted)
      .then((p) => {
        if (wanted === id) loaded = p;
      })
      .catch((e: unknown) => (failure = errorText(e)));
  });

  const enabled = $derived(orderParameters((loaded?.parameters ?? []).filter((p) => p.enabled)));

  function saved(results: TestResult[]) {
    history?.add(results);
    // The last-tested hints in the form come from the parameter list, so it is read again.
    const wanted = id;
    paramsApi
      .tankParameters(wanted)
      .then((p) => {
        if (wanted === id) loaded = p;
      })
      .catch(() => {});
  }
</script>

<PageTitle title={m.nav_tests()} />

<div class="stack">
  <h1>{tankName ? m.detail_pair({ first: m.nav_tests(), second: tankName }) : m.nav_tests()}</h1>

  {#if failure}
    <Alert kind="error">{failure}</Alert>
    <p><a href="/tanks">{m.tanks_back_to_list()}</a></p>
  {/if}

  {#if loaded}
    {#key loaded.tank_id}
      {#if enabled.length === 0}
        <Alert kind="warning">
          {m.tests_no_parameters()}
          <a href={tankPath(loaded.tank_id, 'settings')}>{m.tests_choose_parameters()}</a>
        </Alert>
      {:else}
        <TestEntryForm
          tankId={loaded.tank_id}
          parameters={enabled}
          timeZone={loaded.timezone}
          onsaved={saved}
        />
      {/if}
      <ResultsHistory
        bind:this={history}
        tankId={loaded.tank_id}
        parameters={loaded.parameters}
        timeZone={loaded.timezone}
      />
    {/key}
  {:else if !failure}
    <p class="muted" role="status">{m.loading()}</p>
  {/if}
</div>
