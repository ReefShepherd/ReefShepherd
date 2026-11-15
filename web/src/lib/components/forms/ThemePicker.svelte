<script lang="ts">
  import { m } from '#lib/paraglide/messages.js';
  import { themeOptions } from '#lib/theme/options.js';
  import type { ThemeName } from '#lib/theme/theme.svelte.js';

  interface Props {
    value: ThemeName;
    onchange: (name: ThemeName) => void;
    disabled?: boolean;
  }

  let { value, onchange, disabled = false }: Props = $props();

  const group = $props.id();
</script>

<fieldset class="themes">
  <legend>{m.theme_label()}</legend>
  <div class="options">
    {#each themeOptions as option (option.name)}
      <label class="choice">
        <input
          type="radio"
          name={group}
          value={option.name}
          checked={value === option.name}
          {disabled}
          onchange={() => onchange(option.name)}
        />
        <span class="choice-text">{option.label()}</span>
      </label>
    {/each}
  </div>
</fieldset>

<style>
  .options {
    display: grid;
    gap: var(--space-2);
    grid-template-columns: repeat(auto-fit, minmax(8rem, 1fr));
  }
</style>
