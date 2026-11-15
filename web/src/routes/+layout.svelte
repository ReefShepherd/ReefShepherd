<script lang="ts">
  import '../app.css';
  import { page } from '$app/state';
  import Header from '#lib/components/shell/Header.svelte';
  import Nav from '#lib/components/shell/Nav.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { pathMatches } from '#lib/path.js';
  import { session } from '#lib/session/session.svelte.js';

  let { children } = $props();

  // Sign-in and setup pages draw their own chrome and must not trigger the session redirects.
  const bare = $derived(
    pathMatches(page.url.pathname, '/login') || pathMatches(page.url.pathname, '/setup')
  );

  $effect(() => {
    if (bare) return;
    // The client redirects on its own when sign-in or setup is needed, so other failures are left to the pages.
    session.ready().catch(() => {});
  });
</script>

{#if bare}
  <main id="main" class="bare" tabindex="-1">
    {@render children()}
  </main>
{:else}
  <div class="shell">
    <a class="skip" href="#main">{m.nav_skip_to_content()}</a>
    <Header />
    <Nav />
    <main id="main" tabindex="-1">
      {@render children()}
    </main>
  </div>
{/if}

<style>
  .shell {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    grid-template-rows: auto 1fr auto;
    grid-template-areas: 'header' 'main' 'nav';
    min-height: 100dvh;
  }

  main {
    grid-area: main;
    width: 100%;
    max-width: 72rem;
    min-width: 0;
    margin: 0 auto;
    padding: var(--space-3);
  }

  main:focus {
    outline: none;
  }

  .bare {
    padding-top: var(--space-4);
  }

  .skip {
    position: absolute;
    left: var(--space-2);
    top: -4rem;
    z-index: 20;
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius);
    background: var(--surface);
  }

  .skip:focus {
    top: var(--space-2);
  }

  @media (min-width: 768px) {
    .shell {
      grid-template-columns: 14rem minmax(0, 1fr);
      grid-template-rows: auto 1fr;
      grid-template-areas: 'header header' 'nav main';
    }

    main {
      padding: var(--space-4);
    }
  }
</style>
