<script lang="ts">
  import { m } from '#lib/paraglide/messages.js';
  import { isThemeName, theme, type ThemeName } from '#lib/theme/theme.svelte.js';

  const options: { name: ThemeName; label: () => string }[] = [
    { name: 'light', label: m.theme_light },
    { name: 'dark', label: m.theme_dark },
    { name: 'oled', label: m.theme_oled },
    { name: 'system', label: m.theme_system }
  ];

  function onchange(event: Event & { currentTarget: HTMLSelectElement }) {
    const value = event.currentTarget.value;
    if (isThemeName(value)) theme.set(value);
  }
</script>

<label class="sr-only" for="theme-switcher">{m.theme_label()}</label>
<select id="theme-switcher" value={theme.current} {onchange}>
  {#each options as option (option.name)}
    <option value={option.name}>{option.label()}</option>
  {/each}
</select>
