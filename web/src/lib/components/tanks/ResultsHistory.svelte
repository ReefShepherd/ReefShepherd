<script lang="ts">
  import { untrack } from 'svelte';
  import { errorText } from '#lib/api/error-text.js';
  import { paramsApi, type TankParameter, type TestResult } from '#lib/api/params.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import SelectField from '#lib/components/forms/SelectField.svelte';
  import { fractionDigits } from '#lib/format/decimal.js';
  import { m } from '#lib/paraglide/messages.js';
  import {
    addResults,
    appendPage,
    removeResult,
    replaceResult,
    resultQuery
  } from '#lib/results/paging.js';
  import ResultRow from './ResultRow.svelte';

  interface Props {
    tankId: number;
    parameters: readonly TankParameter[];
    timeZone: string;
  }

  let { tankId, parameters, timeZone }: Props = $props();

  const allParameters = 'all';

  let filter = $state(allParameters);
  let results = $state<TestResult[] | null>(null);
  let nextCursor = $state<string | null>(null);
  let failure = $state('');
  let loadingMore = $state(false);
  // A reply for an older filter is dropped, so a slow request cannot overwrite a newer list.
  let generation = 0;

  const parameterId = $derived(filter === allParameters ? null : Number(filter));
  const decimals = $derived(new Map(parameters.map((p) => [p.parameter_id, p.decimals])));

  async function load(id: number | null) {
    const mine = ++generation;
    failure = '';
    results = null;
    nextCursor = null;
    try {
      const page = await paramsApi.results(tankId, resultQuery(id, null));
      if (mine !== generation) return;
      results = page.results;
      nextCursor = page.next_cursor;
    } catch (e) {
      if (mine !== generation) return;
      failure = errorText(e);
      results = [];
    }
  }

  async function loadMore() {
    if (nextCursor === null || results === null) return;
    const mine = generation;
    loadingMore = true;
    failure = '';
    try {
      const page = await paramsApi.results(tankId, resultQuery(parameterId, nextCursor));
      if (mine !== generation) return;
      results = appendPage(results, page);
      nextCursor = page.next_cursor;
    } catch (e) {
      if (mine === generation) failure = errorText(e);
    } finally {
      loadingMore = false;
    }
  }

  // Changing the filter starts a new list from the newest result.
  $effect(() => {
    const id = parameterId;
    untrack(() => void load(id));
  });

  /** Shows results that were just logged at the top of the list. */
  export function add(added: TestResult[]): void {
    if (results !== null) results = addResults(results, added, parameterId);
  }
</script>

<section class="stack" aria-labelledby="history-heading">
  <h2 id="history-heading">{m.tests_history_title()}</h2>

  <SelectField
    label={m.tests_filter()}
    bind:value={filter}
    options={[
      { value: allParameters, label: m.tests_filter_all() },
      ...parameters.map((p) => ({ value: String(p.parameter_id), label: p.name }))
    ]}
  />

  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}

  {#if results === null}
    <p class="muted" role="status">{m.loading()}</p>
  {:else if results.length === 0}
    {#if !failure}<p class="muted">{m.tests_history_empty()}</p>{/if}
  {:else}
    <ul class="list">
      {#each results as result (result.id)}
        <ResultRow
          {result}
          decimals={decimals.get(result.parameter_id) ?? fractionDigits(result.value)}
          {timeZone}
          onupdated={(saved) => (results = replaceResult(results ?? [], saved))}
          ondeleted={(id) => (results = removeResult(results ?? [], id))}
        />
      {/each}
    </ul>
    {#if nextCursor !== null}
      <div>
        <button type="button" class="btn" disabled={loadingMore} onclick={loadMore}>
          {m.tests_load_more()}
        </button>
      </div>
    {/if}
  {/if}
</section>

<style>
  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }
</style>
