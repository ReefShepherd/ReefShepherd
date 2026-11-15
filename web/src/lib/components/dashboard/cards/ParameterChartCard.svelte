<script module lang="ts">
  import { m } from '#lib/paraglide/messages.js';

  export const cardType = 'parameter_chart';
  export const needsParameter = true;
  export const cardLabel = (parameterName: string | null) =>
    m.dashboard_chart_label({ name: parameterName ?? m.dashboard_parameter_unknown() });
</script>

<script lang="ts">
  import { rangeIds } from '#lib/components/charts/range.js';
  import ParameterChart from '#lib/components/charts/ParameterChart.svelte';
  import type { Card } from '../cards.js';
  import { useDashboard } from '../context.js';

  let { card }: { card: Card } = $props();

  const dash = useDashboard();
  const parameter = $derived(dash.parameter(card.parameter_id));
  // The range buttons only change what this card shows right now; the saved range is the starting one.
  let range = $derived(rangeIds.find((id) => id === card.options.range) ?? '90d');
</script>

{#if !parameter}
  <p class="muted">{m.dashboard_parameter_missing()}</p>
{:else}
  <div class="chart-card">
    <h3>{parameter.name}</h3>
    <ParameterChart tankId={dash.tankId} parameterId={parameter.parameter_id} bind:range />
  </div>
{/if}

<style>
  h3 {
    overflow-wrap: anywhere;
  }

  /* The frame already draws the card, so the chart panel inside it drops its own box. */
  .chart-card :global(.chart-panel) {
    margin: 0;
    padding: 0;
    border: 0;
    background: none;
  }
</style>
