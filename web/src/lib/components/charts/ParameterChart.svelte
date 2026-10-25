<script lang="ts">
  import { chartsApi, type Series } from '#lib/api/charts.js';
  import { errorText } from '#lib/api/error-text.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import { formatDateTime } from '#lib/format/datetime.js';
  import { formatValue } from '#lib/format/number.js';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { session } from '#lib/session/session.svelte.js';
  import { flaggedPoints, outOfRange, toChartPoints } from './data.js';
  import { rangeIds, rangeStart, type RangeId } from './range.js';
  import { chart } from './uplot.js';

  interface Props {
    tankId: string;
    parameterId: string;
    range?: RangeId;
    /** Called with the parameter's name once its readings have loaded. */
    onloaded?: (name: string) => void;
  }

  let { tankId, parameterId, range = $bindable('90d'), onloaded }: Props = $props();

  let series = $state.raw<Series | null>(null);
  let failure = $state('');
  let loading = $state(true);
  let selected = $state<number | null>(null);
  let attempt = $state(0);

  const timeZone = $derived(session.timeZone);

  $effect(() => {
    void attempt;
    const from = rangeStart(range, new Date(), timeZone);
    const controller = new AbortController();
    loading = true;
    failure = '';
    chartsApi
      .series(tankId, parameterId, from, controller.signal)
      .then((loadedSeries) => {
        if (controller.signal.aborted) return;
        series = loadedSeries;
        selected = null;
        loading = false;
        onloaded?.(loadedSeries.name);
      })
      .catch((e: unknown) => {
        if (controller.signal.aborted) return;
        failure = errorText(e);
        loading = false;
      });
    return () => controller.abort();
  });

  const points = $derived(series ? toChartPoints(series.points) : []);
  const low = $derived(series?.range.low ?? null);
  const high = $derived(series?.range.high ?? null);
  const flagged = $derived(flaggedPoints(points, low, high));
  const chosen = $derived(selected === null ? null : (points[selected] ?? null));

  const showValue = (v: number) => formatValue(v, series?.unit_label, series?.decimals ?? 1);
  const showTime = (t: number) => formatDateTime(new Date(t * 1000).toISOString(), timeZone);

  const idealText = $derived.by(() => {
    if (!series) return '';
    if (low !== null && high !== null)
      return m.charts_ideal_between({ low: showValue(low), high: showValue(high) });
    if (low !== null) return m.charts_ideal_min({ low: showValue(low) });
    if (high !== null) return m.charts_ideal_max({ high: showValue(high) });
    return m.charts_ideal_none();
  });

  const params = $derived({
    points,
    low,
    high,
    selected,
    timeZone,
    locale: getLocale(),
    decimals: series?.decimals,
    formatValue: showValue,
    formatTime: showTime,
    onselect: (index: number | null) => (selected = index)
  });

  function rangeLabel(id: RangeId): string {
    switch (id) {
      case '30d':
        return m.charts_range_30d();
      case '90d':
        return m.charts_range_90d();
      case '1y':
        return m.charts_range_1y();
      case 'all':
        return m.charts_range_all();
    }
  }
</script>

<section class="chart-panel card" aria-busy={loading}>
  <div class="ranges" role="group" aria-label={m.charts_range_label()}>
    {#each rangeIds as id (id)}
      <button
        type="button"
        class="range"
        class:active={range === id}
        aria-pressed={range === id}
        onclick={() => (range = id)}
      >
        {rangeLabel(id)}
      </button>
    {/each}
  </div>

  {#if failure}
    <Alert kind="error">{failure}</Alert>
    <button type="button" class="btn" onclick={() => attempt++}>{m.charts_retry()}</button>
  {:else if !series}
    <p class="muted">{m.loading()}</p>
  {:else}
    <p class="hint">{idealText}</p>
    {#if points.length === 0}
      <p class="muted">{m.charts_no_readings()}</p>
    {:else}
      <div
        class="plot"
        class:stale={loading}
        role="img"
        aria-label={m.charts_plot_label({ name: series.name })}
        use:chart={params}
      ></div>

      <div class="detail" aria-live="polite">
        {#if chosen}
          <strong class:flagged={outOfRange(chosen.v, low, high)}>{showValue(chosen.v)}</strong>
          {#if outOfRange(chosen.v, low, high)}
            <span class="badge warning">{m.charts_out_of_range()}</span>
          {/if}
          <span>{showTime(chosen.t)}</span>
          {#if chosen.note}<span class="note">{chosen.note}</span>{/if}
        {:else}
          <span class="muted">{m.charts_tap_hint()}</span>
        {/if}
      </div>

      {#if low !== null || high !== null}
        <h3>{m.charts_flagged_heading()}</h3>
        {#if flagged.length === 0}
          <p class="muted">{m.charts_flagged_none()}</p>
        {:else}
          <ul class="flagged-list">
            {#each flagged as { index, point } (index)}
              <li>
                <button
                  type="button"
                  class="flagged-item"
                  aria-current={selected === index}
                  onclick={() => (selected = index)}
                >
                  <span class="sr-only">{m.charts_flagged_select()}</span>
                  <strong class="flagged">{showValue(point.v)}</strong>
                  <span>{showTime(point.t)}</span>
                  {#if point.note}<span class="note">{point.note}</span>{/if}
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      {/if}
    {/if}
  {/if}
</section>

<style>
  .ranges {
    display: flex;
    gap: var(--space-1);
    margin-bottom: var(--space-2);
  }

  .range {
    flex: 1;
    min-height: var(--touch);
    padding: 0 var(--space-2);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    color: var(--text);
    font: inherit;
    cursor: pointer;
  }

  .range.active {
    border-color: var(--accent);
    background: var(--accent);
    color: var(--accent-text);
    font-weight: 600;
  }

  .hint {
    margin: 0 0 var(--space-2);
    color: var(--text-muted);
  }

  .plot {
    width: 100%;
  }

  .plot.stale {
    opacity: 0.6;
  }

  .detail {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1) var(--space-3);
    min-height: var(--touch);
    margin-top: var(--space-2);
  }

  .note {
    flex-basis: 100%;
    color: var(--text-muted);
    overflow-wrap: anywhere;
  }

  .flagged {
    color: var(--danger);
  }

  .flagged-list {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .flagged-item {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1) var(--space-3);
    width: 100%;
    min-height: var(--touch);
    padding: var(--space-2) 0;
    border: 0;
    border-top: 1px solid var(--border);
    background: transparent;
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .flagged-item[aria-current='true'] {
    background: var(--surface-hover);
  }
</style>
