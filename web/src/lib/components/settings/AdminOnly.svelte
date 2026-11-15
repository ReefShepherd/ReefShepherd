<script lang="ts">
  import type { Snippet } from 'svelte';
  import Alert from '#lib/components/forms/Alert.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';

  let { children }: { children: Snippet } = $props();
</script>

{#if session.me === null}
  <p class="muted" role="status">{m.loading()}</p>
{:else if session.isAdmin}
  {@render children()}
{:else}
  <Alert kind="warning">{m.settings_admin_only()}</Alert>
{/if}
