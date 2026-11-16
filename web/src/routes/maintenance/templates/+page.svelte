<script lang="ts">
  import { errorText } from '#lib/api/error-text.js';
  import { maintenanceApi, type Preset } from '#lib/api/maintenance.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import PresetForm from '#lib/components/maintenance/PresetForm.svelte';
  import PageTitle from '#lib/components/shell/PageTitle.svelte';
  import { guarded } from '#lib/forms/guarded.js';
  import { isOwnPreset, replacePreset } from '#lib/maintenance/presets.js';
  import { m } from '#lib/paraglide/messages.js';
  import { describeSchedule } from '#lib/schedule/describe.js';
  import { defaultDueTime } from '#lib/schedule/schedule.js';

  let presets = $state.raw<Preset[] | null>(null);
  let dueTime = $state(defaultDueTime);
  let failure = $state('');
  let actionFailure = $state('');
  let adding = $state(false);
  let editingId = $state<number | null>(null);
  let attempt = $state(0);

  $effect(() => {
    void attempt;
    const controller = new AbortController();
    presets = null;
    failure = '';
    Promise.all([
      maintenanceApi.presets(true, controller.signal),
      maintenanceApi.defaults(controller.signal)
    ])
      .then(([list, defaults]) => {
        if (controller.signal.aborted) return;
        presets = list;
        dueTime = defaults.due_time;
      })
      .catch((e: unknown) => {
        if (!controller.signal.aborted) failure = errorText(e);
      });
    return () => controller.abort();
  });

  function saved(preset: Preset) {
    presets = replacePreset(presets ?? [], preset);
    adding = false;
    editingId = null;
  }

  const setHidden = (preset: Preset, hidden: boolean) =>
    guarded(async () => saved(await maintenanceApi.updatePreset(preset.id, { archived: hidden })), {
      failure: (text) => (actionFailure = text)
    });
</script>

<PageTitle title={m.maintenance_templates_title()} />

<div class="stack">
  <h1>{m.maintenance_templates_title()}</h1>
  <p><a href="/maintenance">{m.maintenance_back()}</a></p>
  <p class="muted">{m.maintenance_templates_intro()}</p>

  {#if failure}
    <Alert kind="error">{failure}</Alert>
    <div>
      <button type="button" class="btn" onclick={() => attempt++}>{m.maintenance_retry()}</button>
    </div>
  {:else if presets === null}
    <p class="muted" role="status">{m.loading()}</p>
  {:else}
    {#if actionFailure}
      <Alert kind="error">{actionFailure}</Alert>
    {/if}

    {#if adding}
      <PresetForm {dueTime} onsaved={saved} oncancel={() => (adding = false)} />
    {:else}
      <div>
        <button type="button" class="btn primary" onclick={() => (adding = true)}>
          {m.maintenance_template_add()}
        </button>
      </div>
    {/if}

    {#if presets.length === 0}
      <p class="muted">{m.maintenance_templates_empty()}</p>
    {:else}
      <ul class="list">
        {#each presets as preset (preset.id)}
          <li>
            {#if editingId === preset.id}
              <PresetForm {preset} {dueTime} onsaved={saved} oncancel={() => (editingId = null)} />
            {:else}
              <div class="card item" class:hidden={preset.archived_at !== null}>
                <div class="what">
                  <strong class="name">{preset.name}</strong>
                  {#if !isOwnPreset(preset)}
                    <span class="badge">{m.maintenance_template_shipped()}</span>
                  {/if}
                  {#if preset.tankless}
                    <span class="badge">{m.maintenance_whole_system()}</span>
                  {/if}
                  {#if preset.archived_at !== null}
                    <span class="badge warning">{m.maintenance_template_hidden()}</span>
                  {/if}
                </div>
                {#if preset.description}<p class="description">{preset.description}</p>{/if}
                <p class="muted">{describeSchedule(preset.schedule)}</p>
                <div class="row">
                  {#if isOwnPreset(preset)}
                    <button
                      type="button"
                      class="btn"
                      onclick={() => {
                        actionFailure = '';
                        editingId = preset.id;
                      }}
                    >
                      {m.form_edit()}<span class="sr-only"> {preset.name}</span>
                    </button>
                  {/if}
                  <button
                    type="button"
                    class="btn"
                    onclick={() => setHidden(preset, preset.archived_at === null)}
                  >
                    {preset.archived_at === null
                      ? m.maintenance_template_hide()
                      : m.maintenance_template_show()}<span class="sr-only"> {preset.name}</span>
                  </button>
                </div>
              </div>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
</div>

<style>
  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .item {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
  }

  .item.hidden {
    opacity: 0.8;
  }

  .item p {
    margin: 0;
  }

  .what {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--space-2);
  }

  .name {
    overflow-wrap: anywhere;
  }

  .description {
    overflow-wrap: anywhere;
  }
</style>
