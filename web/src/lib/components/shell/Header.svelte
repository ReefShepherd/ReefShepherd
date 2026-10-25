<script lang="ts">
  import { m } from '#lib/paraglide/messages.js';
  import TankSwitcher from './TankSwitcher.svelte';
  import ThemeSwitcher from './ThemeSwitcher.svelte';
</script>

<header>
  <a class="brand" href="/">{m.app_name()}</a>
  <div class="controls">
    <TankSwitcher />
    <ThemeSwitcher />
  </div>
</header>

<style>
  header {
    grid-area: header;
    position: sticky;
    top: 0;
    z-index: 10;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    height: var(--header-h);
    padding: 0 var(--space-3);
    background: var(--surface);
    border-bottom: 1px solid var(--border);
  }

  .brand {
    color: var(--text);
    font-weight: 700;
    text-decoration: none;
  }

  .controls {
    display: flex;
    flex: 1;
    justify-content: flex-end;
    gap: var(--space-2);
    min-width: 0;
  }

  .controls :global(select) {
    min-height: var(--touch);
    min-width: 0;
    max-width: 100%;
    padding: 0 var(--space-2);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg);
    color: var(--text);
    font: inherit;
  }
</style>
