<script lang="ts">
  import type { ChartPoint } from './data.js';
  import { chart } from './uplot.js';

  interface Props {
    /** Oldest first. */
    points: ChartPoint[];
    low: number | null;
    high: number | null;
    /** Accessible description; without one the sparkline is hidden from assistive technology. */
    label?: string;
  }

  let { points, low, high, label }: Props = $props();

  const params = $derived({ points, low, high, variant: 'spark' as const });
</script>

<div
  class="spark"
  role={label ? 'img' : undefined}
  aria-label={label}
  aria-hidden={label ? undefined : 'true'}
  use:chart={params}
></div>

<style>
  .spark {
    width: 100%;
    height: 60px;
    /* Taps go to the card link around the sparkline. */
    pointer-events: none;
  }
</style>
