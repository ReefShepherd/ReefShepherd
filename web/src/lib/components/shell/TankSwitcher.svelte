<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { m } from '#lib/paraglide/messages.js';
  import { currentTank } from '#lib/tank/current-tank.svelte.js';
  import { retargetPath } from '#lib/tank/selection.js';
  import { tankList } from '#lib/tank/tanks.svelte.js';

  // Failures are left to the pages, which show their own errors.
  $effect(() => {
    void tankList.load().catch(() => {});
  });

  const loaded = $derived(tankList.tanks !== null);
  const active = $derived(tankList.active);

  function choose(event: Event) {
    const id = (event.currentTarget as HTMLSelectElement).value;
    currentTank.set(id);
    // A page that belongs to one tank follows the switch instead of showing the old tank.
    const target = retargetPath(page.url.pathname, id);
    if (target !== null) void goto(target);
  }
</script>

{#if loaded && active.length === 0}
  <a class="btn create" href="/tanks/new">{m.tank_switcher_create()}</a>
{:else}
  <label class="sr-only" for="tank-switcher">{m.tank_switcher_label()}</label>
  <select id="tank-switcher" disabled={!loaded} value={currentTank.id ?? ''} onchange={choose}>
    {#if !loaded}
      <option value="">{m.loading()}</option>
    {:else}
      {#each active as tank (tank.id)}
        <option value={String(tank.id)}>{tank.name}</option>
      {/each}
    {/if}
  </select>
{/if}

<style>
  .create {
    min-width: 0;
    padding: 0 var(--space-2);
    font-size: 0.875rem;
    text-align: center;
  }
</style>
