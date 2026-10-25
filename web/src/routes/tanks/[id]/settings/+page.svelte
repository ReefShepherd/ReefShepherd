<script lang="ts">
  import { goto } from '$app/navigation';
  import type { PageProps } from './$types';
  import { errorText } from '#lib/api/error-text.js';
  import { tanksApi, type Tank } from '#lib/api/tanks.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import ConfirmButton from '#lib/components/forms/ConfirmButton.svelte';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import DeleteTank from '#lib/components/tanks/DeleteTank.svelte';
  import ParameterSettings from '#lib/components/tanks/ParameterSettings.svelte';
  import TankForm from '#lib/components/tanks/TankForm.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';
  import { volumeUnitLabel } from '#lib/tank/form.js';
  import { tankList } from '#lib/tank/tanks.svelte.js';
  import { unitCatalog } from '#lib/units/cache.svelte.js';

  let { data }: PageProps = $props();
  const id = $derived(data.tankId);

  let tank = $state<Tank | null>(null);
  let failure = $state('');
  let notice = $state('');

  $effect(() => {
    const wanted = id;
    tank = null;
    failure = '';
    notice = '';
    unitCatalog.load().catch(() => {});
    tanksApi
      .get(wanted)
      .then((loaded) => {
        if (wanted === id) tank = loaded;
      })
      .catch((e: unknown) => (failure = errorText(e)));
  });

  async function saved(next: Tank) {
    tank = next;
    notice = m.tanks_saved();
    await tankList.refresh().catch(() => {});
  }

  async function setArchived(archive: boolean) {
    if (!tank) return;
    failure = '';
    try {
      const next = await (archive ? tanksApi.archive(tank.id) : tanksApi.unarchive(tank.id));
      tank = next;
      await tankList.refresh().catch(() => {});
      if (archive) await goto('/tanks');
      else notice = m.tanks_restored();
    } catch (e) {
      failure = errorText(e);
    }
  }

  async function deleted() {
    await tankList.refresh().catch(() => {});
    await goto('/tanks');
  }
</script>

<PageTitle title={tank ? m.tanks_settings_title({ name: tank.name }) : m.tanks_settings()} />

{#if failure}
  <Alert kind="error">{failure}</Alert>
  <p><a href="/tanks">{m.tanks_back_to_list()}</a></p>
{/if}

{#if tank}
  {#key tank.id}
    <div class="stack">
      <h1>{m.tanks_settings_title({ name: tank.name })}</h1>
      {#if notice}
        <Alert kind="success">{notice}</Alert>
      {/if}
      {#if tank.archived_at}
        <Alert kind="warning">
          <p>{m.tanks_archived_note()}</p>
          <button type="button" class="btn" onclick={() => setArchived(false)}>
            {m.tanks_restore()}
          </button>
        </Alert>
      {/if}

      <TankForm
        {tank}
        volumeUnit={volumeUnitLabel(unitCatalog.families, tank.volume_unit)}
        onsaved={saved}
      />

      <ParameterSettings tankId={tank.id} />

      {#if !tank.archived_at}
        <section class="card stack" aria-labelledby="archive-heading">
          <h2 id="archive-heading">{m.tanks_archive_title()}</h2>
          <p>{m.tanks_archive_help()}</p>
          <div>
            <ConfirmButton
              label={m.tanks_archive()}
              question={m.tanks_archive_question({ name: tank.name })}
              confirmLabel={m.tanks_archive()}
              onconfirm={() => setArchived(true)}
            />
          </div>
        </section>
      {/if}

      {#if session.isAdmin}
        <DeleteTank {tank} ondeleted={deleted} />
      {/if}
    </div>
  {/key}
{:else if !failure}
  <p class="muted" role="status">{m.loading()}</p>
{/if}
