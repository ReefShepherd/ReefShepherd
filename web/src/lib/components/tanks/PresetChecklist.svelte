<script lang="ts">
  import type { ParameterDefinition } from '#lib/api/params.js';
  import { m } from '#lib/paraglide/messages.js';
  import { rangeText } from '#lib/tank/range.js';

  interface Props {
    presets: readonly ParameterDefinition[];
    /** The preset keys that are ticked. */
    chosen: string[];
    error?: string;
  }

  let { presets, chosen = $bindable(), error }: Props = $props();
</script>

<fieldset>
  <legend>{m.tanks_presets()}</legend>
  <p class="hint">{m.tanks_presets_hint()}</p>
  <div class="list">
    {#each presets as preset (preset.id)}
      {#if preset.preset_key}
        <label class="choice">
          <input type="checkbox" value={preset.preset_key} bind:group={chosen} />
          <span class="choice-text">
            <span>{preset.name}</span>
            <span class="hint">
              {rangeText(
                preset.default_low,
                preset.default_high,
                preset.unit_label,
                preset.decimals
              )}
            </span>
          </span>
        </label>
      {/if}
    {/each}
  </div>
  {#if error}<p class="field-error">{error}</p>{/if}
</fieldset>

<style>
  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin-top: var(--space-2);
  }
</style>
