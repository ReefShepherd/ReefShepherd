<script lang="ts">
  import { dashboardApi, type Layout } from '#lib/api/dashboard.js';
  import { errorText } from '#lib/api/error-text.js';
  import { paramsApi, type TankParameter } from '#lib/api/params.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import ConfirmButton from '#lib/components/forms/ConfirmButton.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import CardFrame from './CardFrame.svelte';
  import { fromLayout, toBody, type Card } from './cards.js';
  import { provideDashboard } from './context.js';
  import './grid.css';

  let { tankId }: { tankId: string } = $props();

  let saved = $state.raw<Card[]>([]);
  let parameters = $state.raw<TankParameter[]>([]);
  let loading = $state(true);
  let failure = $state('');
  let editing = $state(false);
  let actionFailure = $state('');
  let attempt = $state(0);

  provideDashboard({ tankId: () => tankId, parameters: () => parameters });

  $effect(() => {
    void attempt;
    const id = tankId;
    const controller = new AbortController();
    loading = true;
    failure = '';
    Promise.all([
      dashboardApi.get(id, controller.signal),
      paramsApi.tankParameters(id, controller.signal)
    ])
      .then(([layout, tankParameters]) => {
        if (controller.signal.aborted) return;
        saved = fromLayout(layout.cards);
        parameters = tankParameters.parameters;
        loading = false;
      })
      .catch((e: unknown) => {
        if (controller.signal.aborted) return;
        failure = errorText(e);
        loading = false;
      });
    return () => controller.abort();
  });

  function adopt(layout: Layout) {
    saved = fromLayout(layout.cards);
    editing = false;
  }

  const save = async (cards: Card[]) => adopt(await dashboardApi.save(tankId, toBody(cards)));
  const reset = async () => adopt(await dashboardApi.reset(tankId));

  async function resetFromEmpty() {
    actionFailure = '';
    try {
      await reset();
    } catch (e) {
      actionFailure = errorText(e);
    }
  }
</script>

<section aria-busy={loading}>
  {#if failure}
    <Alert kind="error">{failure}</Alert>
    <button type="button" class="btn" onclick={() => attempt++}>{m.charts_retry()}</button>
  {:else if loading}
    <p class="muted" role="status">{m.loading()}</p>
  {:else if editing}
    <!-- The drag library lives in this chunk, so viewing the dashboard never downloads it. -->
    {#await import('./EditMode.svelte')}
      <p class="muted" role="status">{m.loading()}</p>
    {:then { default: EditMode }}
      <EditMode cards={saved} onsave={save} onreset={reset} oncancel={() => (editing = false)} />
    {:catch e}
      <Alert kind="error">{errorText(e)}</Alert>
      <button type="button" class="btn" onclick={() => (editing = false)}>
        {m.form_cancel()}
      </button>
    {/await}
  {:else}
    {#if saved.length > 0}
      <div class="toolbar">
        <button type="button" class="btn" onclick={() => (editing = true)}>
          {m.dashboard_edit()}
        </button>
      </div>
    {/if}
    {#if saved.length === 0}
      <p>{m.dashboard_empty()}</p>
      <p class="muted">{m.dashboard_empty_hint()}</p>
      {#if actionFailure}
        <Alert kind="error">{actionFailure}</Alert>
      {/if}
      <div class="row">
        <button type="button" class="btn primary" onclick={() => (editing = true)}>
          {m.dashboard_add_label()}
        </button>
        <ConfirmButton
          label={m.dashboard_reset()}
          question={m.dashboard_reset_question()}
          confirmLabel={m.dashboard_reset_confirm()}
          onconfirm={resetFromEmpty}
        />
      </div>
    {:else}
      <div class="dash-grid" role="list" aria-label={m.dashboard_cards_label()}>
        {#each saved as card (card.id)}
          <div class="dash-slot" class:wide={card.size === 'wide'} role="listitem">
            <CardFrame {card} />
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</section>

<style>
  .toolbar {
    margin-bottom: var(--space-3);
  }
</style>
