<script lang="ts">
  import { tankPath } from '#lib/api/tank-id.js';
  import ParameterChart from '#lib/components/charts/ParameterChart.svelte';
  import type { RangeId } from '#lib/components/charts/range.js';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import type { PageProps } from './$types';

  let { data }: PageProps = $props();
  const tankId = $derived(data.tankId);
  const parameterId = $derived(data.parameterId);

  let range = $state<RangeId>('90d');
  let loaded = $state<{ parameterId: number; name: string } | null>(null);

  // A name left over from the previous parameter must not show while the next one loads.
  const title = $derived(loaded?.parameterId === parameterId ? loaded.name : m.nav_charts());
</script>

<PageTitle {title} />
<p><a href={tankPath(tankId, 'charts')}>{m.charts_back()}</a></p>
<h1>{title}</h1>

{#key `${tankId}/${parameterId}`}
  <ParameterChart
    {tankId}
    {parameterId}
    bind:range
    onloaded={(name) => (loaded = { parameterId, name })}
  />
{/key}
