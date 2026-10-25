<script module lang="ts">
  import { m } from '#lib/paraglide/messages.js';

  export const cardType = 'parameter';
  export const needsParameter = true;
  export const cardLabel = (parameterName: string | null) =>
    parameterName ?? m.dashboard_parameter_unknown();
</script>

<script lang="ts">
  import { chartsApi } from '#lib/api/charts.js';
  import { tankPath } from '#lib/api/tank-id.js';
  import { sparkPoints } from '#lib/components/charts/data.js';
  import { rangeStart } from '#lib/components/charts/range.js';
  import Sparkline from '#lib/components/charts/Sparkline.svelte';
  import { relativeTime } from '#lib/format/datetime.js';
  import { formatValue } from '#lib/format/number.js';
  import type { Card } from '../cards.js';
  import { useDashboard } from '../context.js';
  import { rangeState } from '../status.js';

  let { card }: { card: Card } = $props();

  const dash = useDashboard();
  const parameter = $derived(dash.parameter(card.parameter_id));
  const latest = $derived(parameter?.latest ?? null);
  const level = $derived(parameter ? rangeState(parameter) : 'none');

  let points = $state.raw<[number, number][]>([]);

  // The trend is decoration, so a history that fails to load just leaves the card without it.
  $effect(() => {
    void latest?.result_id;
    const id = card.parameter_id;
    if (id === null || latest === null) {
      points = [];
      return;
    }
    const controller = new AbortController();
    chartsApi
      .series(
        dash.tankId,
        String(id),
        rangeStart('90d', new Date(), dash.timeZone),
        controller.signal
      )
      .then((series) => {
        if (!controller.signal.aborted) points = sparkPoints(series.points);
      })
      .catch(() => {});
    return () => controller.abort();
  });
</script>

{#if !parameter}
  <p class="muted">{m.dashboard_parameter_missing()}</p>
{:else}
  <div class="param">
    <div class="head">
      <h3>
        <a href={tankPath(dash.tankId, 'charts', parameter.parameter_id)}>
          {parameter.name}
        </a>
      </h3>
      {#if parameter.overdue}
        <span class="badge warning">{m.dashboard_overdue()}</span>
      {/if}
    </div>
    {#if latest}
      <p class="value {level}">
        {formatValue(latest.value, parameter.unit_label, parameter.decimals)}
      </p>
      {#if level !== 'none'}
        <p class="status {level}">
          <span aria-hidden="true">{level === 'in' ? '✓' : '⚠'}</span>
          {level === 'in' ? m.dashboard_in_range() : m.charts_out_of_range()}
        </p>
      {/if}
      <p class="muted when">{m.charts_card_tested({ when: relativeTime(latest.measured_at) })}</p>
      {#if points.length > 0}
        <Sparkline
          {points}
          low={parameter.ideal_low}
          high={parameter.ideal_high}
          label={m.charts_plot_label({ name: parameter.name })}
        />
      {/if}
    {:else}
      <p class="muted">{m.charts_card_no_tests()}</p>
    {/if}
  </div>
{/if}

<style>
  .param {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-2);
  }

  h3 {
    margin: 0;
    overflow-wrap: anywhere;
  }

  p {
    margin: 0;
  }

  .value {
    font-size: 1.75rem;
    font-weight: 600;
    line-height: 1.2;
  }

  .in {
    color: var(--success);
  }

  .out {
    color: var(--danger);
  }

  .status {
    font-weight: 600;
  }

  .when {
    font-size: 0.875rem;
  }
</style>
