<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { errorText } from '#lib/api/error-text.js';
  import { paramsApi, type TankParameter } from '#lib/api/params.js';
  import Alert from '#lib/components/forms/Alert.svelte';
  import { refocusMoveButton } from '#lib/dom.js';
  import { m } from '#lib/paraglide/messages.js';
  import { orderParameters, reorderChanges } from '#lib/tank/parameter-settings.js';
  import ParameterSettingsRow from './ParameterSettingsRow.svelte';

  let { tankId }: { tankId: number } = $props();

  let parameters = $state<TankParameter[] | null>(null);
  let failure = $state('');
  let busy = $state(false);

  const ordered = $derived(orderParameters(parameters ?? []));

  async function load() {
    try {
      parameters = (await paramsApi.tankParameters(tankId)).parameters;
    } catch (e) {
      failure = errorText(e);
    }
  }

  onMount(load);

  function changed(updated: TankParameter) {
    parameters = (parameters ?? []).map((p) =>
      p.parameter_id === updated.parameter_id ? updated : p
    );
  }

  async function move(from: number, direction: -1 | 1) {
    const moved = ordered[from].parameter_id;
    failure = '';
    busy = true;
    try {
      for (const change of reorderChanges(ordered, from, from + direction)) {
        await paramsApi.saveTankParameter(tankId, change.parameterId, {
          sort_order: change.sortOrder
        });
      }
    } catch (e) {
      failure = errorText(e);
    } finally {
      await load();
      busy = false;
      // Disabling every button while saving drops focus, so it goes back to the button that was used.
      await tick();
      refocusMoveButton(
        document.querySelector(`[data-parameter-id="${moved}"]`),
        direction < 0 ? 'up' : 'down'
      );
    }
  }
</script>

<section class="stack" aria-labelledby="params-heading">
  <div>
    <h2 id="params-heading">{m.params_settings_title()}</h2>
    <p class="hint">{m.params_settings_intro()}</p>
    <p><a href="/tanks/parameters">{m.params_definitions_link()}</a></p>
  </div>

  {#if failure}
    <Alert kind="error">{failure}</Alert>
  {/if}

  {#if parameters === null}
    {#if !failure}<p class="muted" role="status">{m.loading()}</p>{/if}
  {:else}
    <ul class="list">
      {#each ordered as parameter, i (parameter.parameter_id)}
        <ParameterSettingsRow
          {tankId}
          {parameter}
          canMoveUp={i > 0}
          canMoveDown={i < ordered.length - 1}
          disabled={busy}
          onchanged={changed}
          onmove={(direction) => move(i, direction)}
        />
      {/each}
    </ul>
  {/if}
</section>

<style>
  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    margin: 0;
    padding: 0;
    list-style: none;
  }
</style>
