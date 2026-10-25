<script lang="ts">
  import { onMount } from 'svelte';
  import { errorText } from '#lib/api/error-text.js';
  import { tanksApi, type Tank } from '#lib/api/tanks.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import ConfirmButton from '#lib/components/forms/ConfirmButton.svelte';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import { tankTypeLabel } from '#lib/components/tanks/labels.js';
  import { formatNumber } from '#lib/format/number.js';
  import { m } from '#lib/paraglide/messages.js';
  import { volumeUnitLabel } from '#lib/tank/form.js';
  import { tankList } from '#lib/tank/tanks.svelte.js';
  import { unitCatalog } from '#lib/units/cache.svelte.js';

  let failure = $state('');

  onMount(() => {
    tankList.refresh().catch((e: unknown) => (failure = errorText(e)));
    unitCatalog.load().catch(() => {});
  });

  const tanks = $derived(tankList.tanks);
  const active = $derived((tanks ?? []).filter((t) => t.archived_at === null));
  const archived = $derived((tanks ?? []).filter((t) => t.archived_at !== null));

  async function change(action: () => Promise<Tank>) {
    failure = '';
    try {
      await action();
      await tankList.refresh();
    } catch (e) {
      failure = errorText(e);
    }
  }

  const volume = (t: Tank) =>
    t.volume === null
      ? null
      : `${formatNumber(t.volume, 1)} ${volumeUnitLabel(unitCatalog.families, t.volume_unit)}`;

  function details(t: Tank): string {
    const first = volume(t);
    const second = t.started_on ? m.tanks_started({ date: t.started_on }) : null;
    if (first && second) return m.detail_pair({ first, second });
    return first ?? second ?? m.tanks_no_details();
  }
</script>

<PageTitle title={m.nav_tanks()} />

<div class="stack">
  <div class="row head">
    <h1>{m.nav_tanks()}</h1>
    <div class="row">
      <a class="btn primary" href="/tanks/new">{m.tanks_add()}</a>
      <a class="btn" href="/tanks/parameters">{m.params_definitions_link()}</a>
    </div>
  </div>

  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}

  {#if tanks === null}
    {#if !failure}<p class="muted" role="status">{m.loading()}</p>{/if}
  {:else}
    {#if active.length === 0}
      <p class="muted">{m.tanks_none()}</p>
    {/if}
    <ul class="list">
      {#each active as tank (tank.id)}
        {@render item(tank)}
      {/each}
    </ul>

    {#if archived.length > 0}
      <section class="stack" aria-labelledby="archived-heading">
        <h2 id="archived-heading">{m.tanks_archived_title()}</h2>
        <ul class="list">
          {#each archived as tank (tank.id)}
            {@render item(tank)}
          {/each}
        </ul>
      </section>
    {/if}
  {/if}
</div>

{#snippet item(tank: Tank)}
  <li class="card tank">
    <div class="row">
      <h2 class="name">{tank.name}</h2>
      <span class="badge accent">{tankTypeLabel(tank.tank_type)}</span>
      {#if tank.archived_at}<span class="badge warning">{m.tanks_badge_archived()}</span>{/if}
    </div>
    <p class="hint">{details(tank)}</p>
    <div class="row">
      {#if !tank.archived_at}
        <a class="btn" href="/tanks/{tank.id}/tests">{m.nav_tests()}</a>
        <a class="btn" href="/tanks/{tank.id}/charts">{m.nav_charts()}</a>
      {/if}
      <a class="btn" href="/tanks/{tank.id}/settings">
        {m.tanks_settings()}<span class="sr-only"> {tank.name}</span>
      </a>
      {#if tank.archived_at}
        <button type="button" class="btn" onclick={() => change(() => tanksApi.unarchive(tank.id))}>
          {m.tanks_restore()}<span class="sr-only"> {tank.name}</span>
        </button>
      {:else}
        <ConfirmButton
          label={m.tanks_archive()}
          question={m.tanks_archive_question({ name: tank.name })}
          confirmLabel={m.tanks_archive()}
          onconfirm={() => change(() => tanksApi.archive(tank.id))}
        />
      {/if}
    </div>
  </li>
{/snippet}

<style>
  .head {
    justify-content: space-between;
  }

  .head h1 {
    margin: 0;
  }

  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .tank {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
  }

  .name {
    margin: 0;
    overflow-wrap: anywhere;
  }

  .tank p {
    margin: 0;
  }
</style>
