<script lang="ts">
  import { goto } from '$app/navigation';
  import Dashboard from '#lib/components/dashboard/Dashboard.svelte';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { currentTank } from '#lib/tank/current-tank.svelte.js';
  import { tankList } from '#lib/tank/tanks.svelte.js';

  // A fresh instance has no tank yet, so the first screen is the tank form.
  $effect(() => {
    tankList
      .load()
      .then((tanks) => {
        if (!tanks.some((t) => t.archived_at === null))
          void goto('/tanks/new', { replaceState: true });
      })
      .catch(() => {});
  });

  const tankName = $derived(tankList.active.find((t) => String(t.id) === currentTank.id)?.name);
</script>

<PageTitle title={m.nav_dashboard()} />
<h1>{m.nav_dashboard()}</h1>
{#if tankName}<p class="muted tank-name">{tankName}</p>{/if}

{#if currentTank.id !== null}
  {#key currentTank.id}
    <Dashboard tankId={currentTank.id} />
  {/key}
{:else}
  <p class="muted" role="status">{m.loading()}</p>
{/if}

<style>
  .tank-name {
    margin: calc(-1 * var(--space-2)) 0 var(--space-3);
  }
</style>
