<script lang="ts">
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';
  import { themeOptions } from '#lib/theme/options.js';
  import { isThemeName, theme } from '#lib/theme/theme.svelte.js';

  function onchange(event: Event & { currentTarget: HTMLSelectElement }) {
    const value = event.currentTarget.value;
    // Saving to the account is best effort here; the Appearance page reports failures.
    if (isThemeName(value)) session.chooseTheme(value).catch(() => {});
  }
</script>

<label class="sr-only" for="theme-switcher">{m.theme_label()}</label>
<select id="theme-switcher" value={theme.current} {onchange}>
  {#each themeOptions as option (option.name)}
    <option value={option.name}>{option.label()}</option>
  {/each}
</select>
