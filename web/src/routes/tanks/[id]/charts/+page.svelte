<script lang="ts">
  import { errorText } from '#lib/api/error-text.js';
  import { paramsApi, type TankParameter } from '#lib/api/params.js';
  import { tankPath } from '#lib/api/tank-id.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import CardFrame from '#lib/components/dashboard/CardFrame.svelte';
  import { newCard } from '#lib/components/dashboard/cards.js';
  import { provideDashboard } from '#lib/components/dashboard/context.js';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import type { PageProps } from './$types';

  let { data }: PageProps = $props();
  const tankId = $derived(data.tankId);

  let parameters = $state.raw<TankParameter[] | null>(null);
  let failure = $state('');

  // The cards are the dashboard's own parameter cards, so both pages show a parameter the same way.
  provideDashboard({ tankId: () => tankId, parameters: () => parameters ?? [] });

  const cards = $derived(
    (parameters ?? [])
      .filter((p) => p.enabled)
      .map((p) =>
        newCard(
          {
            key: `parameter:${p.parameter_id}`,
            card_type: 'parameter',
            parameter_id: p.parameter_id
          },
          `p${p.parameter_id}`
        )
      )
  );

  $effect(() => {
    const controller = new AbortController();
    parameters = null;
    failure = '';
    paramsApi
      .tankParameters(tankId, controller.signal)
      .then((loaded) => {
        if (!controller.signal.aborted) parameters = loaded.parameters;
      })
      .catch((e: unknown) => {
        if (!controller.signal.aborted) failure = errorText(e);
      });
    return () => controller.abort();
  });
</script>

<PageTitle title={m.nav_charts()} />
<h1>{m.nav_charts()}</h1>

{#if failure}
  <Alert kind="error">{failure}</Alert>
{:else if parameters === null}
  <p class="muted">{m.loading()}</p>
{:else if cards.length === 0}
  <p class="muted">{m.charts_none_enabled()}</p>
  <p><a href={tankPath(tankId, 'settings')}>{m.charts_none_enabled_link()}</a></p>
{:else}
  <p class="muted">{m.charts_intro()}</p>
  <ul class="grid">
    {#each cards as card (card.id)}
      <li><CardFrame {card} /></li>
    {/each}
  </ul>
{/if}

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(16rem, 100%), 1fr));
    gap: var(--space-3);
    margin: var(--space-3) 0 0;
    padding: 0;
    list-style: none;
  }
</style>
