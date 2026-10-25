<script lang="ts">
  import { page } from '$app/state';
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';

  interface Tab {
    href: string;
    label: () => string;
    adminOnly: boolean;
  }

  const tabs: Tab[] = [
    { href: '/settings', label: m.settings_nav_appearance, adminOnly: false },
    { href: '/settings/account', label: m.settings_nav_account, adminOnly: false },
    { href: '/settings/users', label: m.settings_nav_users, adminOnly: true },
    { href: '/settings/instance', label: m.settings_nav_instance, adminOnly: true },
    { href: '/settings/about', label: m.settings_nav_about, adminOnly: false }
  ];

  const visible = $derived(tabs.filter((t) => !t.adminOnly || session.isAdmin));
</script>

<nav aria-label={m.settings_nav_label()}>
  <ul>
    {#each visible as tab (tab.href)}
      <li>
        <a href={tab.href} aria-current={page.url.pathname === tab.href ? 'page' : undefined}>
          {tab.label()}
        </a>
      </li>
    {/each}
  </ul>
</nav>

<style>
  ul {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
    margin: 0 0 var(--space-3);
    padding: 0;
    list-style: none;
    border-bottom: 1px solid var(--border);
  }

  a {
    display: inline-flex;
    align-items: center;
    min-height: var(--touch);
    padding: 0 var(--space-3);
    border-bottom: 3px solid transparent;
    color: var(--text-muted);
    text-decoration: none;
  }

  a:hover {
    color: var(--text);
  }

  a[aria-current='page'] {
    border-bottom-color: var(--accent);
    color: var(--text);
    font-weight: 600;
  }
</style>
