<script lang="ts">
  import Alert from '#lib/components/forms/Alert.svelte';
  import SettingsNav from '#lib/components/settings/SettingsNav.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { session } from '#lib/session/session.svelte.js';

  let { children } = $props();
</script>

<h1>{m.nav_settings()}</h1>

{#if session.me?.instance.auth_mode === 'disabled'}
  <Alert kind="error">{m.settings_login_off_banner()}</Alert>
{/if}

<SettingsNav />
{@render children()}
