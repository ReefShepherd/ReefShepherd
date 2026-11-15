<script lang="ts">
  import { onMount } from 'svelte';
  import { errorText } from '#lib/api/error-text.js';
  import { isApiError } from '#lib/api/client.js';
  import { paramsApi, type ParameterDefinition } from '#lib/api/params.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import ConfirmButton from '#lib/components/forms/ConfirmButton.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { rangeText } from '#lib/tank/range.js';
  import DefinitionForm from './DefinitionForm.svelte';

  let definitions = $state<ParameterDefinition[] | null>(null);
  let failure = $state('');
  let notice = $state('');
  let adding = $state(false);
  let editingId = $state<number | null>(null);

  const custom = $derived((definitions ?? []).filter((d) => d.preset_key === null));
  const presets = $derived((definitions ?? []).filter((d) => d.preset_key !== null));

  onMount(() => {
    paramsApi
      .definitions(true)
      .then((list) => (definitions = list))
      .catch((e: unknown) => (failure = errorText(e)));
  });

  function replace(saved: ParameterDefinition) {
    const known = (definitions ?? []).some((d) => d.id === saved.id);
    definitions = known
      ? (definitions ?? []).map((d) => (d.id === saved.id ? saved : d))
      : [...(definitions ?? []), saved];
  }

  function saved(definition: ParameterDefinition) {
    const isNew = !(definitions ?? []).some((d) => d.id === definition.id);
    replace(definition);
    adding = false;
    editingId = null;
    notice = isNew
      ? m.params_def_created({ name: definition.name })
      : m.params_def_saved({ name: definition.name });
  }

  async function guarded(action: () => Promise<void>) {
    failure = '';
    notice = '';
    try {
      await action();
    } catch (e) {
      failure = isApiError(e) && e.status === 409 ? m.params_def_delete_conflict() : errorText(e);
    }
  }

  const archive = (d: ParameterDefinition) =>
    guarded(async () => replace(await paramsApi.archiveDefinition(d.id)));
  const restore = (d: ParameterDefinition) =>
    guarded(async () => replace(await paramsApi.unarchiveDefinition(d.id)));
  const remove = (d: ParameterDefinition) =>
    guarded(async () => {
      await paramsApi.removeDefinition(d.id);
      definitions = (definitions ?? []).filter((x) => x.id !== d.id);
      notice = m.params_def_deleted({ name: d.name });
    });

  const interval = (d: ParameterDefinition) =>
    d.default_test_interval_days === null
      ? m.params_def_no_interval()
      : m.params_def_interval_summary({ days: d.default_test_interval_days });
  const summary = (d: ParameterDefinition) =>
    m.detail_pair({
      first: rangeText(d.default_low, d.default_high, d.unit_label, d.decimals),
      second: interval(d)
    });
</script>

<div class="stack">
  <div class="row head">
    <p class="hint grow">{m.params_def_intro()}</p>
    {#if !adding}
      <button
        type="button"
        class="btn primary"
        onclick={() => {
          adding = true;
          editingId = null;
          notice = '';
        }}>{m.params_def_add()}</button
      >
    {/if}
  </div>

  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}
  {#if notice}
    <Alert kind="success">{notice}</Alert>
  {/if}

  {#if adding}
    <DefinitionForm onsaved={saved} oncancel={() => (adding = false)} />
  {/if}

  {#if definitions === null}
    {#if !failure}<p class="muted" role="status">{m.loading()}</p>{/if}
  {:else}
    <section class="stack" aria-labelledby="custom-heading">
      <h2 id="custom-heading">{m.params_def_custom_title()}</h2>
      {#if custom.length === 0}
        <p class="muted">{m.params_def_custom_empty()}</p>
      {:else}
        <ul class="list">
          {#each custom as d (d.id)}
            <li class="card item">
              {#if editingId === d.id}
                <DefinitionForm
                  definition={d}
                  onsaved={saved}
                  oncancel={() => (editingId = null)}
                />
              {:else}
                <div class="row">
                  <strong>{d.name}</strong>
                  {#if d.unit_label}<span class="badge">{d.unit_label}</span>{/if}
                  {#if d.archived_at}<span class="badge warning">{m.params_def_archived()}</span
                    >{/if}
                </div>
                <p class="hint">{summary(d)}</p>
                <div class="row">
                  <button
                    type="button"
                    class="btn"
                    onclick={() => {
                      editingId = d.id;
                      adding = false;
                      notice = '';
                    }}>{m.form_edit()}<span class="sr-only"> {d.name}</span></button
                  >
                  {#if d.archived_at}
                    <button type="button" class="btn" onclick={() => restore(d)}>
                      {m.params_def_restore()}<span class="sr-only"> {d.name}</span>
                    </button>
                  {:else}
                    <ConfirmButton
                      label={m.params_def_archive()}
                      question={m.params_def_archive_question({ name: d.name })}
                      confirmLabel={m.params_def_archive()}
                      onconfirm={() => archive(d)}
                    />
                  {/if}
                  <ConfirmButton
                    label={m.form_delete()}
                    question={m.params_def_delete_question({ name: d.name })}
                    confirmLabel={m.form_delete()}
                    onconfirm={() => remove(d)}
                  />
                </div>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    </section>

    <section class="stack" aria-labelledby="preset-heading">
      <h2 id="preset-heading">{m.params_def_presets_title()}</h2>
      <ul class="list">
        {#each presets as d (d.id)}
          <li class="card item">
            <strong>{d.name}</strong>
            <p class="hint">{summary(d)}</p>
          </li>
        {/each}
      </ul>
    </section>
  {/if}
</div>

<style>
  .head {
    justify-content: space-between;
  }

  .grow {
    flex: 1 1 14rem;
  }

  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .item {
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .item p {
    margin: 0;
  }
</style>
