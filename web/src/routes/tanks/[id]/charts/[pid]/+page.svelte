<script lang="ts">
  import { page } from '$app/state';
  import ParameterChart from '#lib/components/charts/ParameterChart.svelte';
  import type { RangeId } from '#lib/components/charts/range.js';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import { m } from '#lib/paraglide/messages.js';

  const tankId = $derived(page.params.id ?? '');
  const parameterId = $derived(page.params.pid ?? '');

  let range = $state<RangeId>('90d');
  let loaded = $state<{ parameterId: string; name: string } | null>(null);

  // A name left over from the previous parameter must not show while the next one loads.
  const title = $derived(loaded?.parameterId === parameterId ? loaded.name : m.nav_charts());
</script>

<PageTitle {title} />
<p><a href="/tanks/{encodeURIComponent(tankId)}/charts">{m.charts_back()}</a></p>
<h1>{title}</h1>

{#key `${tankId}/${parameterId}`}
  <ParameterChart
    {tankId}
    {parameterId}
    bind:range
    onloaded={(name) => (loaded = { parameterId, name })}
  />
{/key}
