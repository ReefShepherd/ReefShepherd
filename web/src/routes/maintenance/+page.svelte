<script lang="ts">
  import OptionsProvider from '#lib/components/maintenance/OptionsProvider.svelte';
  import TaskBoard from '#lib/components/maintenance/TaskBoard.svelte';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import { m } from '#lib/paraglide/messages.js';
</script>

<PageTitle title={m.maintenance_title()} />

<div class="stack">
  <h1>{m.maintenance_title()}</h1>
  <OptionsProvider>
    <TaskBoard tankId={null} />
  </OptionsProvider>
</div>
