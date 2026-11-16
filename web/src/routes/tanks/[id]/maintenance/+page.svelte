<script lang="ts">
  import OptionsProvider from '#lib/components/maintenance/OptionsProvider.svelte';
  import TaskBoard from '#lib/components/maintenance/TaskBoard.svelte';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { useOpenTank } from '#lib/tank/open-tank.js';
  import type { PageProps } from './$types';

  let { data }: PageProps = $props();
  const id = $derived(data.tankId);

  const open = useOpenTank();
  const tankName = $derived(open.tank && open.tank.id === id ? open.tank.name : undefined);
</script>

<PageTitle title={m.maintenance_title()} />

<div class="stack">
  <h1>
    {tankName
      ? m.detail_pair({ first: m.maintenance_title(), second: tankName })
      : m.maintenance_title()}
  </h1>
  <OptionsProvider>
    {#key id}
      <TaskBoard tankId={id} />
    {/key}
  </OptionsProvider>
</div>
