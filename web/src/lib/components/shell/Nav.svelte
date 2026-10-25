<script lang="ts">
  import { page } from '$app/state';
  import { m } from '#lib/paraglide/messages.js';
  import { currentTank } from '#lib/tank/current-tank.svelte.js';
  import { navEntries } from './nav.js';
  import NavIcon from './NavIcon.svelte';

  const entries = $derived(navEntries(currentTank.id));
</script>

<nav aria-label={m.nav_label()}>
  <ul>
    {#each entries as entry (entry.icon)}
      <li>
        <a href={entry.href} aria-current={entry.isActive(page.url.pathname) ? 'page' : undefined}>
          <NavIcon name={entry.icon} />
          <span>{entry.label()}</span>
        </a>
      </li>
    {/each}
  </ul>
</nav>

<style>
  nav {
    grid-area: nav;
    position: sticky;
    bottom: 0;
    z-index: 10;
    background: var(--surface);
    border-top: 1px solid var(--border);
    padding-bottom: env(safe-area-inset-bottom);
  }

  ul {
    display: flex;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  li {
    flex: 1;
    min-width: 0;
  }

  a {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 2px;
    min-height: 3.5rem;
    padding: var(--space-1);
    color: var(--text-muted);
    font-size: 0.75rem;
    text-decoration: none;
  }

  a[aria-current='page'] {
    color: var(--accent);
    font-weight: 600;
  }

  @media (min-width: 768px) {
    nav {
      top: var(--header-h);
      bottom: auto;
      align-self: start;
      height: calc(100dvh - var(--header-h));
      border-top: 0;
      border-right: 1px solid var(--border);
      padding: var(--space-3) var(--space-2);
    }

    ul {
      flex-direction: column;
      gap: var(--space-1);
    }

    a {
      flex-direction: row;
      justify-content: flex-start;
      gap: var(--space-2);
      min-height: var(--touch);
      padding: var(--space-1) var(--space-3);
      border-radius: var(--radius);
      font-size: 1rem;
    }

    a:hover {
      background: var(--surface-hover);
    }

    a[aria-current='page'] {
      background: var(--accent-soft);
    }
  }
</style>
